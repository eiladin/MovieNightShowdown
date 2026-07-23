package server

import (
	"encoding/json"
	"log"
	"net/http"
)

// libraryPreviewResponse is the JSON body of GET /api/library/preview.
type libraryPreviewResponse struct {
	Count  int     `json:"count"`
	Movies []Movie `json:"movies"`
}

// handleLibraryPreview lets the host preview the filtered Jellyfin library
// (count + a capped list of movies for poster thumbnails) before starting a
// session.
func (s *Server) handleLibraryPreview(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query())

	movies, count, err := s.jellyfin.Movies(r.Context(), filters)
	if err != nil {
		log.Printf("library preview: %v", err)
		http.Error(w, "failed to query Jellyfin library", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(libraryPreviewResponse{Count: count, Movies: movies})
}

// handleLibraryFilters fetches the available filter options (genres, ratings)
// from the Jellyfin library.
func (s *Server) handleLibraryFilters(w http.ResponseWriter, r *http.Request) {
	filters, err := s.jellyfin.GetAvailableFilters(r.Context())
	if err != nil {
		log.Printf("library filters: %v", err)
		http.Error(w, "failed to fetch available filters from Jellyfin", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(filters)
}

// handleLibraryWarm pre-fetches every poster for the filtered library into the
// on-disk cache so the session starts warm. It returns the candidate count
// immediately and warms in the background.
func (s *Server) handleLibraryWarm(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query())

	movies, count, err := s.jellyfin.Movies(r.Context(), filters)
	if err != nil {
		log.Printf("library warm: %v", err)
		http.Error(w, "failed to query Jellyfin library", http.StatusBadGateway)
		return
	}

	if s.cache.enabled() {
		go s.cache.warm(movies, s.jellyfin)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"count": count})
}
