package server

import "testing"

func TestPosterCache_StoreGetPrune(t *testing.T) {
	dir := t.TempDir()
	c := newPosterCache(dir)
	if !c.enabled() {
		t.Fatal("expected cache enabled")
	}
	id := "abc123"
	c.store(id, "tag1", []byte("image-one"))
	data, ok := c.get(id, "tag1")
	if !ok || string(data) != "image-one" {
		t.Fatalf("get after store: ok=%v data=%q", ok, data)
	}
	c.store(id, "tag2", []byte("image-two"))
	if _, ok := c.get(id, "tag1"); ok {
		t.Error("old tag1 file should have been pruned")
	}
	if data, ok := c.get(id, "tag2"); !ok || string(data) != "image-two" {
		t.Fatalf("get tag2: ok=%v data=%q", ok, data)
	}
}

func TestPosterCache_Disabled(t *testing.T) {
	c := newPosterCache("")
	if c.enabled() {
		t.Fatal("empty dir should be disabled")
	}
	if _, ok := c.get("id", "tag"); ok {
		t.Error("disabled cache should always miss")
	}
}

func TestParsePosterRef(t *testing.T) {
	id, tag := parsePosterRef("/api/images/abc123?tag=deadbeef")
	if id != "abc123" || tag != "deadbeef" {
		t.Fatalf("got id=%q tag=%q", id, tag)
	}
	id, tag = parsePosterRef("/api/images/xyz")
	if id != "xyz" || tag != "" {
		t.Fatalf("got id=%q tag=%q", id, tag)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("../../etc/passwd"); got == "../../etc/passwd" {
		t.Errorf("sanitize did not strip separators: %q", got)
	}
}
