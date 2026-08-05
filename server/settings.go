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

	// Runtime holds the settings fixed at container level. They are reported
	// so an operator can see what is in effect, and are absent from the
	// request type because saving cannot change them: each names something
	// established before the process started.
	Runtime runtimeSettings `json:"runtime"`

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

// runtimeSettings are read-only. Changing any of them means changing the
// deployment and recreating the container, so the screen shows them and says
// so rather than offering an input that could not take effect.
type runtimeSettings struct {
	Port       string `json:"port"`
	CacheDir   string `json:"cacheDir"`
	ConfigPath string `json:"configPath"`
}

type jellyfinSettings struct {
	Enabled   bool            `json:"enabled"`
	URL       string          `json:"url"`
	APIKeySet bool            `json:"apiKeySet"`
	UserID    string          `json:"userId"`
	Libraries []libraryOption `json:"libraries"`
}

type plexSettings struct {
	Enabled        bool            `json:"enabled"`
	URL            string          `json:"url"`
	TokenSet       bool            `json:"tokenSet"`
	LibrarySection string          `json:"librarySection"`
	Libraries      []libraryOption `json:"libraries"`
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
	// Libraries is the chosen set, as identifier and name pairs. The screen has
	// both because it enumerated them to render its picker, and writing both means
	// a later start has no names to resolve.
	//
	// An empty list is a value and means "every library", the same as never having
	// chosen. Omitted means unchanged, like every other field here.
	Libraries *[]libraryOption `json:"libraries"`
}

type plexRequest struct {
	Enabled        *bool   `json:"enabled"`
	URL            *string `json:"url"`
	Token          *string `json:"token"`
	ClearToken     bool    `json:"clearToken"`
	LibrarySection *string `json:"librarySection"`
	// Libraries carries the same meaning as on jellyfinRequest.
	Libraries *[]libraryOption `json:"libraries"`
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
	writeJSON(w, http.StatusOK, s.settingsView(s.config(), ""))
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

	// Validate what the server would actually run with, not the file alone. The
	// environment still supplies every key the file leaves unset, so validating
	// the file would reject a save for a credential that is set — just not
	// there. This resolves the pending file in memory; nothing is written yet.
	next := resolveFrom(merged, path)
	if errs := validateSettings(next); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": errs})
		return
	}

	if err := writeConfigFile(path, merged); err != nil {
		http.Error(w, "could not write the config file", http.StatusInternalServerError)
		return
	}

	outcome := s.applyConfig(next)
	// Render from the configuration that is now live, not from the file just
	// written: the environment still contributes every key the file leaves
	// unset, so the file alone does not describe what the server is running.
	writeJSON(w, http.StatusOK, s.settingsView(s.config(), outcome))
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
		setLibraries(&out.Jellyfin.Libraries, req.Jellyfin.Libraries)
	}
	if req.Plex != nil {
		if out.Plex == nil {
			out.Plex = &plexSection{}
		}
		setBool(&out.Plex.Enabled, req.Plex.Enabled)
		setString(&out.Plex.URL, req.Plex.URL, false)
		setString(&out.Plex.Token, req.Plex.Token, req.Plex.ClearToken)
		setString(&out.Plex.LibrarySection, req.Plex.LibrarySection, false)
		setLibraries(&out.Plex.Libraries, req.Plex.Libraries)
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

// setLibraries applies an optional library list. An omitted list leaves the stored
// one alone; a supplied one replaces it, including when it is empty — an empty list
// means "every library", which is a choice and not an absence.
//
// An entry with no identifier is dropped: the screen only ever sends pairs it read
// from the server, so one without an id is a malformed request rather than a
// meaningful selection.
func setLibraries(dst **[]fileLibrary, val *[]libraryOption) {
	if val == nil {
		return
	}
	out := make([]fileLibrary, 0, len(*val))
	for _, l := range *val {
		id := strings.TrimSpace(l.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(l.Name)
		out = append(out, fileLibrary{ID: &id, Name: &name})
	}
	*dst = &out
}

func setBool(dst **bool, val *bool) {
	if val != nil {
		v := *val
		*dst = &v
	}
}

// validateSettings checks a resolved configuration, returning field-keyed
// errors. The keys match the config file's dotted names so a client can attach
// each message to the field that caused it.
//
// It takes the resolved Config rather than the config file because a value the
// environment supplies is just as real as one the file holds. Validating the
// file alone rejects a save for a credential that exists.
func validateSettings(cfg Config) map[string]string {
	errs := map[string]string{}

	checkURL := func(key, val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		u, err := url.Parse(val)
		if err != nil || u.Host == "" {
			errs[key] = "must be a valid URL, for example http://host:port"
			return
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			errs[key] = "must use http or https"
		}
	}

	checkURL("publicUrl", cfg.PublicURL)
	checkURL("jellyfin.url", cfg.JellyfinURL)
	checkURL("plex.url", cfg.PlexURL)

	// A source switched on without its credentials would be advertised and then
	// fail every query, which is the state the setup page exists to prevent.
	// Reject it at the point it is configured instead.
	//
	// A source that is off is not checked: an operator switching one off to fix
	// it later must not be blocked by the very values they are removing.
	if !cfg.JellyfinDisabled && (cfg.JellyfinURL != "" || cfg.JellyfinAPIKey != "") {
		if strings.TrimSpace(cfg.JellyfinURL) == "" {
			errs["jellyfin.url"] = "required when Jellyfin is enabled"
		}
		if strings.TrimSpace(cfg.JellyfinAPIKey) == "" {
			errs["jellyfin.apiKey"] = "required when Jellyfin is enabled"
		}
	}
	if !cfg.PlexDisabled && (cfg.PlexURL != "" || cfg.PlexToken != "") {
		if strings.TrimSpace(cfg.PlexURL) == "" {
			errs["plex.url"] = "required when Plex is enabled"
		}
		if strings.TrimSpace(cfg.PlexToken) == "" {
			errs["plex.token"] = "required when Plex is enabled"
		}
	}
	// Only a deliberate selection requires a token. The built-in default list
	// is inert without one — it is what an untouched deployment carries — so
	// demanding a token for it would reject every save from a deployment that
	// never configured streaming at all.
	if !cfg.StreamingDisabled &&
		cfg.Provenance["streaming.providers"].Source == sourceFile &&
		len(cfg.StreamingProviders) > 0 {
		if strings.TrimSpace(cfg.TMDBReadToken) == "" {
			errs["streaming.tmdbReadToken"] = "required when streaming services are selected"
		}
	}
	return errs
}

// settingsView renders the live configuration for a client, reporting each
// secret as set or unset and never as a value.
//
// It renders the *resolved* configuration, not the config file. Those differ
// for any deployment seeded by environment variables — which is every
// deployment that has not saved settings yet — and rendering the file would
// show that operator a screen of empty fields describing a server that is
// working perfectly well. Worse, saving what they saw would then write those
// blanks over a working configuration.
//
// Provenance travels alongside so the screen can still say which values came
// from the environment rather than from a save.
func (s *Server) settingsView(cfg Config, outcome reloadOutcome) settingsResponse {
	prov := make(map[string]provenanceView, len(cfg.Provenance))
	for k, p := range cfg.Provenance {
		prov[k] = provenanceView{Source: string(p.Source), EnvVar: p.EnvVar, EnvIgnored: p.EnvIgnored}
	}

	providers := cfg.StreamingProviders
	if providers == nil {
		providers = []string{}
	}

	return settingsResponse{
		PublicURL:  cfg.PublicURL,
		SessionTTL: cfg.SessionTTL,
		Runtime: runtimeSettings{
			Port:       cfg.Port,
			CacheDir:   cfg.CacheDir,
			ConfigPath: cfg.ConfigPath,
		},
		Jellyfin: jellyfinSettings{
			Enabled: sourceEnabled(cfg.JellyfinDisabled,
				cfg.JellyfinURL != "" || cfg.JellyfinAPIKey != ""),
			URL:       cfg.JellyfinURL,
			APIKeySet: cfg.JellyfinAPIKey != "",
			UserID:    cfg.JellyfinUserID,
			Libraries: libraryOptions(cfg.JellyfinLibraries),
		},
		Plex: plexSettings{
			Enabled: sourceEnabled(cfg.PlexDisabled,
				cfg.PlexURL != "" || cfg.PlexToken != ""),
			URL:            cfg.PlexURL,
			TokenSet:       cfg.PlexToken != "",
			LibrarySection: cfg.PlexLibrarySection,
			Libraries:      libraryOptions(cfg.PlexLibraries),
		},
		Streaming: streamingSettings{
			Enabled:          sourceEnabled(cfg.StreamingDisabled, cfg.TMDBReadToken != ""),
			TMDBReadTokenSet: cfg.TMDBReadToken != "",
			WatchRegion:      cfg.TMDBWatchRegion,
			Providers:        providers,
		},
		Provenance:      prov,
		Outcome:         string(outcome),
		RestartRequired: outcome == reloadRestartRequired,
	}
}

// libraryOptions renders a resolved library list for the client. It is never nil,
// so the screen can tell "none chosen" from a missing field without a null check.
func libraryOptions(refs []libraryRef) []libraryOption {
	out := make([]libraryOption, 0, len(refs))
	for _, ref := range refs {
		out = append(out, libraryOption(ref))
	}
	return out
}

// sourceEnabled decides what the screen's toggle shows for one source.
//
// A source is on only when it is not switched off and something is actually
// configured for it. An "enabled" flag with no values behind it is not a state
// worth showing as on: no source is registered from it, every query would fail,
// and the operator is left looking at an expanded section demanding credentials
// for a service they do not use.
func sourceEnabled(disabled, hasValues bool) bool {
	return !disabled && hasValues
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
