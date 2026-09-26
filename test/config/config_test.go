package config_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/config"
)

// clearEnv blanks every variable Load reads so the host environment can't
// leak into a test. Load treats empty values as unset.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "RATES_API_URL", "ALLOWED_ORIGINS", "CACHE_TTL", "UPSTREAM_TIMEOUT"} {
		t.Setenv(key, "")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	want := config.Config{
		Port:            "8080",
		UpstreamURL:     "https://api.frankfurter.dev/v2",
		UpstreamTimeout: 10 * time.Second,
		CacheTTL:        time.Hour,
		AllowedOrigins:  []string{"http://localhost:5173"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoad_Overrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", " 9000 ")
	t.Setenv("RATES_API_URL", "https://example.test/v2/")
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:5173, https://app.example.com ,,")
	t.Setenv("CACHE_TTL", "30m")
	t.Setenv("UPSTREAM_TIMEOUT", "2s")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	want := config.Config{
		Port:            "9000",
		UpstreamURL:     "https://example.test/v2",
		UpstreamTimeout: 2 * time.Second,
		CacheTTL:        30 * time.Minute,
		AllowedOrigins:  []string{"http://localhost:5173", "https://app.example.com"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoad_InvalidDurations(t *testing.T) {
	tests := []struct {
		key, value string
	}{
		{"CACHE_TTL", "an hour"},
		{"CACHE_TTL", "0s"},
		{"CACHE_TTL", "-5m"},
		{"UPSTREAM_TIMEOUT", "10"},
		{"UPSTREAM_TIMEOUT", "-1s"},
	}
	for _, tc := range tests {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(tc.key, tc.value)

			if _, err := config.Load(); err == nil {
				t.Errorf("expected error for %s=%q", tc.key, tc.value)
			}
		})
	}
}
