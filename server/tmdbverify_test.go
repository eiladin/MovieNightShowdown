package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const tmdbProviderListBody = `{"results":[
  {"provider_id":8,"provider_name":"Netflix"},
  {"provider_id":337,"provider_name":"Disney Plus"},
  {"provider_id":9,"provider_name":"Amazon Prime Video"}
]}`

// newTMDBStub serves the watch-provider endpoint, rejecting any token other
// than the one given, and counts how many requests reached it.
func newTMDBStub(t *testing.T, validToken string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tmdbProviderListBody))
	}))
	t.Cleanup(ts.Close)
	return ts, &calls
}

// newVerifyServer builds a server whose TMDB base URL points at a stub.
func newVerifyServer(t *testing.T, baseURL string) (*Server, string) {
	t.Helper()
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("publicUrl: http://nas:8080\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CACHE_DIR", t.TempDir())
	cfg, err := resolveConfigAt(path, false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	cfg.tmdbBaseURL = baseURL
	s := New(cfg)
	return s, s.setupToken
}

func postJSON(t *testing.T, s *Server, target, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(string(raw)))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestVerifyTMDBAcceptsAGoodToken(t *testing.T) {
	stub, _ := newTMDBStub(t, "good-token")
	s, setup := newVerifyServer(t, stub.URL)

	rec := postJSON(t, s, "/api/settings/verify/tmdb", setup, verifyTMDBRequest{Token: "good-token"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got verifyTMDBResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Valid {
		t.Errorf("valid = false, want true (message %q)", got.Message)
	}
}

func TestVerifyTMDBRejectsABadToken(t *testing.T) {
	stub, _ := newTMDBStub(t, "good-token")
	s, setup := newVerifyServer(t, stub.URL)

	rec := postJSON(t, s, "/api/settings/verify/tmdb", setup, verifyTMDBRequest{Token: "bad-token"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got verifyTMDBResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Valid {
		t.Error("valid = true for a token the upstream rejected")
	}
}

// TestVerifyTMDBNeverEchoesTheToken guards the obvious mistake in a route whose
// whole job is to receive a credential.
func TestVerifyTMDBNeverEchoesTheToken(t *testing.T) {
	stub, _ := newTMDBStub(t, "good-token")
	s, setup := newVerifyServer(t, stub.URL)

	for _, token := range []string{"good-token", "CANDIDATE-SECRET-VALUE"} {
		rec := postJSON(t, s, "/api/settings/verify/tmdb", setup, verifyTMDBRequest{Token: token})
		if strings.Contains(rec.Body.String(), token) {
			t.Errorf("response echoes the submitted token %q: %s", token, rec.Body.String())
		}
	}
}

// TestVerifyTMDBPersistsNothing pins that verification is a check, not a save.
func TestVerifyTMDBPersistsNothing(t *testing.T) {
	stub, _ := newTMDBStub(t, "good-token")
	s, setup := newVerifyServer(t, stub.URL)
	path := s.config().ConfigPath
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if rec := postJSON(t, s, "/api/settings/verify/tmdb", setup, verifyTMDBRequest{Token: "good-token"}); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Error("verification wrote to the config file")
	}
	if s.config().TMDBReadToken != "" {
		t.Error("verification applied the candidate token to the live config")
	}
}

func TestVerifyAndProviderRoutesRequireSetupToken(t *testing.T) {
	stub, _ := newTMDBStub(t, "good-token")
	s, _ := newVerifyServer(t, stub.URL)

	for _, target := range []string{"/api/settings/verify/tmdb", "/api/settings/providers"} {
		rec := postJSON(t, s, target, "", verifyTMDBRequest{Token: "good-token"})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a setup token: status = %d, want 401", target, rec.Code)
		}
	}
}

func TestProviderListReturnsSortedOptions(t *testing.T) {
	stub, _ := newTMDBStub(t, "good-token")
	s, setup := newVerifyServer(t, stub.URL)

	rec := postJSON(t, s, "/api/settings/providers", setup, providerListRequest{Token: "good-token", Region: "gb"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got providerListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Region != "GB" {
		t.Errorf("region = %q, want GB (normalized)", got.Region)
	}
	want := []string{"Amazon Prime Video", "Disney Plus", "Netflix"}
	if len(got.Providers) != len(want) {
		t.Fatalf("got %d providers, want %d", len(got.Providers), len(want))
	}
	for i, name := range want {
		if got.Providers[i].Name != name {
			t.Errorf("provider %d = %q, want %q (the picker order must be stable)", i, got.Providers[i].Name, name)
		}
	}
	if got.Providers[2].ID != "netflix" {
		t.Errorf("Netflix id = %q, want the slug the source list uses", got.Providers[2].ID)
	}
}

func TestProviderListIsCachedPerRegion(t *testing.T) {
	stub, calls := newTMDBStub(t, "good-token")
	s, setup := newVerifyServer(t, stub.URL)

	for i := 0; i < 3; i++ {
		if rec := postJSON(t, s, "/api/settings/providers", setup, providerListRequest{Token: "good-token", Region: "US"}); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("upstream calls = %d, want 1: repeated requests for one region must be served from cache", got)
	}

	if rec := postJSON(t, s, "/api/settings/providers", setup, providerListRequest{Token: "good-token", Region: "GB"}); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls = %d, want 2: a different region is a different list", got)
	}
}

func TestProviderListWithoutATokenIsRejected(t *testing.T) {
	stub, _ := newTMDBStub(t, "good-token")
	s, setup := newVerifyServer(t, stub.URL)

	rec := postJSON(t, s, "/api/settings/providers", setup, providerListRequest{Region: "US"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when no token is stored or supplied", rec.Code)
	}
}

// TestEnvironmentOnlyKeepsDefaultProviders is the compatibility guarantee: a
// deployment that never created a config file must not lose its streaming
// sources on upgrade.
func TestEnvironmentOnlyKeepsDefaultProviders(t *testing.T) {
	clearConfigEnv(t)
	t.Chdir(t.TempDir())
	t.Setenv("TMDB_READ_TOKEN", "tmdb-token")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.StreamingProviders) != 3 {
		t.Errorf("providers = %v, want the three defaults", cfg.StreamingProviders)
	}
	if !cfg.StreamingConfigured() {
		t.Error("an environment-only deployment with a token must still offer streaming")
	}
}

// TestFileManagedStreamingHasNoDefaults is the other half of the rule: once the
// file manages streaming, services are chosen explicitly or not at all.
func TestFileManagedStreamingHasNoDefaults(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
streaming:
  enabled: true
  tmdbReadToken: tmdb-token
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.StreamingProviders) != 0 {
		t.Errorf("providers = %v, want none: a file-managed deployment selects explicitly", cfg.StreamingProviders)
	}
	if cfg.StreamingConfigured() {
		t.Error("streaming with no providers must not report as configured")
	}
}

// TestFileManagedStreamingIgnoresTheEnvironmentList confirms the file's
// management of the section outranks a leftover environment variable.
func TestFileManagedStreamingIgnoresTheEnvironmentList(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, "streaming:\n  tmdbReadToken: tmdb-token\n")
	t.Setenv("STREAMING_PROVIDERS", "netflix,prime")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.StreamingProviders) != 0 {
		t.Errorf("providers = %v, want none", cfg.StreamingProviders)
	}
	if p := cfg.Provenance["streaming.providers"]; !p.EnvIgnored {
		t.Error("want STREAMING_PROVIDERS reported as ignored so the operator can see why")
	}
}

// TestEmptyProviderSelectionRegistersNoStreamingSource confirms the empty case
// reaches the existing degradation path rather than a new one.
func TestEmptyProviderSelectionRegistersNoStreamingSource(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("CACHE_DIR", t.TempDir())
	writeConfig(t, `
streaming:
  enabled: true
  tmdbReadToken: tmdb-token
plex:
  url: http://plex.local:32400
  token: plex-token
`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	s := New(cfg)

	got := sourceIDs(configuredSources(s.currentSources().sources, s.currentSources().order))
	if len(got) != 1 || got[0] != SourcePlex {
		t.Errorf("sources = %v, want only Plex: streaming with no providers advertises nothing", got)
	}
}

// TestVerifyTMDBIgnoresAWarmProviderCache is the regression test for a false
// positive: the provider cache is keyed by region alone, so a verification
// served from it would report any token as valid once anyone had looked at that
// region. Verification must always reach the upstream.
func TestVerifyTMDBIgnoresAWarmProviderCache(t *testing.T) {
	stub, calls := newTMDBStub(t, "good-token")
	s, setup := newVerifyServer(t, stub.URL)

	// Warm the cache for this region with a token that works.
	if rec := postJSON(t, s, "/api/settings/providers", setup, providerListRequest{Token: "good-token", Region: "US"}); rec.Code != http.StatusOK {
		t.Fatalf("warming: status = %d", rec.Code)
	}
	warmed := calls.Load()

	rec := postJSON(t, s, "/api/settings/verify/tmdb", setup, verifyTMDBRequest{Token: "bad-token", Region: "US"})
	var got verifyTMDBResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Valid {
		t.Error("a bad token was reported valid because the region's provider list was cached")
	}
	if calls.Load() == warmed {
		t.Error("verification made no upstream call; it was served from the cache")
	}
}
