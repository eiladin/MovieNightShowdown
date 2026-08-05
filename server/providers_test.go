package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubProviderList serves TMDB's watch-provider list and records how many times
// it was asked, so tests can assert the network is used only when needed.
func stubProviderList(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"provider_id": 8, "provider_name": "Netflix"},
				{"provider_id": 43, "provider_name": "Starz"},
				{"provider_id": 1899, "provider_name": "HBO Max"},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func resolveConfig(baseURL string, requested ...string) Config {
	return Config{
		TMDBReadToken:      "token",
		TMDBWatchRegion:    defaultWatchRegion,
		tmdbBaseURL:        baseURL,
		StreamingProviders: requested,
	}
}

// The built-in table exists so the common configuration costs no network call
// at startup, and so the ids this app shipped with never change.
func TestResolveUsesBuiltInTableWithoutNetwork(t *testing.T) {
	calls := 0
	srv := stubProviderList(t, &calls)
	cfg := resolveConfig(srv.URL, defaultStreamingProviders...)

	got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)

	if calls != 0 {
		t.Fatalf("expected no TMDB call for table-only entries, got %d", calls)
	}
	want := []StreamingProvider{
		{ID: SourceNetflix, Name: "Netflix", TMDBID: 8},
		{ID: SourcePrime, Name: "Prime Video", TMDBID: 9},
		{ID: SourceDisney, Name: "Disney+", TMDBID: 337},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// Anything outside the table is resolved against TMDB by name.
func TestResolveLooksUpUnknownNamesInTMDB(t *testing.T) {
	calls := 0
	srv := stubProviderList(t, &calls)
	cfg := resolveConfig(srv.URL, "netflix", "starz")

	got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)

	if calls != 1 {
		t.Fatalf("expected exactly one TMDB call, got %d", calls)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want two providers", got)
	}
	if got[1].ID != "starz" || got[1].Name != "Starz" || got[1].TMDBID != 43 {
		t.Fatalf("starz resolved to %+v", got[1])
	}
}

// A multi-word provider is matched by its name and by the slug of that name,
// and every spelling lands on one identifier.
//
// The id is the one knownProviders pins for that TMDB id, not the slug of TMDB's
// name for it: provider 1899 is "max" here and "HBO Max" upstream. Resolving the
// slug of the upstream name instead would give one service two ids depending on
// how it was configured, and two ids never merge into one deck entry.
func TestResolveMatchesNameAndSlug(t *testing.T) {
	calls := 0
	srv := stubProviderList(t, &calls)

	for _, entry := range []string{"hbo max", "hbo-max", "max"} {
		cfg := resolveConfig(srv.URL, entry)
		got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)
		if len(got) != 1 {
			t.Fatalf("%q resolved to %+v, want one provider", entry, got)
		}
		if got[0].ID != "max" || got[0].TMDBID != 1899 || got[0].Name != "HBO Max" {
			t.Fatalf("%q resolved to %+v, want the pinned id \"max\"", entry, got[0])
		}
	}
}

// A slug written into a config file before the built-in table was consulted must
// keep resolving. It is the id an earlier build of the settings screen wrote, and
// an upgrade that silently stopped resolving it would drop a source the operator
// selected.
func TestResolveAcceptsTheUpstreamSlugAsAnAlias(t *testing.T) {
	calls := 0
	srv := stubProviderList(t, &calls)
	cfg := resolveConfig(srv.URL, "hbo-max")

	got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)

	if len(got) != 1 || got[0].TMDBID != 1899 {
		t.Fatalf("got %+v, want the HBO Max provider", got)
	}
	if got[0].ID != "max" {
		t.Errorf("id = %q, want the canonical \"max\" rather than the alias", got[0].ID)
	}
}

// Two providers whose names differ only in punctuation reduce to one slug. Only
// one may claim it: the picker keys its rows by id, and resolution can map an id
// to a single provider, so offering both promises a choice the server cannot
// keep. The winner is the one that sorts first by name, which is stable across
// calls even though TMDB's array order is not.
func TestProviderCatalogGivesACollidingSlugToOneProvider(t *testing.T) {
	// Neither id is in knownProviders, so nothing is pinned and both fall back
	// to the slug of their upstream name.
	list := tmdbProviderList{}
	list.Results = append(list.Results,
		struct {
			ProviderID   int    `json:"provider_id"`
			ProviderName string `json:"provider_name"`
		}{526, "AMC+"},
		struct {
			ProviderID   int    `json:"provider_id"`
			ProviderName string `json:"provider_name"`
		}{80, "AMC"},
	)

	cat := newProviderCatalog(list)

	seen := map[SourceID]int{}
	for _, o := range cat.options {
		seen[o.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("id %q appears %d times in the offered options", id, n)
		}
	}
	if got, ok := cat.bySlug["amc"]; !ok || got.TMDBID != 80 {
		t.Errorf("bySlug[amc] = %+v, want AMC (80) — the name that sorts first", got)
	}
	// The loser is not erased. It is still selectable, just not by the shared
	// slug, so a deployment that wants it has a way to say so.
	if got, ok := cat.lookup("amc+"); !ok || got.TMDBID != 526 {
		t.Errorf("lookup(\"amc+\") = %+v, want AMC+ (526) by its exact name", got)
	}
	if got, ok := cat.byID[526]; !ok || got.Name != "AMC+" {
		t.Errorf("byID[526] = %+v, want AMC+", got)
	}
}

// A numeric entry is a TMDB provider id, which is what the Discover query
// actually needs.
func TestResolveAcceptsNumericProviderIDs(t *testing.T) {
	calls := 0
	srv := stubProviderList(t, &calls)
	cfg := resolveConfig(srv.URL, "43")

	got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)

	if len(got) != 1 || got[0].TMDBID != 43 || got[0].Name != "Starz" {
		t.Fatalf("got %+v, want the Starz provider", got)
	}
}

// An id TMDB does not list for this region is still offered: the id is all the
// query needs, and refusing it would second-guess the operator.
func TestResolveKeepsUnlistedNumericID(t *testing.T) {
	calls := 0
	srv := stubProviderList(t, &calls)
	cfg := resolveConfig(srv.URL, "12345")

	got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)

	if len(got) != 1 || got[0].TMDBID != 12345 || got[0].ID != "tmdb-12345" {
		t.Fatalf("got %+v, want an unnamed provider for id 12345", got)
	}
}

// A name TMDB has never heard of is skipped, not fatal.
func TestResolveSkipsUnknownNames(t *testing.T) {
	calls := 0
	srv := stubProviderList(t, &calls)
	cfg := resolveConfig(srv.URL, "netflix", "not-a-real-service")

	got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)

	if len(got) != 1 || got[0].ID != SourceNetflix {
		t.Fatalf("got %+v, want netflix alone", got)
	}
}

// A TMDB outage at startup must not cost a deployment the sources the built-in
// table can satisfy.
func TestResolveFallsBackToTableWhenTMDBFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	cfg := resolveConfig(srv.URL, "hulu", "peacock", "starz")

	got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)

	if len(got) != 2 {
		t.Fatalf("got %+v, want the two table entries", got)
	}
	if got[0].ID != "hulu" || got[1].ID != "peacock" {
		t.Fatalf("got %+v, want hulu and peacock", got)
	}
}

// Without a token nothing is resolved, so no streaming source can exist.
func TestResolveReturnsNothingWithoutToken(t *testing.T) {
	cfg := resolveConfig("http://unused", "netflix")
	cfg.TMDBReadToken = ""

	if got := resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

// The region reaches the provider-list query: which services exist depends on
// it.
func TestResolveSendsConfiguredRegion(t *testing.T) {
	var gotRegion, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRegion = r.URL.Query().Get("watch_region")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	t.Cleanup(srv.Close)
	cfg := resolveConfig(srv.URL, "starz")
	cfg.TMDBWatchRegion = "GB"

	resolveStreamingProviders(context.Background(), cfg, cfg.StreamingProviders)

	if gotRegion != "GB" {
		t.Fatalf("watch_region = %q, want GB", gotRegion)
	}
	if gotAuth != "Bearer token" {
		t.Fatalf("Authorization = %q, want the bearer token", gotAuth)
	}
}

func TestSlugifyProvider(t *testing.T) {
	cases := map[string]string{
		"Netflix":            "netflix",
		"HBO Max":            "hbo-max",
		"Disney+":            "disney",
		"Apple TV+":          "apple-tv",
		"Amazon Prime Video": "amazon-prime-video",
		"  Spaced  Out  ":    "spaced-out",
		"+++":                "",
	}
	for in, want := range cases {
		if got := slugifyProvider(in); got != want {
			t.Fatalf("slugifyProvider(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every built-in id must survive being put in a URL path unchanged.
func TestKnownProviderSlugsAreURLSafe(t *testing.T) {
	seen := map[SourceID]bool{SourceJellyfin: true}
	for _, p := range knownProviders {
		if slugifyProvider(string(p.slug)) != string(p.slug) {
			t.Fatalf("built-in slug %q is not URL-safe", p.slug)
		}
		if seen[p.slug] {
			t.Fatalf("built-in slug %q is duplicated", p.slug)
		}
		seen[p.slug] = true
	}
}
