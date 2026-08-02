package server

import (
	"path/filepath"
	"testing"
)

func TestPosterCache_StoreGetPrune(t *testing.T) {
	dir := t.TempDir()
	c := newPosterCache(dir)
	if !c.enabled() {
		t.Fatal("expected cache enabled")
	}
	id := "abc123"
	c.store(SourceJellyfin, id, "tag1", []byte("image-one"))
	data, ok := c.get(SourceJellyfin, id, "tag1")
	if !ok || string(data) != "image-one" {
		t.Fatalf("get after store: ok=%v data=%q", ok, data)
	}
	c.store(SourceJellyfin, id, "tag2", []byte("image-two"))
	if _, ok := c.get(SourceJellyfin, id, "tag1"); ok {
		t.Error("old tag1 file should have been pruned")
	}
	if data, ok := c.get(SourceJellyfin, id, "tag2"); !ok || string(data) != "image-two" {
		t.Fatalf("get tag2: ok=%v data=%q", ok, data)
	}
}

func TestPosterCache_Disabled(t *testing.T) {
	c := newPosterCache("")
	if c.enabled() {
		t.Fatal("empty dir should be disabled")
	}
	if _, ok := c.get(SourceJellyfin, "id", "tag"); ok {
		t.Error("disabled cache should always miss")
	}
}

func TestParsePosterRef(t *testing.T) {
	cases := []struct {
		in     string
		source SourceID
		id     string
		tag    string
	}{
		{"/api/images/jellyfin/abc123?tag=deadbeef", SourceJellyfin, "abc123", "deadbeef"},
		{"/api/images/jellyfin/abc123", SourceJellyfin, "abc123", ""},
		{"/api/images/netflix/_xyz.jpg?tag=", SourceNetflix, "_xyz.jpg", ""},
		{"/api/images/abc123", "", "", ""},
		{"/other/jellyfin/abc123", "", "", ""},
		{"", "", "", ""},
	}
	for _, c := range cases {
		source, id, tag := parsePosterRef(c.in)
		if source != c.source || id != c.id || tag != c.tag {
			t.Fatalf("parsePosterRef(%q) = (%q,%q,%q), want (%q,%q,%q)",
				c.in, source, id, tag, c.source, c.id, c.tag)
		}
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("../../etc/passwd"); got == "../../etc/passwd" {
		t.Errorf("sanitize did not strip separators: %q", got)
	}
}

// TestPosterCache_SourcesDoNotCollide guards the source component of the cache
// key: two sources can legitimately use the same item id (a TMDB id, say), and
// each must keep its own bytes.
func TestPosterCache_SourcesDoNotCollide(t *testing.T) {
	c := newPosterCache(t.TempDir())
	const id, tag = "603", "tag1"
	c.store(SourceJellyfin, id, tag, []byte("jellyfin-image"))
	c.store(SourceNetflix, id, tag, []byte("netflix-image"))

	if data, ok := c.get(SourceJellyfin, id, tag); !ok || string(data) != "jellyfin-image" {
		t.Errorf("jellyfin get: ok=%v data=%q", ok, data)
	}
	if data, ok := c.get(SourceNetflix, id, tag); !ok || string(data) != "netflix-image" {
		t.Errorf("netflix get: ok=%v data=%q", ok, data)
	}
}

// TestPosterCache_PruneOldIsScopedToSource pins the pruneOld glob to a single
// source. An unscoped glob would let one source's poster refresh evict another
// source's cached poster for the same id.
func TestPosterCache_PruneOldIsScopedToSource(t *testing.T) {
	c := newPosterCache(t.TempDir())
	const id = "603"
	c.store(SourceJellyfin, id, "tag1", []byte("jellyfin-image"))
	c.store(SourceNetflix, id, "tag1", []byte("netflix-image"))

	// Netflix refreshes its artwork, pruning its own stale tag1 file.
	c.store(SourceNetflix, id, "tag2", []byte("netflix-image-v2"))

	if _, ok := c.get(SourceNetflix, id, "tag1"); ok {
		t.Error("netflix tag1 should have been pruned")
	}
	if data, ok := c.get(SourceJellyfin, id, "tag1"); !ok || string(data) != "jellyfin-image" {
		t.Errorf("jellyfin tag1 was evicted by a netflix prune: ok=%v data=%q", ok, data)
	}
}

// TestPosterCache_PathStaysInDir checks that hostile source ids and tags cannot
// walk out of the cache directory: sanitize must fold every separator away.
func TestPosterCache_PathStaysInDir(t *testing.T) {
	dir := t.TempDir()
	c := newPosterCache(dir)
	cases := []struct {
		source SourceID
		id     string
		tag    string
	}{
		{"../../etc", "passwd", "tag"},
		{SourceJellyfin, "../../etc/passwd", "tag"},
		{SourceJellyfin, "abc", "../../etc/passwd"},
	}
	for _, tc := range cases {
		p := c.path(tc.source, tc.id, tc.tag)
		if filepath.Dir(p) != dir {
			t.Errorf("path(%q,%q,%q) = %q, want a file directly in %q", tc.source, tc.id, tc.tag, p, dir)
		}
		c.store(tc.source, tc.id, tc.tag, []byte("payload"))
		if data, ok := c.get(tc.source, tc.id, tc.tag); !ok || string(data) != "payload" {
			t.Errorf("round trip for (%q,%q,%q): ok=%v data=%q", tc.source, tc.id, tc.tag, ok, data)
		}
	}
}
