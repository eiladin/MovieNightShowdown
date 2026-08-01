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

	sources := selectSources(s.sources, filters.Sources)
	movies, failed, err := gatherShoe(r.Context(), sources, filters)
	if err != nil {
		log.Printf("library preview: %v", err)
		http.Error(w, "failed to query any selected source", http.StatusBadGateway)
		return
	}
	for _, f := range failed {
		log.Printf("library preview: source %s unavailable", f)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(libraryPreviewResponse{Count: len(movies), Movies: movies})
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

	sources := selectSources(s.sources, filters.Sources)
	movies, _, err := gatherShoe(r.Context(), sources, filters)
	if err != nil {
		log.Printf("library warm: %v", err)
		http.Error(w, "failed to query any selected source", http.StatusBadGateway)
		return
	}

	if s.cache.enabled() {
		go s.cache.warm(movies, s.fetchers)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"count": len(movies)})
}
