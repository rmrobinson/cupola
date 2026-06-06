package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesGooglePollenConfigAndExpandsEnv(t *testing.T) {
	t.Setenv("GOOGLE_POLLEN_API_KEY", "env-key")
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte(`
location:
  lat: 43.45
  lon: -80.49
  timezone: America/Toronto
collectors:
  pollen_google:
    enabled: true
    api_key: "${GOOGLE_POLLEN_API_KEY}"
    poll_interval: 12h
    days: 5
    language_code: en-CA
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := cfg.Collectors.PollenGoogle
	if got == nil || !got.Enabled {
		t.Fatalf("pollen config = %+v, want enabled", got)
	}
	if got.APIKey != "env-key" || got.PollInterval.Duration.String() != "12h0m0s" || got.Days != 5 || got.LanguageCode != "en-CA" {
		t.Fatalf("unexpected pollen config: %+v", got)
	}
}
