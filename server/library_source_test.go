package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// jellyfinLibraryStub serves /Items and /Items/Filters per parentId, so a scoped
// query can be told apart from an unscoped one and a scoped vocabulary from a
// server-wide one.
type jellyfinLibraryStub struct {
	*httptest.Server
	mu      sync.Mutex
	queries []url.Values
}

func newJellyfinLibraryStub(t *testing.T, genresByParent map[string][]string) *jellyfinLibraryStub {
	t.Helper()
	ts := &jellyfinLibraryStub{}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.queries = append(ts.queries, r.URL.Query())
		ts.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/Items":
			parent := r.URL.Query().Get("ParentId")
			_, _ = fmt.Fprintf(w, `{"TotalRecordCount":1,"Items":[
			  {"Id":"item-%s","Name":"Film in %s","ProductionYear":2001,
			   "ImageTags":{"Primary":"tag1"},"ProviderIds":{"Tmdb":"555"}}]}`,
				parent, parent)
		case "/Items/Filters":
			genres := genresByParent[r.URL.Query().Get("parentId")]
			quoted := make([]string, 0, len(genres))
			for _, g := range genres {
				quoted = append(quoted, `"`+g+`"`)
			}
			_, _ = fmt.Fprintf(w, `{"Genres":[%s],"OfficialRatings":["PG"]}`,
				strings.Join(quoted, ","))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func (s *jellyfinLibraryStub) lastQueryFor(path string) url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queries) == 0 {
		return nil
	}
	return s.queries[len(s.queries)-1]
}

// --- identity ---

func TestLibraryScopedIdentity(t *testing.T) {
	cases := []struct {
		name    string
		ref     libraryRef
		wantID  SourceID
		wantLbl string
	}{
		{
			// The upgrade case: a deployment that has chosen nothing keeps the id
			// its saved host selections and cached poster URLs already use.
			name:    "no library",
			ref:     libraryRef{},
			wantID:  "jellyfin",
			wantLbl: "Jellyfin",
		},
		{
			name:    "id and name",
			ref:     libraryRef{ID: "abc123", Name: "Kids Movies"},
			wantID:  "jellyfin-abc123",
			wantLbl: "Jellyfin — Kids Movies",
		},
		{
			// Configured by bare identifier: honest about what it knows rather
			// than inventing a label.
			name:    "id only",
			ref:     libraryRef{ID: "abc123"},
			wantID:  "jellyfin-abc123",
			wantLbl: "Jellyfin — abc123",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewJellyfinClient(Config{JellyfinURL: "http://nas:8096", JellyfinAPIKey: "k"}, tc.ref)
			if c.ID() != tc.wantID {
				t.Errorf("ID = %q, want %q", c.ID(), tc.wantID)
			}
			if c.Name() != tc.wantLbl {
				t.Errorf("Name = %q, want %q", c.Name(), tc.wantLbl)
			}
		})
	}
}

// The identifier is the library's own, never a slug of its name: the name gets
// renamed and this value is embedded in the image proxy path and in the host's
// persisted source selection.
func TestLibraryScopedIDIgnoresTheName(t *testing.T) {
	a := libraryScopedID(SourcePlex, libraryRef{ID: "3", Name: "Films"})
	b := libraryScopedID(SourcePlex, libraryRef{ID: "3", Name: "Renamed Films"})
	if a != b {
		t.Errorf("renaming a library changed its id: %q vs %q", a, b)
	}
	if a != "plex-3" {
		t.Errorf("id = %q, want plex-3", a)
	}
}

// --- source set ---

func libraryConfig(jellyfin, plex []libraryRef) Config {
	return Config{
		JellyfinURL:       "http://nas:8096",
		JellyfinAPIKey:    "key",
		PlexURL:           "http://plex:32400",
		PlexToken:         "token",
		JellyfinLibraries: jellyfin,
		PlexLibraries:     plex,
	}
}

func TestBuildSourceSetOneSourcePerLibrary(t *testing.T) {
	cfg := libraryConfig(
		[]libraryRef{{ID: "aaa", Name: "Movies"}, {ID: "bbb", Name: "Kids Movies"}},
		[]libraryRef{{ID: "1", Name: "Films"}},
	)

	set := buildSourceSet(cfg)

	want := []SourceID{"jellyfin-aaa", "jellyfin-bbb", "plex-1"}
	if len(set.order) != len(want) {
		t.Fatalf("order = %v, want %v", set.order, want)
	}
	for i, id := range want {
		if set.order[i] != id {
			t.Errorf("order[%d] = %q, want %q", i, set.order[i], id)
		}
		if _, ok := set.sources[id]; !ok {
			t.Errorf("no source registered for %q", id)
		}
		if _, ok := set.fetchers[id]; !ok {
			t.Errorf("no poster fetcher registered for %q", id)
		}
	}

	// Every library is offered under its qualified name. Two servers pointed at
	// the same media both hold a "Kids Movies", and the picker is one flat list.
	descs := configuredSources(set.sources, set.order)
	labels := make([]string, 0, len(descs))
	for _, d := range descs {
		labels = append(labels, d.Label)
	}
	wantLabels := []string{"Jellyfin — Movies", "Jellyfin — Kids Movies", "Plex — Films"}
	for i, w := range wantLabels {
		if labels[i] != w {
			t.Errorf("label[%d] = %q, want %q", i, labels[i], w)
		}
	}
}

// The upgrade-safety test. A deployment that has never chosen a library gets
// exactly what it had before these settings existed: one source per service, under
// the bare service id.
func TestBuildSourceSetWithNoLibrariesIsUnchanged(t *testing.T) {
	set := buildSourceSet(libraryConfig(nil, nil))

	want := []SourceID{SourceJellyfin, SourcePlex}
	if len(set.order) != len(want) {
		t.Fatalf("order = %v, want %v", set.order, want)
	}
	for i, id := range want {
		if set.order[i] != id {
			t.Errorf("order[%d] = %q, want %q", i, set.order[i], id)
		}
	}
}

// Two entries naming the same library would deal the same movies twice into the
// shoe and leave the picker with two identical chips.
func TestBuildSourceSetSkipsADuplicateLibrary(t *testing.T) {
	set := buildSourceSet(libraryConfig([]libraryRef{{ID: "aaa"}, {ID: "aaa"}}, nil))

	count := 0
	for _, id := range set.order {
		if id == "jellyfin-aaa" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("registered %d sources for one library, want 1", count)
	}
}

// --- scoping ---

func TestJellyfinScopesItsQueryToItsLibrary(t *testing.T) {
	stub := newJellyfinLibraryStub(t, nil)
	c := NewJellyfinClient(Config{JellyfinURL: stub.URL, JellyfinAPIKey: "k"},
		libraryRef{ID: "abc123", Name: "Kids Movies"})

	movies, _, err := c.Movies(context.Background(), Filters{})
	if err != nil {
		t.Fatalf("Movies: %v", err)
	}
	if got := stub.lastQueryFor("/Items").Get("ParentId"); got != "abc123" {
		t.Errorf("ParentId = %q, want the source's library", got)
	}
	// The poster path names this source, not the service, because the proxy looks
	// a fetcher up by exactly that id.
	if want := "/api/images/jellyfin-abc123/"; !strings.HasPrefix(movies[0].PosterURL, want) {
		t.Errorf("PosterURL = %q, want the prefix %q", movies[0].PosterURL, want)
	}
	// The badge stays the bare service name; the qualified form lives on the
	// source descriptor.
	if movies[0].Availability[0].Label != "Jellyfin" {
		t.Errorf("badge label = %q, want the bare service name",
			movies[0].Availability[0].Label)
	}
	if movies[0].Availability[0].Source != "jellyfin-abc123" {
		t.Errorf("availability source = %q, want this library's source",
			movies[0].Availability[0].Source)
	}
}

// A request must not be able to redirect a source at another library. Under one
// source per library the scope is the source's identity.
func TestJellyfinIgnoresARequestedLibraryID(t *testing.T) {
	stub := newJellyfinLibraryStub(t, nil)
	c := NewJellyfinClient(Config{JellyfinURL: stub.URL, JellyfinAPIKey: "k"},
		libraryRef{ID: "mine"})

	if _, _, err := c.Movies(context.Background(), Filters{LibraryID: "someone-elses"}); err != nil {
		t.Fatalf("Movies: %v", err)
	}
	if got := stub.lastQueryFor("/Items").Get("ParentId"); got != "mine" {
		t.Errorf("ParentId = %q, want the source's own library to win", got)
	}
}

func TestJellyfinWithNoLibrarySendsNoParentID(t *testing.T) {
	stub := newJellyfinLibraryStub(t, nil)
	c := NewJellyfinClient(Config{JellyfinURL: stub.URL, JellyfinAPIKey: "k"}, libraryRef{})

	if _, _, err := c.Movies(context.Background(), Filters{}); err != nil {
		t.Fatalf("Movies: %v", err)
	}
	if got := stub.lastQueryFor("/Items"); got.Has("ParentId") {
		t.Errorf("ParentId = %q, want it absent for an unscoped source", got.Get("ParentId"))
	}
}

// The filter-correctness win. An unscoped vocabulary offers a host genres that
// only exist in libraries this source cannot return, so the filter matches
// nothing and the reason is invisible.
func TestJellyfinVocabularyIsScopedToItsLibrary(t *testing.T) {
	stub := newJellyfinLibraryStub(t, map[string][]string{
		"":     {"Comedy", "Documentary", "Horror"},
		"kids": {"Animation", "Family"},
	})
	c := NewJellyfinClient(Config{JellyfinURL: stub.URL, JellyfinAPIKey: "k"},
		libraryRef{ID: "kids", Name: "Kids Movies"})

	got, err := c.Vocabulary(context.Background())
	if err != nil {
		t.Fatalf("Vocabulary: %v", err)
	}
	want := []string{"Animation", "Family"}
	if len(got.Genres) != len(want) {
		t.Fatalf("genres = %v, want %v", got.Genres, want)
	}
	for i, w := range want {
		if got.Genres[i] != w {
			t.Errorf("genre[%d] = %q, want %q", i, got.Genres[i], w)
		}
	}
}

// --- reload tiering ---

func TestSourcesDifferOnALibraryChange(t *testing.T) {
	base := libraryConfig([]libraryRef{{ID: "aaa", Name: "Movies"}}, nil)

	same := libraryConfig([]libraryRef{{ID: "aaa", Name: "Movies"}}, nil)
	if sourcesDiffer(base, same) {
		t.Error("identical library lists reported as different; a reformatted config " +
			"file would end every session")
	}

	added := libraryConfig([]libraryRef{{ID: "aaa", Name: "Movies"}, {ID: "bbb"}}, nil)
	if !sourcesDiffer(base, added) {
		t.Error("an added library was not reported; the save would claim a change it did not make")
	}

	renamed := libraryConfig([]libraryRef{{ID: "aaa", Name: "Films"}}, nil)
	if !sourcesDiffer(base, renamed) {
		t.Error("a renamed library was not reported; the picker would keep the stale label")
	}

	// Order decides the canonical source order, which decides whose genre names
	// win in the merged vocabulary. A reordered list is a different configuration.
	reordered := libraryConfig([]libraryRef{{ID: "bbb"}, {ID: "aaa", Name: "Movies"}}, nil)
	if !sourcesDiffer(added, reordered) {
		t.Error("a reordered library list was not reported")
	}
}

// A film in two libraries is one card carrying both availabilities. Two sources
// against the same server is the normal case once libraries are split out, and a
// deck full of duplicates would be the obvious way to get it wrong.
func TestOneFilmInTwoLibrariesMergesIntoOneCard(t *testing.T) {
	stub := newJellyfinLibraryStub(t, nil)
	cfg := Config{JellyfinURL: stub.URL, JellyfinAPIKey: "k"}
	a := NewJellyfinClient(cfg, libraryRef{ID: "aaa"})
	b := NewJellyfinClient(cfg, libraryRef{ID: "bbb"})

	moviesA, err := a.Search(context.Background(), Filters{})
	if err != nil {
		t.Fatalf("search a: %v", err)
	}
	moviesB, err := b.Search(context.Background(), Filters{})
	if err != nil {
		t.Fatalf("search b: %v", err)
	}

	// Both stub items carry the same TMDB id, which is the join key.
	merged := MergeMovies(moviesA, moviesB)
	if len(merged) != 1 {
		t.Fatalf("merged into %d cards, want 1", len(merged))
	}
	if len(merged[0].Availability) != 2 {
		t.Errorf("availability = %+v, want one entry per library", merged[0].Availability)
	}
}
