package server

import (
	"log"
	"net/http"
	"time"
)

type Server struct {
	mux      *http.ServeMux
	cfg      Config
	jellyfin *JellyfinClient
	store    *Store
}

func New(cfg Config) *Server {
	ttl, err := time.ParseDuration(cfg.SessionTTL)
	if err != nil {
		log.Printf("server: invalid SESSION_TTL %q, defaulting to 4h: %v", cfg.SessionTTL, err)
		ttl = 4 * time.Hour
	}

	s := &Server{
		mux:      http.NewServeMux(),
		cfg:      cfg,
		jellyfin: NewJellyfinClient(cfg),
		store:    NewStore(ttl),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /api/library/preview", s.handleLibraryPreview)
	s.mux.HandleFunc("GET /api/images/{id}", s.handleImage)
	s.mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /ws", s.handleWS)
	// static handler is registered in main.go via SetStatic (needs the embed.FS)
}
