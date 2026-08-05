// Package server implements the movie-night backend: session and roster
// management, the WebSocket hub that keeps every device's deck in sync, the
// movie sources it deals from, and the poster image proxy and cache.
package server

import (
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// providerResolveTimeout bounds the one TMDB call made at startup to resolve
// provider names. Startup must not hang on a slow upstream; on timeout the
// built-in table is all that resolves.
const providerResolveTimeout = 10 * time.Second

type Server struct {
	mux *http.ServeMux
	cfg Config
	// Jellyfin is deliberately not a field: it is reachable only through the
	// sources and fetchers maps, like every other source, so no handler can
	// depend on it existing.
	store *Store
	cache *posterCache
	// sources is the live source set, replaced wholesale on a configuration
	// change. It is an atomic pointer rather than three mutex-guarded fields so
	// a request loads one consistent view and never straddles a reload.
	sources atomic.Pointer[sourceSet]
	// providers caches TMDB's per-region provider list for the settings
	// screen's picker.
	providers *providerCache
	// cfgMu guards cfg, which a configuration save replaces. Readers of cfg
	// take it; the source set has its own atomic pointer.
	cfgMu sync.RWMutex
	// setupToken authorizes configuration changes. It is generated on first
	// start and printed to the log, which is the only delivery channel an
	// application without accounts has.
	setupToken string
	version    string
	commit     string
}

func New(cfg Config) *Server {
	ttl, err := time.ParseDuration(cfg.SessionTTL)
	if err != nil {
		log.Printf("server: invalid SESSION_TTL %q, defaulting to 4h: %v", cfg.SessionTTL, err)
		ttl = 4 * time.Hour
	}

	s := &Server{
		mux:       http.NewServeMux(),
		cfg:       cfg,
		store:     NewStore(ttl),
		cache:     newPosterCache(cfg.CacheDir),
		providers: newProviderCache(),
	}
	s.setupToken = ensureSetupToken(cfg.ConfigPath)
	logSetupToken(s.setupToken)
	s.sources.Store(buildSourceSet(cfg))

	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

// SetBuildInfo records the version and commit baked in at build time.
func (s *Server) SetBuildInfo(version, commit string) {
	s.version = version
	s.commit = commit
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /api/setup", s.handleSetup)
	s.mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	s.mux.HandleFunc("POST /api/settings", s.handleSetSettings)
	s.mux.HandleFunc("POST /api/settings/verify/tmdb", s.handleVerifyTMDB)
	s.mux.HandleFunc("POST /api/settings/verify/jellyfin", s.handleVerifyJellyfin)
	s.mux.HandleFunc("POST /api/settings/verify/plex", s.handleVerifyPlex)
	s.mux.HandleFunc("POST /api/settings/jellyfin/users", s.handleJellyfinUsers)
	s.mux.HandleFunc("POST /api/settings/providers", s.handleProviderList)
	s.mux.HandleFunc("GET /api/library/preview", s.handleLibraryPreview)
	s.mux.HandleFunc("GET /api/library/filters", s.handleLibraryFilters)
	s.mux.HandleFunc("POST /api/library/warm", s.handleLibraryWarm)
	s.mux.HandleFunc("GET /api/images/{source}/{id}", s.handleImage)
	s.mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /ws", s.handleWS)
	// static handler is registered in main.go via SetStatic (needs the embed.FS)
}
