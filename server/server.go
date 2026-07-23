package server

import "net/http"

type Server struct {
	mux      *http.ServeMux
	cfg      Config
	jellyfin *JellyfinClient
}

func New(cfg Config) *Server {
	s := &Server{mux: http.NewServeMux(), cfg: cfg, jellyfin: NewJellyfinClient(cfg)}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /api/library/preview", s.handleLibraryPreview)
	s.mux.HandleFunc("GET /api/images/{id}", s.handleImage)
	// static handler is registered in main.go via SetStatic (needs the embed.FS)
}
