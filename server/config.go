package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds resolved server configuration. Values come from the config
// file, the environment, or a built-in default, resolved per key by
// LoadConfig; Provenance records which applied to each.
type Config struct {
	JellyfinURL    string
	JellyfinAPIKey string
	JellyfinUserID string
	PlexURL        string
	PlexToken      string
	// PlexLibrarySection is the key of the Plex library section holding
	// movies. It is optional: with it unset the client discovers the first
	// section of type "movie" on first use. A server with more than one movie
	// section needs it set, since discovery would otherwise pick whichever
	// Plex listed first.
	PlexLibrarySection string
	PublicURL          string
	Port               string
	SessionTTL         string
	CacheDir           string
	TMDBReadToken      string
	// TMDBWatchRegion is the ISO 3166-1 region streaming availability is
	// judged against. Which services exist, and what they carry, both depend
	// on it.
	TMDBWatchRegion string
	// StreamingProviders is the list of streaming services this deployment
	// asks for, as written in the environment: provider names or numeric TMDB
	// provider ids, normalized but not yet resolved. Resolution needs the
	// network, so it happens in New rather than here. The list is inert
	// without TMDBReadToken, since every streaming source is queried via TMDB.
	StreamingProviders []string

	// ConfigPath is where the config file was looked for, whether or not one
	// was found. Reported at startup so "which file is in effect" is never a
	// question.
	ConfigPath string

	// JellyfinEnabled, PlexEnabled and StreamingEnabled are the config file's
	// per-source toggles, reported so the settings screen can render them.
	// They do not gate the Configured predicates: LoadConfig clears a disabled
	// source's credentials instead, so "disabled" has exactly one meaning
	// downstream and a hand-built Config with a zero value here still behaves
	// as its credentials say. They default to true, so a deployment without a
	// config file behaves exactly as it did before one existed.
	JellyfinEnabled  bool
	PlexEnabled      bool
	StreamingEnabled bool

	// Provenance records how each setting was resolved, keyed by the config
	// file's dotted key names. It carries no values, only their origins.
	Provenance map[string]settingProvenance

	// tmdbBaseURL is the TMDB API root. It is unexported and not read from the
	// environment: it exists only so tests can point resolution at a stub.
	tmdbBaseURL string
}

// defaultStreamingProviders is the set offered when STREAMING_PROVIDERS is
// unset, preserving the behaviour deployments had before it existed.
var defaultStreamingProviders = []string{
	string(SourceNetflix), string(SourcePrime), string(SourceDisney),
}

// parseStreamingProviders reads the comma-separated STREAMING_PROVIDERS value.
// Entries are trimmed, lowercased, and de-duplicated; empty entries are
// skipped. An unset (or whitespace-only) value yields the default set.
//
// Entries are deliberately not validated here. Any TMDB watch provider may be
// named, and knowing which names exist requires asking TMDB, so an unrecognized
// entry is reported (and skipped) during resolution rather than at parse time.
func parseStreamingProviders(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return defaultStreamingProviders
	}
	seen := make(map[string]bool)
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// LoadConfig resolves configuration from the config file, the environment, and
// built-in defaults, in that order of precedence, per key.
//
// The config file is authoritative because the application owns it: the
// settings screen writes it, and an environment variable that outranked it
// would make saving a setting silently ineffective. Environment variables
// therefore seed a deployment that has no file yet and are reported as ignored
// once the file sets the same key. PORT and CONFIG_FILE are excluded and remain
// environment-only — the listener cannot be rebound live, and the config path
// is how the file is found at all.
//
// An error is returned only for a config file that cannot be used: malformed
// YAML, or a missing file at a path the operator named explicitly. A missing
// file at the default path is not an error, since no deployment has one until
// it saves settings for the first time.
func LoadConfig() (Config, error) {
	path, explicit := resolveConfigPath()
	return resolveConfigAt(path, explicit)
}

// resolveConfigAt performs the resolution LoadConfig describes against an
// explicit path. It is separate so a configuration save can re-resolve the file
// it just wrote without consulting CONFIG_FILE again, which a running process
// must not re-read: the path it started with is the path it owns.
func resolveConfigAt(path string, explicit bool) (Config, error) {
	file, err := loadConfigFile(path)
	if err != nil {
		return Config{}, err
	}
	if file == nil && explicit {
		// A typo in CONFIG_FILE must not start the application with none of
		// its settings, silently, looking healthy.
		return Config{}, fmt.Errorf("config file %s: does not exist (set by CONFIG_FILE)", path)
	}
	if file != nil {
		checkConfigFilePermissions(path)
		log.Printf("config: loaded %s", path)
	} else {
		log.Printf("config: no config file at %s; using environment and defaults", path)
	}

	// Dereference the file's optional sections once. An absent section is an
	// all-nil value, which resolves exactly as an absent file does: every key
	// falls through to the environment.
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

	r := newResolver(file)
	cfg := Config{
		ConfigPath: path,

		JellyfinEnabled: r.enabled("jellyfin.enabled", jf.Enabled),
		JellyfinURL:     r.str("jellyfin.url", "JELLYFIN_URL", jf.URL, ""),
		JellyfinAPIKey:  r.secret("jellyfin.apiKey", "JELLYFIN_API_KEY", jf.APIKey),
		JellyfinUserID:  r.str("jellyfin.userId", "JELLYFIN_USER_ID", jf.UserID, ""),

		PlexEnabled:        r.enabled("plex.enabled", px.Enabled),
		PlexURL:            r.str("plex.url", "PLEX_URL", px.URL, ""),
		PlexToken:          r.secret("plex.token", "PLEX_TOKEN", px.Token),
		PlexLibrarySection: r.str("plex.librarySection", "PLEX_LIBRARY_SECTION", px.LibrarySection, ""),

		StreamingEnabled: r.enabled("streaming.enabled", st.Enabled),
		TMDBReadToken:    r.secret("streaming.tmdbReadToken", "TMDB_READ_TOKEN", st.TMDBReadToken),
		TMDBWatchRegion:  r.str("streaming.watchRegion", "TMDB_WATCH_REGION", st.WatchRegion, defaultWatchRegion),

		PublicURL:  r.str("publicUrl", "PUBLIC_URL", top.PublicURL, "http://localhost:8080"),
		SessionTTL: r.str("sessionTtl", "SESSION_TTL", top.SessionTTL, "4h"),
		CacheDir:   r.str("cacheDir", "CACHE_DIR", top.CacheDir, filepath.Join(os.TempDir(), "mns-posters")),

		Port:        os.Getenv("PORT"),
		tmdbBaseURL: tmdbAPIBase,
	}
	cfg.StreamingProviders = r.providers("streaming.providers", "STREAMING_PROVIDERS", st.Providers)
	// A source the operator switched off must not be queryable, so its
	// credentials are dropped here rather than checked at every use. The
	// values stay in the config file; only this resolved view forgets them.
	if !cfg.JellyfinEnabled {
		cfg.JellyfinURL, cfg.JellyfinAPIKey, cfg.JellyfinUserID = "", "", ""
	}
	if !cfg.PlexEnabled {
		cfg.PlexURL, cfg.PlexToken, cfg.PlexLibrarySection = "", "", ""
	}
	if !cfg.StreamingEnabled {
		cfg.TMDBReadToken = ""
	}
	cfg.TMDBWatchRegion = strings.ToUpper(cfg.TMDBWatchRegion)
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	cfg.Provenance = r.prov
	logProvenance(r.prov, map[string]string{
		"publicUrl": cfg.PublicURL, "sessionTtl": cfg.SessionTTL, "cacheDir": cfg.CacheDir,
		"jellyfin.enabled": strconv.FormatBool(cfg.JellyfinEnabled), "jellyfin.url": cfg.JellyfinURL,
		"jellyfin.apiKey": cfg.JellyfinAPIKey, "jellyfin.userId": cfg.JellyfinUserID,
		"plex.enabled": strconv.FormatBool(cfg.PlexEnabled), "plex.url": cfg.PlexURL,
		"plex.token": cfg.PlexToken, "plex.librarySection": cfg.PlexLibrarySection,
		"streaming.enabled": strconv.FormatBool(cfg.StreamingEnabled), "streaming.tmdbReadToken": cfg.TMDBReadToken,
		"streaming.watchRegion": cfg.TMDBWatchRegion, "streaming.providers": strings.Join(cfg.StreamingProviders, ","),
	})
	return cfg, nil
}

// JellyfinConfigured reports whether this deployment can query Jellyfin. Both
// values are needed: a URL without a key cannot authenticate, and a key without
// a URL has nowhere to go.
func (c Config) JellyfinConfigured() bool {
	return c.JellyfinURL != "" && c.JellyfinAPIKey != ""
}

// PlexConfigured reports whether this deployment can query Plex. Both values
// are needed for the same reason Jellyfin needs both: a URL without a token
// cannot authenticate, and a token without a URL has nowhere to go.
func (c Config) PlexConfigured() bool {
	return c.PlexURL != "" && c.PlexToken != ""
}

// StreamingConfigured reports whether this deployment can query any streaming
// service. Every streaming source goes through TMDB, so the token is required;
// STREAMING_PROVIDERS can also narrow the list to nothing.
func (c Config) StreamingConfigured() bool {
	return c.TMDBReadToken != "" && len(c.StreamingProviders) > 0
}

// String renders the config for logging with the API key masked.
func (c Config) String() string {
	masked := "(unset)"
	if c.JellyfinAPIKey != "" {
		masked = "***"
	}
	maskedTMDB := "(unset)"
	if c.TMDBReadToken != "" {
		maskedTMDB = "***"
	}
	maskedPlex := "(unset)"
	if c.PlexToken != "" {
		maskedPlex = "***"
	}
	return fmt.Sprintf(
		"JellyfinURL=%s JellyfinAPIKey=%s JellyfinUserID=%s PlexURL=%s PlexToken=%s PlexLibrarySection=%s PublicURL=%s Port=%s SessionTTL=%s CacheDir=%s TMDBReadToken=%s TMDBWatchRegion=%s StreamingProviders=%s",
		c.JellyfinURL, masked, c.JellyfinUserID,
		c.PlexURL, maskedPlex, c.PlexLibrarySection,
		c.PublicURL, c.Port, c.SessionTTL, c.CacheDir, maskedTMDB,
		c.TMDBWatchRegion, strings.Join(c.StreamingProviders, ","),
	)
}
