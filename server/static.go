package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// SetStatic registers the SPA handler using the embedded dist filesystem.
// dist must be the sub-filesystem rooted at the built web/dist directory.
func (s *Server) SetStatic(dist fs.FS) {
	fileServer := http.FileServer(http.FS(dist))
	s.mux.Handle("/", spaFallback(dist, fileServer))
}

// spaFallback serves the requested file if it exists, otherwise index.html
// (so client-side routes like /join/ABCD work on refresh).
func spaFallback(dist fs.FS, fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			// not a real file -> serve index.html for the SPA router
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
