package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// libraryRef names one media library to draw movies from.
//
// Name is optional. An environment variable supplies bare strings, so a
// deployment configured that way has identifiers and no display names until
// something learns them. That is deliberate rather than a gap: the identifier is
// all a query needs, and the name is only a label — so a library configured by id
// works with no network call at all, and one configured by name is resolvable
// later without blocking startup.
type libraryRef struct {
	// ID is the library's own identifier, opaque to this application. Jellyfin's
	// is a hexadecimal string, Plex's is an integer, and neither is this
	// application's to reformat or case-fold.
	ID string
	// Name is the library's display name, empty when only an identifier is known.
	Name string
}

// Config holds resolved server configuration. Values come from the config
// file, the environment, or a built-in default, resolved per key by
// LoadConfig; Provenance records which applied to each.
type Config struct {
	JellyfinURL    string
	JellyfinAPIKey string
	JellyfinUserID string
	PlexURL        string
	PlexToken      string
	// PlexLibrarySection is the deprecated single-section setting. It is folded
	// into PlexLibraries during resolution and must not be read directly:
	// PLEX_LIBRARY_SECTION remains honoured so an existing deployment does not
	// silently change which library it deals from, but PLEX_LIBRARY_SECTIONS is
	// the setting that means anything.
	PlexLibrarySection string

	// JellyfinLibraries and PlexLibraries are the libraries this deployment draws
	// from, one movie source per entry.
	//
	// An empty list means every library, which is what an unconfigured deployment
	// has always done: one unscoped source per service. It is not "no libraries" —
	// that state would leave a configured service with nothing to query, which no
	// operator asks for by leaving a setting blank.
	JellyfinLibraries []libraryRef
	PlexLibraries     []libraryRef
	PublicURL         string
	Port              string
	SessionTTL        string
	CacheDir          string
	TMDBReadToken     string
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

	// JellyfinDisabled, PlexDisabled and StreamingDisabled are the config
	// file's per-source toggles, inverted.
	//
	// The inversion is load-bearing. Stored as "enabled", the zero value of a
	// hand-built Config would disable every source, which is a trap for any
	// caller that builds one directly. Stored as "disabled", the zero value
	// means what a deployment without a config file has always meant: a source
	// is available whenever its credentials are set.
	//
	// They gate the Configured predicates rather than causing credentials to be
	// cleared, so a disabled source keeps its resolved values and the settings
	// screen can still show what is stored for it.
	JellyfinDisabled  bool
	PlexDisabled      bool
	StreamingDisabled bool

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
// once the file sets the same key.
//
// PORT, CACHE_DIR and CONFIG_FILE are excluded and remain environment-only.
// Each names something established before the process starts: the listener is
// already bound and cannot be rebound live, the cache directory is a path that
// has to be mounted before it can be used, and the config path is how the file
// is found at all.
//
// A missing config file is never an error. A deployment has none until it saves
// settings for the first time, and the application creates it on its first
// write. What is an error is a file that cannot be used: malformed YAML, or an
// explicitly named path whose directory cannot be created — every later save
// would fail against it, so it fails at startup instead.
func LoadConfig() (Config, error) {
	path, explicit := resolveConfigPath()
	cfg, err := resolveConfigAt(path, explicit)
	if err != nil {
		return Config{}, err
	}
	logResolvedConfig(cfg)
	return cfg, nil
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
	if err := prepareConfigPath(file, path, explicit); err != nil {
		return Config{}, err
	}
	return resolveFrom(file, path), nil
}

// prepareConfigPath reports whether a config file was found and makes sure an
// explicitly named path is usable.
func prepareConfigPath(file *configFile, path string, explicit bool) error {
	if file == nil && explicit {
		// A missing file at an explicitly configured path is the normal first
		// boot: a fresh deployment has saved nothing yet, and the application
		// creates the file itself when it first writes. Refusing to start here
		// would crash-loop every new install that sets CONFIG_FILE.
		//
		// What is worth catching is a path that cannot be used at all, since
		// every later save would fail against it. Creating the directory now
		// turns that into one clear startup error instead of a settings screen
		// that reports success and persists nothing.
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("config file %s: its directory cannot be created: %w", path, err)
		}
	}
	if file != nil {
		checkConfigFilePermissions(path)
		log.Printf("config: loaded %s", path)
	} else {
		log.Printf("config: no config file at %s yet; using environment and defaults, "+
			"and saving settings will create it", path)
	}
	return nil
}

// resolveFrom merges an in-memory config file with the environment and the
// built-in defaults.
//
// It takes the file rather than a path so a pending change can be resolved
// before it is written. Validating and reporting against the merged file alone
// would ignore every value the environment supplies — which is how a save came
// to be rejected for a credential that was set, just not in the file.
func resolveFrom(file *configFile, path string) Config {
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

		JellyfinDisabled: r.disabled("jellyfin.enabled", jf.Enabled),
		JellyfinURL:      r.str("jellyfin.url", "JELLYFIN_URL", jf.URL, ""),
		JellyfinAPIKey:   r.secret("jellyfin.apiKey", "JELLYFIN_API_KEY", jf.APIKey),
		JellyfinUserID:   r.str("jellyfin.userId", "JELLYFIN_USER_ID", jf.UserID, ""),

		PlexDisabled:       r.disabled("plex.enabled", px.Enabled),
		PlexURL:            r.str("plex.url", "PLEX_URL", px.URL, ""),
		PlexToken:          r.secret("plex.token", "PLEX_TOKEN", px.Token),
		PlexLibrarySection: r.str("plex.librarySection", "PLEX_LIBRARY_SECTION", px.LibrarySection, ""),

		StreamingDisabled: r.disabled("streaming.enabled", st.Enabled),
		TMDBReadToken:     r.secret("streaming.tmdbReadToken", "TMDB_READ_TOKEN", st.TMDBReadToken),
		TMDBWatchRegion:   r.str("streaming.watchRegion", "TMDB_WATCH_REGION", st.WatchRegion, defaultWatchRegion),

		PublicURL:  r.str("publicUrl", "PUBLIC_URL", top.PublicURL, "http://localhost:8080"),
		SessionTTL: r.str("sessionTtl", "SESSION_TTL", top.SessionTTL, "4h"),

		// CacheDir joins PORT and CONFIG_FILE as environment-only. It names a
		// path inside the container, so a new value has to be mounted before it
		// can be used — which means editing the deployment and recreating it
		// anyway. Offering it in a form would imply a change that a save alone
		// can never make good on.
		CacheDir:    os.Getenv("CACHE_DIR"),
		Port:        os.Getenv("PORT"),
		tmdbBaseURL: tmdbAPIBase,
	}
	// PLEX_LIBRARY_SECTION is listed after the plural so the current name wins
	// when both are set, and so the deprecation notice names the right one.
	cfg.JellyfinLibraries = r.libraries("jellyfin.libraries", jf.Libraries, "JELLYFIN_LIBRARIES")
	cfg.PlexLibraries = r.libraries("plex.libraries", px.Libraries,
		"PLEX_LIBRARY_SECTIONS", "PLEX_LIBRARY_SECTION")

	// A streaming section in the file means the deployment manages streaming
	// through the settings screen, which changes what an absent provider list
	// means; see resolver.providers.
	streamingManaged := file != nil && file.Streaming != nil
	cfg.StreamingProviders = r.providers("streaming.providers", "STREAMING_PROVIDERS", st.Providers, streamingManaged)
	cfg.TMDBWatchRegion = normalizeRegion(cfg.TMDBWatchRegion)
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(os.TempDir(), "mns-posters")
	}
	cfg.Provenance = r.prov
	return cfg
}

// logResolvedConfig states where every setting came from.
func logResolvedConfig(cfg Config) {
	logProvenance(cfg.Provenance, map[string]string{
		"publicUrl": cfg.PublicURL, "sessionTtl": cfg.SessionTTL,
		"jellyfin.enabled": strconv.FormatBool(!cfg.JellyfinDisabled), "jellyfin.url": cfg.JellyfinURL,
		"jellyfin.apiKey": maskSecret(cfg.JellyfinAPIKey), "jellyfin.userId": cfg.JellyfinUserID,
		"jellyfin.libraries": describeLibraries(cfg.JellyfinLibraries),
		"plex.enabled":       strconv.FormatBool(!cfg.PlexDisabled), "plex.url": cfg.PlexURL,
		"plex.token": maskSecret(cfg.PlexToken), "plex.librarySection": cfg.PlexLibrarySection,
		"plex.libraries":    describeLibraries(cfg.PlexLibraries),
		"streaming.enabled": strconv.FormatBool(!cfg.StreamingDisabled), "streaming.tmdbReadToken": maskSecret(cfg.TMDBReadToken),
		"streaming.watchRegion": cfg.TMDBWatchRegion, "streaming.providers": strings.Join(cfg.StreamingProviders, ","),
	})
}

// normalizeRegion puts a watch region into the form TMDB expects. It is a
// function rather than an inline ToUpper so resolution and the settings
// endpoints cannot disagree about what "us" means.
func normalizeRegion(region string) string {
	return strings.ToUpper(strings.TrimSpace(region))
}

// JellyfinConfigured reports whether this deployment can query Jellyfin. Both
// values are needed: a URL without a key cannot authenticate, and a key without
// a URL has nowhere to go.
func (c Config) JellyfinConfigured() bool {
	return !c.JellyfinDisabled && c.JellyfinURL != "" && c.JellyfinAPIKey != ""
}

// PlexConfigured reports whether this deployment can query Plex. Both values
// are needed for the same reason Jellyfin needs both: a URL without a token
// cannot authenticate, and a token without a URL has nowhere to go.
func (c Config) PlexConfigured() bool {
	return !c.PlexDisabled && c.PlexURL != "" && c.PlexToken != ""
}

// StreamingConfigured reports whether this deployment can query any streaming
// service. Every streaming source goes through TMDB, so the token is required;
// STREAMING_PROVIDERS can also narrow the list to nothing.
func (c Config) StreamingConfigured() bool {
	return !c.StreamingDisabled && c.TMDBReadToken != "" && len(c.StreamingProviders) > 0
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
