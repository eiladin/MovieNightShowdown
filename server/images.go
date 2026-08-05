package server

import (
	"net/http"
	"strings"
)

// handleImage serves a movie's primary poster from the on-disk cache, fetching
// from the owning source on a miss. Images are keyed by source + item id +
// image tag (the ?tag= query param). With a tag the response is immutable for a
// year, because changed artwork gets a new tag and therefore a new URL.
//
// Every poster is proxied through here; upstream URLs and credentials are never
// exposed to a client.
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	source := SourceID(r.PathValue("source"))
	id := r.PathValue("id")
	if source == "" || id == "" {
		http.NotFound(w, r)
		return
	}
	// A decoded %2F would otherwise let a caller reach an arbitrary upstream
	// path. Poster ids are a single path segment by construction.
	if strings.Contains(id, "/") || strings.Contains(id, "..") {
		http.NotFound(w, r)
		return
	}
	fetcher, ok := s.currentSources().fetchers[source]
	if !ok {
		http.NotFound(w, r)
		return
	}
	tag := r.URL.Query().Get("tag")

	data, err := s.cache.ensure(r.Context(), fetcher, source, id, tag)
	if err != nil {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", http.DetectContentType(data))
	if tag != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
