package server

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"slices"
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
		"JELLYFIN_LIBRARIES",
		"PLEX_URL", "PLEX_TOKEN", "PLEX_LIBRARY_SECTION", "PLEX_LIBRARY_SECTIONS",
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

// --- library lists ---

func libraryIDs(refs []libraryRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

func TestLibrariesFromEnvironment(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("JELLYFIN_LIBRARIES", "Movies,Anime")
	t.Setenv("PLEX_LIBRARY_SECTIONS", "1,3")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if got := libraryIDs(cfg.JellyfinLibraries); !slices.Equal(got, []string{"Movies", "Anime"}) {
		t.Errorf("jellyfin libraries = %v", got)
	}
	if got := libraryIDs(cfg.PlexLibraries); !slices.Equal(got, []string{"1", "3"}) {
		t.Errorf("plex libraries = %v", got)
	}
	// Nothing knows a name from an identifier yet, and nothing needs to.
	for _, ref := range cfg.JellyfinLibraries {
		if ref.Name != "" {
			t.Errorf("ref %+v carries a name; the environment supplies bare strings", ref)
		}
	}
}

// The case of an entry is left exactly as written. normalizeProviders lowercases,
// which is right for a provider name and wrong here: this same list holds opaque
// identifiers, and folding one corrupts it. Name matching folds case at the point
// of comparison instead.
func TestLibraryEntriesKeepTheirCase(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("JELLYFIN_LIBRARIES", "  A1B2C3d4  , Kids Movies ,,A1B2C3d4")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Trimmed, de-duplicated, empty entries dropped — and not lowercased.
	want := []string{"A1B2C3d4", "Kids Movies"}
	if got := libraryIDs(cfg.JellyfinLibraries); !slices.Equal(got, want) {
		t.Errorf("libraries = %v, want %v", got, want)
	}
}

// The plural is the current name. The singular stays honoured so an existing
// deployment does not silently change which library it deals from, and provenance
// has to name whichever one actually supplied the value — naming the variable that
// was merely checked first would send an operator to the wrong line of their
// compose file.
func TestPlexLibrarySectionsSupersedesTheSingular(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PLEX_LIBRARY_SECTIONS", "5")
	t.Setenv("PLEX_LIBRARY_SECTION", "9")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if got := libraryIDs(cfg.PlexLibraries); !slices.Equal(got, []string{"5"}) {
		t.Errorf("libraries = %v, want the plural variable to win", got)
	}
	if p := cfg.Provenance["plex.libraries"]; p.EnvVar != "PLEX_LIBRARY_SECTIONS" {
		t.Errorf("provenance names %q, want the variable that supplied the value", p.EnvVar)
	}
}

func TestDeprecatedPlexLibrarySectionStillResolves(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PLEX_LIBRARY_SECTION", "9")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if got := libraryIDs(cfg.PlexLibraries); !slices.Equal(got, []string{"9"}) {
		t.Errorf("libraries = %v, want the deprecated variable to be honoured", got)
	}
	if p := cfg.Provenance["plex.libraries"]; p.EnvVar != "PLEX_LIBRARY_SECTION" {
		t.Errorf("provenance names %q, want the deprecated variable that supplied it", p.EnvVar)
	}
}

func TestLibrariesFromFileWinAndCarryNames(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
jellyfin:
  libraries:
    - id: aaa
      name: Movies
    - id: bbb
      name: Kids Movies
`)
	t.Setenv("JELLYFIN_LIBRARIES", "ignored")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	want := []libraryRef{{ID: "aaa", Name: "Movies"}, {ID: "bbb", Name: "Kids Movies"}}
	if !slices.Equal(cfg.JellyfinLibraries, want) {
		t.Errorf("libraries = %+v, want %+v", cfg.JellyfinLibraries, want)
	}
	if p := cfg.Provenance["jellyfin.libraries"]; p.Source != sourceFile || !p.EnvIgnored {
		t.Errorf("provenance = %+v, want file with the variable reported as ignored", p)
	}
}

// An empty list in the file is a value, not an absence: it must not fall through
// to the environment. Every reader treats it the same as no list at all — every
// library — but the distinction decides which source supplied it, and therefore
// what the settings screen shows.
func TestEmptyLibraryListInFileIsNotAbsent(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
plex:
  libraries: []
`)
	t.Setenv("PLEX_LIBRARY_SECTIONS", "7")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.PlexLibraries) != 0 {
		t.Errorf("libraries = %+v, want the file's empty list to win", cfg.PlexLibraries)
	}
	if p := cfg.Provenance["plex.libraries"]; p.Source != sourceFile {
		t.Errorf("provenance source = %q, want the config file", p.Source)
	}
}

// A file entry with no identifier names nothing and cannot be queried.
func TestLibraryEntryWithoutAnIDIsDropped(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
plex:
  libraries:
    - name: Orphan
    - id: "2"
      name: Films
`)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	want := []libraryRef{{ID: "2", Name: "Films"}}
	if !slices.Equal(cfg.PlexLibraries, want) {
		t.Errorf("libraries = %+v, want %+v", cfg.PlexLibraries, want)
	}
}

// The shipped compose file passes every variable through as `${VAR:-}`, so an
// unconfigured deployment has them all present and empty. Present-and-empty
// supplies nothing and must not be reported as overriding anything.
func TestEmptyLibraryEnvironmentVariableSuppliesNothing(t *testing.T) {
	clearConfigEnv(t)
	writeConfig(t, `
jellyfin:
  libraries:
    - id: aaa
`)
	t.Setenv("JELLYFIN_LIBRARIES", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if p := cfg.Provenance["jellyfin.libraries"]; p.EnvIgnored {
		t.Errorf("provenance = %+v; the variable is present and empty, so there is "+
			"no conflict to report", p)
	}
}

// With nothing configured anywhere the list is empty, which every reader takes as
// every library. This is the upgrade-safety case: a deployment that has never
// heard of these settings resolves exactly as it did before they existed.
func TestNoLibraryConfigurationResolvesToEveryLibrary(t *testing.T) {
	clearConfigEnv(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.JellyfinLibraries) != 0 || len(cfg.PlexLibraries) != 0 {
		t.Errorf("libraries = %+v / %+v, want both empty",
			cfg.JellyfinLibraries, cfg.PlexLibraries)
	}
	for _, key := range []string{"jellyfin.libraries", "plex.libraries"} {
		p := cfg.Provenance[key]
		if p.Source != sourceDefault {
			t.Errorf("%s source = %q, want the default", key, p.Source)
		}
		if p.EnvVar == "" {
			t.Errorf("%s names no variable; the screen still has to say which would apply", key)
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
// maskSecret is what keeps a credential out of the logger's hands entirely, rather
// than handing it over and relying on a flag to stop it being printed. A static
// analyser cannot follow that flag, and neither can a reader adding a setting.
func TestMaskSecretNeverReturnsTheValue(t *testing.T) {
	for _, value := range []string{"a", "JELLYFIN-SECRET-VALUE", "***", strings.Repeat("x", 64)} {
		got := maskSecret(value)
		if got != maskedSecret {
			t.Errorf("maskSecret(%q) = %q, want %q", value, got, maskedSecret)
		}
	}
	if got := maskSecret(""); got != absentSecret {
		t.Errorf("maskSecret(\"\") = %q, want %q", got, absentSecret)
	}
}

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
