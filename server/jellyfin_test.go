package server

import (
	"context"
	"testing"
)

// TestJellyfinClient_Movies is an integration test against a real Jellyfin
// server. It skips automatically unless JELLYFIN_URL/JELLYFIN_API_KEY are
// set in the environment (see docs/TASKS.md 2.2/2.3 Verify).
func TestJellyfinClient_Movies(t *testing.T) {
	cfg := LoadConfig()
	if cfg.JellyfinURL == "" || cfg.JellyfinAPIKey == "" {
		t.Skip("JELLYFIN_URL/JELLYFIN_API_KEY not set; skipping live Jellyfin integration test")
	}

	client := NewJellyfinClient(cfg)
	ctx := context.Background()

	all, allCount, err := client.Movies(ctx, ParseFilters(nil))
	if err != nil {
		t.Fatalf("Movies(no filters): %v", err)
	}
	t.Logf("no filters: %d movies fetched, TotalRecordCount=%d", len(all), allCount)
	if allCount == 0 {
		t.Fatal("expected a non-zero movie count from the real Jellyfin library")
	}
	for _, m := range all {
		if m.PosterURL == "" || m.PosterURL[:12] != "/api/images/" {
			t.Fatalf("movie %q has non-proxied PosterURL %q", m.Title, m.PosterURL)
		}
	}

	filtered, filteredCount, err := client.Movies(ctx, Filters{Genres: []string{"Action"}, MaxMovies: defaultMaxMovies})
	if err != nil {
		t.Fatalf("Movies(genres=Action): %v", err)
	}
	t.Logf("genres=Action: %d movies fetched, TotalRecordCount=%d", len(filtered), filteredCount)

	if filteredCount >= allCount {
		t.Fatalf("expected genres=Action (%d) to reduce the count vs no filter (%d)", filteredCount, allCount)
	}
}
