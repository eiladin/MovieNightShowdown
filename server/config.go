package server

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds server configuration loaded from environment variables.
type Config struct {
	JellyfinURL    string
	JellyfinAPIKey string
	JellyfinUserID string
	PublicURL      string
	Port           string
	SessionTTL     string
	CacheDir       string
}

// LoadConfig reads configuration from environment variables, applying
// defaults for the optional ones.
func LoadConfig() Config {
	cfg := Config{
		JellyfinURL:    os.Getenv("JELLYFIN_URL"),
		JellyfinAPIKey: os.Getenv("JELLYFIN_API_KEY"),
		JellyfinUserID: os.Getenv("JELLYFIN_USER_ID"),
		PublicURL:      os.Getenv("PUBLIC_URL"),
		Port:           os.Getenv("PORT"),
		SessionTTL:     os.Getenv("SESSION_TTL"),
		CacheDir:       os.Getenv("CACHE_DIR"),
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = "http://localhost:8080"
	}
	if cfg.SessionTTL == "" {
		cfg.SessionTTL = "4h"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(os.TempDir(), "mns-posters")
	}
	return cfg
}

// String renders the config for logging with the API key masked.
func (c Config) String() string {
	masked := "(unset)"
	if c.JellyfinAPIKey != "" {
		masked = "***"
	}
	return fmt.Sprintf(
		"JellyfinURL=%s JellyfinAPIKey=%s JellyfinUserID=%s PublicURL=%s Port=%s SessionTTL=%s CacheDir=%s",
		c.JellyfinURL, masked, c.JellyfinUserID, c.PublicURL, c.Port, c.SessionTTL, c.CacheDir,
	)
}
