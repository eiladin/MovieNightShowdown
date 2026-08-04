package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// settingsResponse is the JSON body of GET and POST /api/settings.
//
// It carries no credential. A stored secret is reported as a boolean only,
// because a settings screen never needs to read one back — it only needs to
// know whether one is set, so it can render a masked placeholder instead of an
// empty field. Returning the value would put every credential in a browser's
// memory, its cache, and any proxy in between, for no gain.
type settingsResponse struct {
	PublicURL  string `json:"publicUrl"`
	SessionTTL string `json:"sessionTtl"`

	Jellyfin  jellyfinSettings  `json:"jellyfin"`
	Plex      plexSettings      `json:"plex"`
	Streaming streamingSettings `json:"streaming"`

	// Provenance says where each setting's live value came from, so the screen
	// can flag one the operator may believe is in effect when it is not.
	Provenance map[string]provenanceView `json:"provenance"`

	// Outcome reports what a save did to the running server: nothing changed,
	// it was applied live, or it is persisted but needs a restart. It is empty
	// on a read.
	Outcome string `json:"outcome,omitempty"`

	// RestartRequired reports that the saved values are not yet live.
	RestartRequired bool `json:"restartRequired"`
}

type jellyfinSettings struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url"`
	APIKeySet bool   `json:"apiKeySet"`
	UserID    string `json:"userId"`
}

type plexSettings struct {
	Enabled        bool   `json:"enabled"`
	URL            string `json:"url"`
	TokenSet       bool   `json:"tokenSet"`
	LibrarySection string `json:"librarySection"`
}

type streamingSettings struct {
	Enabled          bool     `json:"enabled"`
	TMDBReadTokenSet bool     `json:"tmdbReadTokenSet"`
	WatchRegion      string   `json:"watchRegion"`
	Providers        []string `json:"providers"`
}

// provenanceView is the client-facing form of settingProvenance. It names the
// origin and, when the config file has overridden an environment variable,
// which variable is being ignored.
type provenanceView struct {
	Source     string `json:"source"`
	EnvVar     string `json:"envVar,omitempty"`
	EnvIgnored bool   `json:"envIgnored,omitempty"`
}

// settingsRequest is the JSON body of POST /api/settings.
//
// Every field is a pointer: an omitted field means "leave this alone". That is
// what makes a partial save safe, and it is load-bearing for secrets in
// particular — the screen never receives a stored credential, so it cannot send
// one back, and treating an omitted secret as "clear" would wipe a working
// credential every time anyone changed an unrelated field. Clearing is an
// explicit act; see the clear flags.
type settingsRequest struct {
	PublicURL  *string `json:"publicUrl"`
	SessionTTL *string `json:"sessionTtl"`

	Jellyfin  *jellyfinRequest  `json:"jellyfin"`
	Plex      *plexRequest      `json:"plex"`
	Streaming *streamingRequest `json:"streaming"`
}

type jellyfinRequest struct {
	Enabled     *bool   `json:"enabled"`
	URL         *string `json:"url"`
	APIKey      *string `json:"apiKey"`
	ClearAPIKey bool    `json:"clearApiKey"`
	UserID      *string `json:"userId"`
	ClearUserID bool    `json:"clearUserId"`
}

type plexRequest struct {
	Enabled        *bool   `json:"enabled"`
	URL            *string `json:"url"`
	Token          *string `json:"token"`
	ClearToken     bool    `json:"clearToken"`
	LibrarySection *string `json:"librarySection"`
}

type streamingRequest struct {
	Enabled            *bool     `json:"enabled"`
	TMDBReadToken      *string   `json:"tmdbReadToken"`
	ClearTMDBReadToken bool      `json:"clearTmdbReadToken"`
	WatchRegion        *string   `json:"watchRegion"`
	Providers          *[]string `json:"providers"`
}

// handleGetSettings returns the current configuration without any credential.
//
// It requires the setup token: the shape of a deployment's configuration —
// which sources exist, which URLs they point at, whether a credential is set —
// is reconnaissance, not public information.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSetup(r) {
		unauthorized(w)
		return
	}
	file, err := loadConfigFile(s.config().ConfigPath)
	if err != nil {
		http.Error(w, "config file unreadable", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.settingsView(file, ""))
}

// handleSetSettings validates and persists a configuration change.
func (s *Server) handleSetSettings(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSetup(r) {
		unauthorized(w)
		return
	}

	var req settingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"errors": map[string]string{"_": "request body is not valid JSON"},
		})
		return
	}

	path := s.config().ConfigPath
	file, err := loadConfigFile(path)
	if err != nil {
		http.Error(w, "config file unreadable", http.StatusInternalServerError)
		return
	}
	if file == nil {
		file = &configFile{}
	}

	merged := applySettings(file, req)
	if errs := validateSettings(merged); len(errs) > 0 {
		// Validation runs on the merged result, not the request, so a change
		// that would leave the configuration invalid is rejected even when the
		// offending value was already on disk. Nothing is written.
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": errs})
		return
	}

	if err := writeConfigFile(path, merged); err != nil {
		http.Error(w, "could not write the config file", http.StatusInternalServerError)
		return
	}

	// Re-resolve from the file just written rather than from the request: the
	// environment still contributes every key the file does not set, so the
	// request alone does not describe what the server will actually run with.
	next, err := resolveConfigAt(path, true)
	if err != nil {
		http.Error(w, "the saved configuration could not be reloaded", http.StatusInternalServerError)
		return
	}
	outcome := s.applyConfig(next)
	writeJSON(w, http.StatusOK, s.settingsView(merged, outcome))
}

// applySettings merges a request onto the stored config file, returning the
// result. The stored file is not mutated: validation may reject the result, and
// a rejected write must leave the previous configuration untouched.
func applySettings(file *configFile, req settingsRequest) *configFile {
	out := *file
	if file.Jellyfin != nil {
		jf := *file.Jellyfin
		out.Jellyfin = &jf
	}
	if file.Plex != nil {
		px := *file.Plex
		out.Plex = &px
	}
	if file.Streaming != nil {
		st := *file.Streaming
		out.Streaming = &st
	}

	setString(&out.PublicURL, req.PublicURL, false)
	setString(&out.SessionTTL, req.SessionTTL, false)

	if req.Jellyfin != nil {
		if out.Jellyfin == nil {
			out.Jellyfin = &jellyfinSection{}
		}
		setBool(&out.Jellyfin.Enabled, req.Jellyfin.Enabled)
		setString(&out.Jellyfin.URL, req.Jellyfin.URL, false)
		setString(&out.Jellyfin.APIKey, req.Jellyfin.APIKey, req.Jellyfin.ClearAPIKey)
		setString(&out.Jellyfin.UserID, req.Jellyfin.UserID, req.Jellyfin.ClearUserID)
	}
	if req.Plex != nil {
		if out.Plex == nil {
			out.Plex = &plexSection{}
		}
		setBool(&out.Plex.Enabled, req.Plex.Enabled)
		setString(&out.Plex.URL, req.Plex.URL, false)
		setString(&out.Plex.Token, req.Plex.Token, req.Plex.ClearToken)
		setString(&out.Plex.LibrarySection, req.Plex.LibrarySection, false)
	}
	if req.Streaming != nil {
		if out.Streaming == nil {
			out.Streaming = &streamingSection{}
		}
		setBool(&out.Streaming.Enabled, req.Streaming.Enabled)
		setString(&out.Streaming.TMDBReadToken, req.Streaming.TMDBReadToken, req.Streaming.ClearTMDBReadToken)
		setString(&out.Streaming.WatchRegion, req.Streaming.WatchRegion, false)
		if req.Streaming.Providers != nil {
			p := normalizeProviders(*req.Streaming.Providers)
			out.Streaming.Providers = &p
		}
	}
	return &out
}

// setString applies one optional field. An omitted value leaves the stored one
// alone; an explicit clear removes it entirely, which is distinct from setting
// it to the empty string.
func setString(dst **string, val *string, clear bool) {
	if clear {
		*dst = nil
		return
	}
	if val != nil {
		v := *val
		*dst = &v
	}
}

func setBool(dst **bool, val *bool) {
	if val != nil {
		v := *val
		*dst = &v
	}
}

// validateSettings checks a merged configuration, returning field-keyed errors.
// The keys match the config file's dotted names so a client can attach each
// message to the field that caused it.
func validateSettings(cf *configFile) map[string]string {
	errs := map[string]string{}

	checkURL := func(key string, val *string) {
		if val == nil || *val == "" {
			return
		}
		u, err := url.Parse(*val)
		if err != nil || u.Host == "" {
			errs[key] = "must be a valid URL, for example http://host:port"
			return
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			errs[key] = "must use http or https"
		}
	}

	checkURL("publicUrl", cf.PublicURL)
	checkURL("jellyfin.url", jellyfinURL(cf.Jellyfin))
	checkURL("plex.url", plexURL(cf.Plex))

	// A source switched on without its credentials would be advertised and
	// then fail every query, which is the state the setup page exists to
	// prevent. Reject it at the point it is configured instead.
	if cf.Jellyfin != nil && enabledOrDefault(cf.Jellyfin.Enabled) {
		if isBlank(cf.Jellyfin.URL) {
			errs["jellyfin.url"] = "required when Jellyfin is enabled"
		}
		if isBlank(cf.Jellyfin.APIKey) {
			errs["jellyfin.apiKey"] = "required when Jellyfin is enabled"
		}
	}
	if cf.Plex != nil && enabledOrDefault(cf.Plex.Enabled) {
		if isBlank(cf.Plex.URL) {
			errs["plex.url"] = "required when Plex is enabled"
		}
		if isBlank(cf.Plex.Token) {
			errs["plex.token"] = "required when Plex is enabled"
		}
	}
	if cf.Streaming != nil && enabledOrDefault(cf.Streaming.Enabled) {
		if isBlank(cf.Streaming.TMDBReadToken) {
			errs["streaming.tmdbReadToken"] = "required when streaming is enabled"
		}
	}
	return errs
}

func isBlank(s *string) bool { return s == nil || strings.TrimSpace(*s) == "" }

func enabledOrDefault(b *bool) bool { return b == nil || *b }

func jellyfinURL(s *jellyfinSection) *string {
	if s == nil {
		return nil
	}
	return s.URL
}

func plexURL(s *plexSection) *string {
	if s == nil {
		return nil
	}
	return s.URL
}

// settingsView renders the stored configuration for a client, reporting each
// secret as set or unset and never as a value.
func (s *Server) settingsView(file *configFile, outcome reloadOutcome) settingsResponse {
	var top configFile
	var jf jellyfinSection
	var px plexSection
	var st streamingSection
	if file != nil {
		top = *file
		if file.Jellyfin != nil {
			jf = *file.Jellyfin
		}
		if file.Plex != nil {
			px = *file.Plex
		}
		if file.Streaming != nil {
			st = *file.Streaming
		}
	}

	live := s.config()
	prov := make(map[string]provenanceView, len(live.Provenance))
	for k, p := range live.Provenance {
		prov[k] = provenanceView{Source: string(p.Source), EnvVar: p.EnvVar, EnvIgnored: p.EnvIgnored}
	}

	providers := []string{}
	if st.Providers != nil {
		providers = *st.Providers
	}

	return settingsResponse{
		PublicURL:  orEmpty(top.PublicURL),
		SessionTTL: orEmpty(top.SessionTTL),
		Jellyfin: jellyfinSettings{
			Enabled:   enabledOrDefault(jf.Enabled),
			URL:       orEmpty(jf.URL),
			APIKeySet: !isBlank(jf.APIKey),
			UserID:    orEmpty(jf.UserID),
		},
		Plex: plexSettings{
			Enabled:        enabledOrDefault(px.Enabled),
			URL:            orEmpty(px.URL),
			TokenSet:       !isBlank(px.Token),
			LibrarySection: orEmpty(px.LibrarySection),
		},
		Streaming: streamingSettings{
			Enabled:          enabledOrDefault(st.Enabled),
			TMDBReadTokenSet: !isBlank(st.TMDBReadToken),
			WatchRegion:      orEmpty(st.WatchRegion),
			Providers:        providers,
		},
		Provenance:      prov,
		Outcome:         string(outcome),
		RestartRequired: outcome == reloadRestartRequired,
	}
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// unauthorized answers a request with a missing or wrong setup token. The two
// cases are deliberately indistinguishable: telling a caller that a token was
// recognized but wrong confirms the header name and the format for them.
func unauthorized(w http.ResponseWriter) {
	http.Error(w, "a valid setup token is required", http.StatusUnauthorized)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
