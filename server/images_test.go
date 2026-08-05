package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeFetcher records whether it was asked for a poster, so a test can assert
// that a rejected request never reached an upstream source.
type fakeFetcher struct {
	calls int
	data  []byte
	err   error
}

func (f *fakeFetcher) fetchPoster(ctx context.Context, id, tag string) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

// newImageTestServer builds a Server with exactly one registered fetcher and a
// disabled cache, so handleImage's dispatch is what the test observes.
func newImageTestServer(source SourceID, f PosterFetcher) *Server {
	s := &Server{cache: newPosterCache("")}
	s.sources.Store(&sourceSet{
		sources:  map[SourceID]MovieSource{},
		fetchers: map[SourceID]PosterFetcher{source: f},
	})
	return s
}

func doImageRequest(s *Server, source, id, query string) *httptest.ResponseRecorder {
	target := "/api/images/" + source + "/" + id + query
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetPathValue("source", source)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.handleImage(rec, req)
	return rec
}

func TestHandleImage_RegisteredSource(t *testing.T) {
	f := &fakeFetcher{data: []byte("poster-bytes")}
	s := newImageTestServer(SourceJellyfin, f)

	rec := doImageRequest(s, "jellyfin", "abc123", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "poster-bytes" {
		t.Errorf("body = %q, want %q", got, "poster-bytes")
	}
	if f.calls != 1 {
		t.Errorf("fetcher called %d times, want 1", f.calls)
	}
}

// TestHandleImage_RejectsUnknownSource is the security-relevant case: a request
// naming a source this deployment did not register must be refused outright,
// never fanned out to whichever fetcher happens to be registered (which would
// attach that source's credentials to an attacker-chosen request).
func TestHandleImage_RejectsUnknownSource(t *testing.T) {
	cases := []struct {
		name   string
		source string
		id     string
	}{
		{"unregistered source", "netflix", "abc123"},
		{"empty source", "", "abc123"},
		{"empty id", "jellyfin", ""},
		{"id with slash", "jellyfin", "abc/../secret"},
		{"id with dot dot", "jellyfin", "..abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeFetcher{data: []byte("poster-bytes")}
			s := newImageTestServer(SourceJellyfin, f)

			rec := doImageRequest(s, c.source, c.id, "")
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
			if f.calls != 0 {
				t.Errorf("registered fetcher was called %d times; it must not be reached", f.calls)
			}
		})
	}
}

func TestHandleImage_CacheControl(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"tagged is immutable for a year", "?tag=deadbeef", "public, max-age=31536000, immutable"},
		{"untagged is short lived", "", "public, max-age=86400"},
		{"empty tag is short lived", "?tag=", "public, max-age=86400"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newImageTestServer(SourceJellyfin, &fakeFetcher{data: []byte("poster-bytes")})
			rec := doImageRequest(s, "jellyfin", "abc123", c.query)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("Cache-Control"); got != c.want {
				t.Errorf("Cache-Control = %q, want %q", got, c.want)
			}
		})
	}
}
