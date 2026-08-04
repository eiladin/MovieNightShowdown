package server

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

// defaultConfigPath is where the config file is looked for when CONFIG_FILE is
// unset. A file missing from here is silence; a file missing from an explicitly
// configured path is fatal (see resolveConfigPath and LoadConfig).
const defaultConfigPath = "./config/config.yaml"

// configFile is the on-disk configuration the application owns and the settings
// screen writes.
//
// Every scalar is a pointer so "absent from the file" and "present but empty"
// remain distinguishable. That distinction is what makes per-key resolution
// correct: an absent key falls through to the environment, while a key the
// operator deliberately blanked must stay blank rather than silently
// resurrecting an environment variable they thought they had overridden.
type configFile struct {
	// SetupToken authorizes configuration changes. It is generated on first
	// start and is the only field the application writes without being asked.
	SetupToken *string           `yaml:"setupToken"`
	PublicURL  *string           `yaml:"publicUrl"`
	SessionTTL *string           `yaml:"sessionTtl"`
	CacheDir   *string           `yaml:"cacheDir"`
	Jellyfin   *jellyfinSection  `yaml:"jellyfin"`
	Plex       *plexSection      `yaml:"plex"`
	Streaming  *streamingSection `yaml:"streaming"`
}

type jellyfinSection struct {
	Enabled *bool   `yaml:"enabled"`
	URL     *string `yaml:"url"`
	APIKey  *string `yaml:"apiKey"`
	UserID  *string `yaml:"userId"`
}

type plexSection struct {
	Enabled        *bool   `yaml:"enabled"`
	URL            *string `yaml:"url"`
	Token          *string `yaml:"token"`
	LibrarySection *string `yaml:"librarySection"`
}

type streamingSection struct {
	Enabled       *bool     `yaml:"enabled"`
	TMDBReadToken *string   `yaml:"tmdbReadToken"`
	WatchRegion   *string   `yaml:"watchRegion"`
	Providers     *[]string `yaml:"providers"`
}

// loadConfigFile reads and parses the config file at path.
//
// It returns (nil, nil) when the file does not exist, so callers can tell the
// three outcomes apart: present, absent, and malformed. Absent is only an error
// when the operator named the path themselves.
func loadConfigFile(path string) (*configFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}
	var cf configFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}
	return &cf, nil
}

// resolveConfigPath returns the config file path and whether it was set
// explicitly. The two cases differ only in what a missing file means.
func resolveConfigPath() (path string, explicit bool) {
	if p := os.Getenv("CONFIG_FILE"); p != "" {
		return p, true
	}
	return defaultConfigPath, false
}

// settingSource is where a resolved setting's value came from.
type settingSource string

const (
	sourceDefault settingSource = "default"
	sourceEnv     settingSource = "environment"
	sourceFile    settingSource = "config file"
)

// settingProvenance records how one setting was resolved, so startup can state
// it and the settings screen can flag a value the operator may believe is in
// effect when it is not.
type settingProvenance struct {
	Source settingSource
	// EnvVar is the environment variable that would supply this setting.
	EnvVar string
	// EnvIgnored is true when EnvVar is set but the config file won, which is
	// the one combination an operator is likely to misread.
	EnvIgnored bool
	// Secret marks a setting whose value must never be logged or returned.
	Secret bool
}

// resolver merges the config file, the environment, and built-in defaults into
// one value per key, recording where each came from.
//
// Resolution is per key rather than per source on purpose: a file that sets
// plex.url and an environment that sets PLEX_TOKEN must yield both. Overlaying
// whole structs would let one source's presence discard the other's values.
type resolver struct {
	file *configFile
	prov map[string]settingProvenance
}

func newResolver(file *configFile) *resolver {
	return &resolver{file: file, prov: map[string]settingProvenance{}}
}

// str resolves one string setting: the file wins when it sets the key,
// otherwise the environment, otherwise the default.
func (r *resolver) str(key, envVar string, fileVal *string, def string) string {
	env, envSet := os.LookupEnv(envVar)
	p := settingProvenance{EnvVar: envVar}
	switch {
	case fileVal != nil:
		p.Source = sourceFile
		p.EnvIgnored = envSet
		r.prov[key] = p
		return *fileVal
	case envSet && env != "":
		p.Source = sourceEnv
		r.prov[key] = p
		return env
	default:
		p.Source = sourceDefault
		r.prov[key] = p
		return def
	}
}

// secret resolves a credential. It behaves exactly as str but marks the
// provenance entry so nothing downstream logs or returns the value.
func (r *resolver) secret(key, envVar string, fileVal *string) string {
	v := r.str(key, envVar, fileVal, "")
	p := r.prov[key]
	p.Secret = true
	r.prov[key] = p
	return v
}

// enabled resolves a source's on/off toggle. Only the config file can disable a
// source: with no file, a source is on whenever its credentials are present,
// which is the behaviour every existing deployment has.
func (r *resolver) enabled(key string, fileVal *bool) bool {
	if fileVal != nil {
		r.prov[key] = settingProvenance{Source: sourceFile}
		return *fileVal
	}
	r.prov[key] = settingProvenance{Source: sourceDefault}
	return true
}

// providers resolves the streaming provider list.
//
// The file's list wins when present — including an empty list, which means
// "offer none" rather than "fall back to the defaults".
//
// fileManaged reports whether the config file has a streaming section at all.
// It decides what an absent list means, and the two answers are deliberately
// different. A deployment configured only by environment variables keeps the
// three defaults it has always had, so no existing install loses its streaming
// sources on upgrade. A deployment whose streaming section is managed by the
// file has a settings screen and must choose explicitly; defaulting there would
// silently add services nobody selected.
func (r *resolver) providers(key, envVar string, fileVal *[]string, fileManaged bool) []string {
	env, envSet := os.LookupEnv(envVar)
	p := settingProvenance{EnvVar: envVar}
	if fileVal != nil {
		p.Source = sourceFile
		p.EnvIgnored = envSet
		r.prov[key] = p
		return normalizeProviders(*fileVal)
	}
	if fileManaged {
		p.Source = sourceFile
		p.EnvIgnored = envSet
		r.prov[key] = p
		return []string{}
	}
	if envSet && env != "" {
		p.Source = sourceEnv
		r.prov[key] = p
		return parseStreamingProviders(env)
	}
	p.Source = sourceDefault
	r.prov[key] = p
	return defaultStreamingProviders
}

// checkConfigFilePermissions warns when the config file is readable beyond its
// owner. It holds credentials in plaintext, so group and world access is worth
// naming — but the file belongs to the operator, so this warns and continues.
func checkConfigFilePermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		log.Printf("config: %s is mode %#o — it holds credentials in plaintext and should be 0600", path, mode)
	}
}

// logProvenance states where every setting came from, so a value that is not
// what an operator expects can be traced from the first place anyone looks.
// Secrets are reported as set or unset and never printed.
func logProvenance(prov map[string]settingProvenance, values map[string]string) {
	for _, key := range provenanceOrder {
		p, ok := prov[key]
		if !ok {
			continue
		}
		value := values[key]
		if p.Secret {
			value = "(unset)"
			if values[key] != "" {
				value = "***"
			}
		}
		line := fmt.Sprintf("config: %s = %s (%s", key, value, p.Source)
		if p.EnvIgnored {
			line += fmt.Sprintf("; %s present, ignored", p.EnvVar)
		}
		log.Print(line + ")")
	}
}

// provenanceOrder fixes the log's line order. A map iterates randomly, and a
// startup log that reorders itself between restarts cannot be diffed.
var provenanceOrder = []string{
	"publicUrl", "sessionTtl", "cacheDir",
	"jellyfin.enabled", "jellyfin.url", "jellyfin.apiKey", "jellyfin.userId",
	"plex.enabled", "plex.url", "plex.token", "plex.librarySection",
	"streaming.enabled", "streaming.tmdbReadToken", "streaming.watchRegion", "streaming.providers",
}

// normalizeProviders applies the same trimming, lowercasing, and de-duplication
// parseStreamingProviders applies to the environment form, so a provider list
// means the same thing whichever source supplied it. Unlike the environment
// form it never substitutes the defaults: an empty list from the file is a
// deliberate "offer none".
func normalizeProviders(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		name := strings.ToLower(strings.TrimSpace(p))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
