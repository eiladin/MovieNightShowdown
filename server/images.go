package server

import "net/http"

// handleImage serves a movie's primary poster from the on-disk cache, fetching
// from Jellyfin on a miss. Images are keyed by item id + Primary image tag
// (the ?tag= query param). With a tag the response is immutable for a year,
// because changed artwork gets a new tag and therefore a new URL.
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	tag := r.URL.Query().Get("tag")

	data, err := s.cache.ensure(r.Context(), s.jellyfin, id, tag)
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
