package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// setupTokenBytes is the entropy behind the setup token. 32 bytes is well past
// what brute force over a network could reach and costs nothing to carry.
const setupTokenBytes = 32

// ensureSetupToken returns the deployment's setup token, generating and
// persisting one on first start.
//
// The token authorizes configuration changes. Without it, any client that can
// reach the application could rewrite where its sources point — which is both a
// request-forgery path into the network's interior and a way to have the server
// send its stored credentials to an attacker's host. Whoever can read the
// startup log is whoever deployed the application, which is exactly the
// audience that should be able to configure it.
//
// A token that cannot be persisted is not fatal: the application runs on a
// read-only filesystem in some deployments, and refusing to start would be a
// worse outcome than a token that changes on restart. The log says which
// happened.
func ensureSetupToken(path string) string {
	if path == "" {
		// No config file store — a Config built directly rather than through
		// LoadConfig. Issue a token for this process and persist nothing;
		// there is nowhere to persist it to.
		return newSetupToken()
	}
	file, err := loadConfigFile(path)
	if err == nil && file != nil && file.SetupToken != nil && *file.SetupToken != "" {
		return *file.SetupToken
	}

	token := newSetupToken()
	if file == nil {
		file = &configFile{}
	}
	file.SetupToken = &token
	if err := writeConfigFile(path, file); err != nil {
		log.Printf("setup: could not persist the setup token to %s (%v); it will change on restart", path, err)
	}
	return token
}

// newSetupToken generates a token. A failure from crypto/rand means the system
// has no usable entropy source; continuing would authorize configuration
// changes with a predictable token, which is worse than not starting.
func newSetupToken() string {
	buf := make([]byte, setupTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("setup: cannot generate a setup token: %v", err)
	}
	return hex.EncodeToString(buf)
}

// logSetupToken prints the setup token at startup.
//
// This is the one credential deliberately written to the log: the log is the
// delivery mechanism, and there is no account system to deliver it any other
// way. Do not "fix" this by masking it.
func logSetupToken(token string) {
	log.Printf("setup: token for configuration changes: %s", token)
	log.Print("setup: keep this private — it authorizes changes to this server's configuration")
}

// authorizeSetup reports whether a request carries the setup token.
//
// The comparison is constant-time. A byte-by-byte comparison that returns early
// leaks the token one position at a time to anyone who can measure the
// response, which over a LAN is entirely practical.
func (s *Server) authorizeSetup(r *http.Request) bool {
	if s.setupToken == "" {
		return false
	}
	header := r.Header.Get("Authorization")
	presented, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.setupToken)) == 1
}

// writeConfigFile serializes the config file to disk atomically at mode 0600.
//
// The write goes to a temporary file in the same directory and is renamed over
// the target, because a rename within one filesystem is atomic while a direct
// write is not. A crash midway through a direct write would leave a truncated
// file, and the next start would fail to parse it — the application would have
// destroyed its own configuration.
func writeConfigFile(path string, cf *configFile) error {
	data, err := yaml.Marshal(cf)
	if err != nil {
		return fmt.Errorf("config file %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	// 0700: the directory holds credentials, so it should not be listable by
	// anyone else either.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("config file %s: %w", path, err)
	}
	tmpName := tmp.Name()
	// Remove the temporary file on any path that does not reach the rename.
	// Leaving .config-*.yaml files behind in a config directory is its own
	// small mess.
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("config file %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("config file %s: %w", path, err)
	}
	// fsync before rename: the rename can otherwise be durable while the
	// contents are not, leaving an empty file after a power loss.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("config file %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config file %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("config file %s: %w", path, err)
	}
	return nil
}
