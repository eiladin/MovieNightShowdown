package server

import "testing"

// TestJellyfinItemToMovie_ID pins the id namespacing. The TMDB id is the join
// key that lets a library film and the same film from a streaming source merge
// into a single card: if this regresses, the same film appears twice in a deck.
func TestJellyfinItemToMovie_ID(t *testing.T) {
	cases := []struct {
		name string
		id   string
		tmdb string
		want string
	}{
		{"with tmdb id", "abc123", "603", "tmdb:603"},
		{"without tmdb id", "abc123", "", "jf:abc123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var it jellyfinItem
			it.ID = c.id
			it.ProviderIds.Tmdb = c.tmdb
			if got := it.toMovie(SourceJellyfin).ID; got != c.want {
				t.Errorf("ID = %q, want %q", got, c.want)
			}
		})
	}
}

func TestJellyfinItemToMovie_Runtime(t *testing.T) {
	cases := []struct {
		name  string
		ticks int64
		want  int
	}{
		{"zero", 0, 0},
		{"whole minutes", 90 * 60 * 10_000_000, 90},
		// 90m30s truncates down to 90, it does not round up.
		{"partial minute truncates", (90*60 + 30) * 10_000_000, 90},
		{"under a minute", 59 * 10_000_000, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := jellyfinItem{RunTimeTicks: c.ticks}
			if got := it.toMovie(SourceJellyfin).Runtime; got != c.want {
				t.Errorf("Runtime = %d, want %d", got, c.want)
			}
		})
	}
}

func TestJellyfinItemToMovie_PosterURL(t *testing.T) {
	cases := []struct {
		name string
		id   string
		tags map[string]string
		want string
	}{
		{"no image tags", "abc123", nil, "/api/images/jellyfin/abc123"},
		{"empty primary tag", "abc123", map[string]string{"Primary": ""}, "/api/images/jellyfin/abc123"},
		{"other tag only", "abc123", map[string]string{"Backdrop": "xyz"}, "/api/images/jellyfin/abc123"},
		{"primary tag", "abc123", map[string]string{"Primary": "deadbeef"}, "/api/images/jellyfin/abc123?tag=deadbeef"},
		{"tag needing escaping", "abc123", map[string]string{"Primary": "a b&c=d"}, "/api/images/jellyfin/abc123?tag=a+b%26c%3Dd"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := jellyfinItem{ID: c.id, ImageTags: c.tags}
			if got := it.toMovie(SourceJellyfin).PosterURL; got != c.want {
				t.Errorf("PosterURL = %q, want %q", got, c.want)
			}
		})
	}
}

// TestJellyfinItemToMovie_PosterURLRoundTrip checks that the URL toMovie builds
// is the URL parsePosterRef understands. The two must agree: the image proxy
// and the cache warmer both resolve posters by re-parsing this string.
func TestJellyfinItemToMovie_PosterURLRoundTrip(t *testing.T) {
	for _, tag := range []string{"", "deadbeef", "a b&c=d"} {
		it := jellyfinItem{ID: "abc123", ImageTags: map[string]string{"Primary": tag}}
		source, id, gotTag := parsePosterRef(it.toMovie(SourceJellyfin).PosterURL)
		if source != SourceJellyfin || id != "abc123" || gotTag != tag {
			t.Errorf("round trip for tag %q = (%q,%q,%q), want (%q,%q,%q)",
				tag, source, id, gotTag, SourceJellyfin, "abc123", tag)
		}
	}
}

func TestJellyfinItemToMovie_Availability(t *testing.T) {
	m := jellyfinItem{ID: "abc123"}.toMovie(SourceJellyfin)
	if len(m.Availability) != 1 {
		t.Fatalf("Availability has %d entries, want 1", len(m.Availability))
	}
	if a := m.Availability[0]; a.Source != SourceJellyfin || a.Label != "Jellyfin" {
		t.Errorf("Availability[0] = {%q, %q}, want {%q, %q}", a.Source, a.Label, SourceJellyfin, "Jellyfin")
	}
}

func TestJellyfinItemToMovie_Fields(t *testing.T) {
	it := jellyfinItem{
		ID:              "abc123",
		Name:            "A Film",
		ProductionYear:  1999,
		Genres:          []string{"Drama", "Sci-Fi"},
		Overview:        "An overview.",
		CommunityRating: 8.5,
		OfficialRating:  "R",
	}
	m := it.toMovie(SourceJellyfin)
	if m.Title != "A Film" || m.Year != 1999 || m.Overview != "An overview." ||
		m.CommunityRating != 8.5 || m.OfficialRating != "R" {
		t.Errorf("unexpected mapping: %+v", m)
	}
	if len(m.Genres) != 2 || m.Genres[0] != "Drama" || m.Genres[1] != "Sci-Fi" {
		t.Errorf("Genres = %v, want [Drama Sci-Fi]", m.Genres)
	}
}
