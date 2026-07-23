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

// handleLibraryPreview lets the admin preview the filtered Jellyfin library
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
