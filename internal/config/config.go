// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime settings for the API server.
type Config struct {
	// Port the HTTP server listens on.
	Port string
	// UpstreamURL is the base URL of the Frankfurter API.
	UpstreamURL string
	// UpstreamTimeout bounds each call to the upstream API.
	UpstreamTimeout time.Duration
	// CacheTTL is how long a fetched rate table is considered fresh.
	CacheTTL time.Duration
	// AllowedOrigins lists origins permitted by CORS. "*" allows any origin.
	AllowedOrigins []string
}

// Load reads configuration from the environment, applying defaults for
// anything that is unset.
func Load() (Config, error) {
	cfg := Config{
		Port:           getEnv("PORT", "8080"),
		UpstreamURL:    strings.TrimRight(getEnv("RATES_API_URL", "https://api.frankfurter.dev/v2"), "/"),
		AllowedOrigins: splitList(getEnv("ALLOWED_ORIGINS", "http://localhost:5173")),
	}

	var err error
	if cfg.CacheTTL, err = getDuration("CACHE_TTL", time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.UpstreamTimeout, err = getDuration("UPSTREAM_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := getEnv(key, "")
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: invalid %s %q: %w", key, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("config: %s must be positive, got %s", key, d)
	}
	return d, nil
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
