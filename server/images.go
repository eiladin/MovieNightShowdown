package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// handleImage proxies a movie's primary poster image from Jellyfin so the
// API key never reaches the browser and clients never talk to Jellyfin
// directly.
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	reqURL := fmt.Sprintf("%s/Items/%s/Images/Primary?maxWidth=600", s.jellyfin.baseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, reqURL, nil)
	if err != nil {
		http.Error(w, "failed to build image request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("X-Emby-Token", s.jellyfin.apiKey)

	resp, err := s.jellyfin.http.Do(req)
	if err != nil {
		http.Error(w, "failed to fetch image", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
}
