package server

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes a config file into a temp dir and points CONFIG_FILE at
// it, returning the path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_FILE", path)
	return path
}

// clearConfigEnv unsets every environment variable resolution consults, so a
// test starts from a known state rather than inheriting the developer's shell.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"CONFIG_FILE", "JELLYFIN_URL", "JELLYFIN_API_KEY", "JELLYFIN_USER_ID",
		"PLEX_URL", "PLEX_TOKEN", "PLEX_LIBRARY_SECTION",
		"TMDB_READ_TOKEN", "TMDB_WATCH_REGION", "STREAMING_PROVIDERS",
		"PUBLIC_URL", "SESSION_TTL", "CACHE_DIR", "PORT",
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

func TestLoadConfigFileOnly(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
publicUrl: http://nas.local:8080
jellyfin:
  url: http://jellyfin.local:8096
  apiKey: jf-key
plex:
  url: http://plex.local:32400
  token: plex-token
  librarySection: "2"
streaming:
  tmdbReadToken: tmdb-token
  watchRegion: gb
  providers: [Netflix, " prime ", netflix]
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PublicURL != "http://nas.local:8080" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	if !cfg.JellyfinConfigured() || !cfg.PlexConfigured() || !cfg.StreamingConfigured() {
		t.Error("want all three sources configured from the file alone")
	}
	if cfg.PlexLibrarySection != "2" {
		t.Errorf("PlexLibrarySection = %q, want 2", cfg.PlexLibrarySection)
	}
	if cfg.TMDBWatchRegion != "GB" {
		t.Errorf("TMDBWatchRegion = %q, want GB (upper-cased)", cfg.TMDBWatchRegion)
	}
	// Providers from the file get the same normalization the environment form
	// gets: trimmed, lower-cased, de-duplicated.
	want := []string{"netflix", "prime"}
	if strings.Join(cfg.StreamingProviders, ",") != strings.Join(want, ",") {
		t.Errorf("StreamingProviders = %v, want %v", cfg.StreamingProviders, want)
	}
}

func TestLoadConfigEnvironmentOnly(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("JELLYFIN_URL", "http://jellyfin.local:8096")
	t.Setenv("JELLYFIN_API_KEY", "jf-key")
	t.Setenv("PUBLIC_URL", "http://nas.local:8080")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.JellyfinConfigured() {
		t.Error("want Jellyfin configured from the environment with no config file")
	}
	if cfg.PublicURL != "http://nas.local:8080" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	// The defaults a deployment has today must survive: no file means the
	// streaming provider defaults still apply.
	if len(cfg.StreamingProviders) != 3 {
		t.Errorf("StreamingProviders = %v, want the three defaults", cfg.StreamingProviders)
	}
	if cfg.Provenance["publicUrl"].Source != sourceEnv {
		t.Errorf("publicUrl provenance = %q, want %q", cfg.Provenance["publicUrl"].Source, sourceEnv)
	}
}

// TestLoadConfigMergesPerKey is the case the whole resolver exists for: a file
// and an environment that each set part of one source must yield both, not one
// source's values discarding the other's.
func TestLoadConfigMergesPerKey(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
plex:
  url: http://from-file:32400
`)
	t.Setenv("PLEX_TOKEN", "token-from-env")
	t.Setenv("PLEX_URL", "http://from-env:32400")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PlexURL != "http://from-file:32400" {
		t.Errorf("PlexURL = %q, want the file's value to win", cfg.PlexURL)
	}
	if cfg.PlexToken != "token-from-env" {
		t.Errorf("PlexToken = %q, want the environment's value, which the file does not set", cfg.PlexToken)
	}
	if p := cfg.Provenance["plex.url"]; p.Source != sourceFile || !p.EnvIgnored {
		t.Errorf("plex.url provenance = %+v, want file with PLEX_URL reported as ignored", p)
	}
	if p := cfg.Provenance["plex.token"]; p.Source != sourceEnv || p.EnvIgnored {
		t.Errorf("plex.token provenance = %+v, want environment and not ignored", p)
	}
}

// An environment variable that is present but empty supplies nothing, so it is
// not a value the config file overrode and must not be reported as one.
//
// This is the state of every variable in a stock deployment: the shipped
// docker-compose.yml passes each one through as `${VAR:-}`, so with nothing set on
// the host they all exist in the container as empty strings. Reporting them as
// overridden badged every setting on the settings screen with "set but ignored"
// for a conflict that did not exist — and a warning that fires on everything is
// one nobody reads the day it means something.
func TestEmptyEnvironmentVariableIsNotReportedAsOverridden(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
publicUrl: http://saved:8080
plex:
  url: http://from-file:32400
  token: token-from-file
streaming:
  providers: [netflix]
`)
	// Exactly what compose produces for an unconfigured host: present, empty.
	for _, k := range []string{"PUBLIC_URL", "PLEX_URL", "PLEX_TOKEN", "STREAMING_PROVIDERS"} {
		t.Setenv(k, "")
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	for _, key := range []string{"publicUrl", "plex.url", "plex.token", "streaming.providers"} {
		p := cfg.Provenance[key]
		if p.Source != sourceFile {
			t.Errorf("%s source = %q, want the config file", key, p.Source)
		}
		if p.EnvIgnored {
			t.Errorf("%s reported %s as set but ignored; it is present and empty, so it "+
				"supplied nothing and there is nothing to warn about", key, p.EnvVar)
		}
	}
}

// The warning still has to fire when it is real, or removing the false positives
// would have removed the whole mitigation.
func TestNonEmptyEnvironmentVariableIsStillReportedAsOverridden(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
plex:
  url: http://from-file:32400
streaming:
  providers: [netflix]
`)
	t.Setenv("PLEX_URL", "http://from-env:32400")
	t.Setenv("STREAMING_PROVIDERS", "hulu")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	for _, key := range []string{"plex.url", "streaming.providers"} {
		if p := cfg.Provenance[key]; !p.EnvIgnored {
			t.Errorf("%s provenance = %+v, want %s reported as ignored", key, p, p.EnvVar)
		}
	}
}

// TestLoadConfigEmptyStringInFileIsNotAbsent pins the reason every scalar is a
// pointer: a key the operator deliberately blanked must stay blank rather than
// falling back to an environment variable they believe they overrode.
func TestLoadConfigEmptyStringInFileIsNotAbsent(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
plex:
  url: ""
`)
	t.Setenv("PLEX_URL", "http://from-env:32400")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PlexURL != "" {
		t.Errorf("PlexURL = %q, want empty: the file set it explicitly", cfg.PlexURL)
	}
}

// TestLoadConfigDisabledSourceKeepsCredentials pins that switching a source off
// makes it unqueryable without discarding what is stored for it. The settings
// screen renders the resolved configuration, so clearing the values here would
// blank the fields of a source the operator merely paused — and they would have
// to retype a credential to turn it back on.
func TestLoadConfigDisabledSourceKeepsCredentials(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
plex:
  enabled: false
  url: http://plex.local:32400
  token: plex-token
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PlexConfigured() {
		t.Error("want Plex unconfigured when the file disables it")
	}
	if !cfg.PlexDisabled {
		t.Error("want PlexDisabled true")
	}
	if cfg.PlexToken != "plex-token" || cfg.PlexURL != "http://plex.local:32400" {
		t.Error("a disabled source must keep its stored values so they can be shown and re-enabled")
	}
}

// TestZeroConfigEnablesEverySource is why the toggles are stored inverted: a
// Config built directly, as several tests and any embedder do, must behave as
// its credentials say rather than silently disabling every source.
func TestZeroConfigEnablesEverySource(t *testing.T) {
	cfg := Config{PlexURL: "http://plex.local:32400", PlexToken: "t"}
	if !cfg.PlexConfigured() {
		t.Error("a zero-valued toggle must mean enabled, not disabled")
	}
}

func TestLoadConfigEmptyProviderListMeansNone(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
streaming:
  tmdbReadToken: tmdb-token
  providers: []
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// An empty list in the file is a deliberate "offer none", unlike an unset
	// environment variable, which falls back to the defaults.
	if len(cfg.StreamingProviders) != 0 {
		t.Errorf("StreamingProviders = %v, want none", cfg.StreamingProviders)
	}
	if cfg.StreamingConfigured() {
		t.Error("streaming with no providers must not report as configured")
	}
}

func TestLoadConfigMissingDefaultPathIsNotAnError(t *testing.T) {
	clearConfigEnv(t)
	// Run from a directory with no ./config/config.yaml.
	t.Chdir(t.TempDir())

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v, want no error when the default path is empty", err)
	}
	if cfg.PublicURL != "http://localhost:8080" {
		t.Errorf("PublicURL = %q, want the built-in default", cfg.PublicURL)
	}
}

// TestLoadConfigMissingExplicitPathStarts is the first-boot case: a deployment
// that sets CONFIG_FILE and has saved nothing yet must start, since the
// application creates the file itself on its first write.
func TestLoadConfigMissingExplicitPathStarts(t *testing.T) {
	clearConfigEnv(t)
	dir := filepath.Join(t.TempDir(), "config")
	missing := filepath.Join(dir, "config.yaml")
	t.Setenv("CONFIG_FILE", missing)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v, want a fresh deployment to start", err)
	}
	if cfg.ConfigPath != missing {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, missing)
	}
	// The directory is created now so a later save has somewhere to land.
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("config directory was not created: %v", err)
	}
}

// TestLoadConfigUnusableExplicitPathIsFatal is what the missing-file rule was
// actually protecting against: a path no save could ever be written to, which
// must fail at startup rather than at the first save.
func TestLoadConfigUnusableExplicitPathIsFatal(t *testing.T) {
	clearConfigEnv(t)
	// A regular file cannot be a parent directory.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	unusable := filepath.Join(blocker, "config.yaml")
	t.Setenv("CONFIG_FILE", unusable)

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("want an error for a config path that cannot be written to")
	}
	if !strings.Contains(err.Error(), unusable) {
		t.Errorf("error = %q, want it to name the path", err)
	}
}

func TestLoadConfigMalformedFileIsFatal(t *testing.T) {
	clearConfigEnv(t)
	path := writeConfig(t, "plex:\n  url: [unclosed\n")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("want an error for malformed YAML")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name the path", err)
	}
}

// TestProvenanceLogHasNoSecrets is the check that matters most in this file: a
// startup log is written to disk, shipped in bug reports, and re-read later.
func TestProvenanceLogHasNoSecrets(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
jellyfin:
  url: http://jellyfin.local:8096
  apiKey: JELLYFIN-SECRET-VALUE
plex:
  url: http://plex.local:32400
  token: PLEX-SECRET-VALUE
streaming:
  tmdbReadToken: TMDB-SECRET-VALUE
`)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	out := buf.String()
	for _, secret := range []string{"JELLYFIN-SECRET-VALUE", "PLEX-SECRET-VALUE", "TMDB-SECRET-VALUE"} {
		if strings.Contains(out, secret) {
			t.Errorf("startup log contains the credential %q", secret)
		}
	}
	if !strings.Contains(out, "plex.token = ***") {
		t.Errorf("want a set credential reported as masked; log was:\n%s", out)
	}
	if !strings.Contains(out, "plex.url = http://plex.local:32400") {
		t.Errorf("want non-secret values reported in full; log was:\n%s", out)
	}
}

func TestProvenanceLogNamesIgnoredEnvironmentVariables(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, "plex:\n  url: http://from-file:32400\n")
	t.Setenv("PLEX_URL", "http://from-env:32400")

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !strings.Contains(buf.String(), "PLEX_URL present, ignored") {
		t.Errorf("want the ignored environment variable named; log was:\n%s", buf.String())
	}
}

func TestConfigFilePermissionWarning(t *testing.T) {
	clearConfigEnv(t)
	path := writeConfig(t, "publicUrl: http://nas.local:8080\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !strings.Contains(buf.String(), "should be 0600") {
		t.Errorf("want a permissions warning for a world-readable config; log was:\n%s", buf.String())
	}
}

func TestLoadConfigFileAbsentReturnsNilNil(t *testing.T) {
	cf, err := loadConfigFile(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}
	if cf != nil {
		t.Error("want nil for an absent file, so callers can tell absent from empty")
	}
}
