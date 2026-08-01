package server

import "testing"

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
