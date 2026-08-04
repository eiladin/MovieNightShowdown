package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSettingsServer builds a server whose config file lives in a temp dir,
// returning it, the config path, and the setup token it issued.
func newSettingsServer(t *testing.T, body string) (*Server, string, string) {
	t.Helper()
	clearConfigEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if body != "" {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	// Resolve through the real path rather than hand-building a Config: a save
	// re-resolves from the file, and a hand-built baseline would differ from
	// the resolved result in settings the request never touched.
	t.Setenv("CACHE_DIR", t.TempDir())
	t.Setenv("SESSION_TTL", "1h")
	cfg, err := resolveConfigAt(path, false)
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	s := New(cfg)
	return s, path, s.setupToken
}

func settingsRequestFor(t *testing.T, method, token string, body any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, "/api/settings", &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestSettingsRequireSetupToken(t *testing.T) {
	s, _, token := newSettingsServer(t, "")

	cases := []struct {
		name   string
		method string
		token  string
	}{
		{"get without token", http.MethodGet, ""},
		{"get with wrong token", http.MethodGet, "wrong"},
		{"post without token", http.MethodPost, ""},
		{"post with wrong token", http.MethodPost, "wrong"},
		// A token of the right length but wrong value must fail exactly like a
		// short one; the constant-time compare exists for this case.
		{"post with same-length wrong token", http.MethodPost, strings.Repeat("a", len(token))},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, settingsRequestFor(t, tc.method, tc.token, settingsRequest{}))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", tc.name, rec.Code)
		}
	}
}

func TestSettingsWritePersistsAtMode0600(t *testing.T) {
	s, path, token := newSettingsServer(t, "")

	url := "http://plex.local:32400"
	tok := "plex-secret"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodPost, token, settingsRequest{
		Plex: &plexRequest{URL: &url, Token: &tok},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file mode = %#o, want 0600: it holds credentials", perm)
	}

	cf, err := loadConfigFile(path)
	if err != nil || cf == nil || cf.Plex == nil {
		t.Fatalf("reload: cf = %+v, err = %v", cf, err)
	}
	if cf.Plex.Token == nil || *cf.Plex.Token != "plex-secret" {
		t.Error("the submitted credential was not persisted")
	}
}

// TestSettingsResponseCarriesNoSecret is the check that matters most here: a
// response body reaches a browser, its cache, and any proxy in between.
func TestSettingsResponseCarriesNoSecret(t *testing.T) {
	s, _, token := newSettingsServer(t, `
plex:
  url: http://plex.local:32400
  token: PLEX-SECRET-VALUE
jellyfin:
  enabled: false
  url: http://jellyfin.local:8096
  apiKey: JELLYFIN-SECRET-VALUE
streaming:
  enabled: false
  tmdbReadToken: TMDB-SECRET-VALUE
`)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, settingsRequestFor(t, method, token, settingsRequest{}))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", method, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, secret := range []string{"PLEX-SECRET-VALUE", "JELLYFIN-SECRET-VALUE", "TMDB-SECRET-VALUE"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s response contains the credential %q", method, secret)
			}
		}
		var got settingsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: decode response: %v", method, err)
		}
		if !got.Plex.TokenSet {
			t.Errorf("%s: want tokenSet true so the screen can render a placeholder", method)
		}
	}
}

// TestSettingsOmittedSecretIsUnchanged pins the rule that keeps a working
// credential alive: the screen never receives a secret, so it cannot send one
// back, and an unrelated edit must not wipe it.
func TestSettingsOmittedSecretIsUnchanged(t *testing.T) {
	s, path, token := newSettingsServer(t, `
plex:
  url: http://plex.local:32400
  token: keep-me
`)

	section := "3"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodPost, token, settingsRequest{
		Plex: &plexRequest{LibrarySection: &section},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	cf, _ := loadConfigFile(path)
	if cf.Plex.Token == nil || *cf.Plex.Token != "keep-me" {
		t.Error("an omitted secret was not preserved")
	}
	if cf.Plex.LibrarySection == nil || *cf.Plex.LibrarySection != "3" {
		t.Error("the submitted field was not applied")
	}
}

func TestSettingsExplicitClearRemovesSecret(t *testing.T) {
	s, path, token := newSettingsServer(t, `
plex:
  enabled: false
  url: http://plex.local:32400
  token: remove-me
`)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodPost, token, settingsRequest{
		Plex: &plexRequest{ClearToken: true},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	cf, _ := loadConfigFile(path)
	if cf.Plex.Token != nil {
		t.Errorf("token = %q, want removed by an explicit clear", *cf.Plex.Token)
	}
}

func TestSettingsValidationRejectsAndLeavesFileUntouched(t *testing.T) {
	original := "plex:\n  enabled: false\n  url: http://plex.local:32400\n"
	s, path, token := newSettingsServer(t, original)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	cases := []struct {
		name    string
		body    settingsRequest
		wantKey string
	}{
		{
			name:    "unparseable url",
			body:    settingsRequest{Plex: &plexRequest{URL: ptr("://nonsense")}},
			wantKey: "plex.url",
		},
		{
			name:    "non-http scheme",
			body:    settingsRequest{Plex: &plexRequest{URL: ptr("ftp://plex.local")}},
			wantKey: "plex.url",
		},
		{
			name:    "enabled without credentials",
			body:    settingsRequest{Plex: &plexRequest{Enabled: ptr(true)}},
			wantKey: "plex.token",
		},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodPost, token, tc.body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body %s)", tc.name, rec.Code, rec.Body.String())
			continue
		}
		var got struct {
			Errors map[string]string `json:"errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: decode: %v", tc.name, err)
		}
		if _, ok := got.Errors[tc.wantKey]; !ok {
			t.Errorf("%s: errors = %v, want a message keyed %q", tc.name, got.Errors, tc.wantKey)
		}
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("a rejected write modified the config file")
	}
}

func TestSettingsMalformedBodyIsRejected(t *testing.T) {
	s, _, token := newSettingsServer(t, "")

	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader("{not json"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestSetupTokenPersistsAcrossRestart(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")

	first := ensureSetupToken(path)
	if len(first) != setupTokenBytes*2 {
		t.Errorf("token length = %d, want %d hex characters", len(first), setupTokenBytes*2)
	}
	if second := ensureSetupToken(path); second != first {
		t.Error("the setup token changed across restarts; it must be persisted")
	}
}

// TestSetupTokenPersistenceDoesNotDisturbSettings guards the one write the
// application makes without being asked.
func TestSetupTokenPersistenceDoesNotDisturbSettings(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("plex:\n  url: http://plex.local:32400\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	ensureSetupToken(path)

	cf, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cf.Plex == nil || cf.Plex.URL == nil || *cf.Plex.URL != "http://plex.local:32400" {
		t.Error("persisting the setup token discarded existing settings")
	}
}

func TestSetupTokenWithoutConfigPathIsEphemeral(t *testing.T) {
	// A Config built directly rather than through LoadConfig has nowhere to
	// persist a token. It must still get one rather than leaving config writes
	// unauthorizable.
	if got := ensureSetupToken(""); got == "" {
		t.Error("want a token even with no config path")
	}
}

func TestWriteConfigFileIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	if err := writeConfigFile(path, &configFile{PublicURL: ptr("http://nas.local:8080")}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("config dir mode = %#o, want 0700", perm)
	}

	// No temporary files may survive a successful write.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config-") {
			t.Errorf("temporary file %q left behind", e.Name())
		}
	}
}

func ptr[T any](v T) *T { return &v }

// TestSettingsReportsRuntimeSettingsReadOnly pins that the container-level
// settings are visible but not writable: they are reported so an operator can
// see what is in effect, and absent from the request type because saving cannot
// change something established before the process started.
func TestSettingsReportsRuntimeSettingsReadOnly(t *testing.T) {
	s, path, token := newSettingsServer(t, "publicUrl: http://nas:8080\n")

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodGet, token, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Runtime.Port != s.config().Port {
		t.Errorf("runtime port = %q, want %q", got.Runtime.Port, s.config().Port)
	}
	if got.Runtime.CacheDir != s.config().CacheDir {
		t.Errorf("runtime cacheDir = %q, want the live value", got.Runtime.CacheDir)
	}
	if got.Runtime.ConfigPath != path {
		t.Errorf("runtime configPath = %q, want %q", got.Runtime.ConfigPath, path)
	}

	// Nothing in a save request can reach them: the config file has no key for
	// the cache directory or the port at all.
	cf, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if cf == nil || strings.Contains(string(raw), "cacheDir") || strings.Contains(string(raw), "port") {
		t.Error("a container-level setting reached the config file")
	}
}

// TestSettingsShowsEnvironmentSeededValues is the regression test for a screen
// that rendered the config file instead of the resolved configuration.
//
// A deployment configured entirely by environment variables has no file, so the
// screen showed empty fields for a server that was working — and saving what it
// showed was rejected as "required when Plex is enabled" for credentials that
// demonstrably existed.
func TestSettingsShowsEnvironmentSeededValues(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("CONFIG_FILE", path)
	t.Setenv("CACHE_DIR", t.TempDir())
	t.Setenv("PLEX_URL", "http://plex.local:32400")
	t.Setenv("PLEX_TOKEN", "plex-secret")
	t.Setenv("PUBLIC_URL", "http://nas:8080")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	s := New(cfg)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodGet, s.setupToken, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Plex.URL != "http://plex.local:32400" {
		t.Errorf("plex.url = %q, want the environment-seeded value", got.Plex.URL)
	}
	if !got.Plex.TokenSet {
		t.Error("want tokenSet true: the credential exists, it just came from the environment")
	}
	if got.PublicURL != "http://nas:8080" {
		t.Errorf("publicUrl = %q, want the environment-seeded value", got.PublicURL)
	}
	// Still no secret in the body, and the provenance says where it came from.
	if strings.Contains(rec.Body.String(), "plex-secret") {
		t.Error("response contains the credential")
	}
	if p := got.Provenance["plex.url"]; p.Source != string(sourceEnv) {
		t.Errorf("plex.url provenance = %q, want %q", p.Source, sourceEnv)
	}
}

// TestSettingsShowsDisabledSourceValues pins that pausing a source does not
// blank its fields; the operator must be able to switch it back on without
// retyping a credential.
func TestSettingsShowsDisabledSourceValues(t *testing.T) {
	s, _, token := newSettingsServer(t, `
plex:
  enabled: false
  url: http://plex.local:32400
  token: plex-secret
`)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodGet, token, nil))
	var got settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Plex.Enabled {
		t.Error("want the toggle reported as off")
	}
	if got.Plex.URL != "http://plex.local:32400" || !got.Plex.TokenSet {
		t.Error("a disabled source must still show what is stored for it")
	}
}

// TestSettingsSaveAcceptsEnvironmentSuppliedCredentials is the second half of
// the same regression. Validation ran against the merged config file while the
// credentials lived in the environment, so saving the screen back rejected a
// Plex token that was demonstrably set — the settings screen could not be used
// at all on an environment-configured deployment.
func TestSettingsSaveAcceptsEnvironmentSuppliedCredentials(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("CONFIG_FILE", path)
	t.Setenv("CACHE_DIR", t.TempDir())
	t.Setenv("PLEX_URL", "http://plex.local:32400")
	t.Setenv("PLEX_TOKEN", "plex-secret")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	s := New(cfg)

	// Save exactly what the screen renders: the URL is visible and editable,
	// the token is a placeholder the client omits.
	url := "http://plex.local:32400"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodPost, s.setupToken, settingsRequest{
		Plex: &plexRequest{Enabled: ptr(true), URL: &url},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// TestSettingsToggleFollowsWhatIsConfigured pins that a fresh deployment does
// not open with every section expanded and empty, demanding credentials for
// services the operator does not use.
func TestSettingsToggleFollowsWhatIsConfigured(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("CONFIG_FILE", path)
	t.Setenv("CACHE_DIR", t.TempDir())
	t.Setenv("PLEX_URL", "http://plex.local:32400")
	t.Setenv("PLEX_TOKEN", "plex-secret")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	s := New(cfg)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodGet, s.setupToken, nil))
	var got settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Plex.Enabled {
		t.Error("want Plex on: it has credentials")
	}
	if got.Jellyfin.Enabled {
		t.Error("want Jellyfin off: nothing is configured for it")
	}
	if got.Streaming.Enabled {
		t.Error("want streaming off: no TMDB token is configured")
	}
}
