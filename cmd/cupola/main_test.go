package main

import (
	"testing"
	"time"

	"github.com/rmrobinson/cupola/internal/config"
)

func TestResolveAirQualityConfigUsesDedicatedConfig(t *testing.T) {
	got := resolveAirQualityConfig(config.CollectorsConfig{
		AirQualityEnvCanada: &config.EnvCanadaAirQualityConfig{
			Enabled:      true,
			Province:     "BC",
			StationCode:  "bc-station",
			Location:     "Vancouver",
			PollInterval: config.Duration{Duration: 45 * time.Minute},
		},
	})

	if !got.enabled {
		t.Fatalf("enabled = false, want true")
	}
	if got.province != "BC" || got.stationProvince != "BC" || got.stationCode != "bc-station" || got.location != "Vancouver" {
		t.Fatalf("unexpected resolved config: %+v", got)
	}
	if got.interval != 45*time.Minute {
		t.Fatalf("interval = %s, want 45m", got.interval)
	}
}

func TestResolveAirQualityConfigDefaultsInterval(t *testing.T) {
	got := resolveAirQualityConfig(config.CollectorsConfig{
		AirQualityEnvCanada: &config.EnvCanadaAirQualityConfig{Enabled: true},
	})

	if got.interval != 30*time.Minute {
		t.Fatalf("interval = %s, want 30m", got.interval)
	}
}

func TestResolveAirQualityConfigDisabledWhenDedicatedConfigMissing(t *testing.T) {
	got := resolveAirQualityConfig(config.CollectorsConfig{
		WeatherEnvCanada: &config.EnvCanadaWeatherConfig{
			Enabled:     false,
			StationCode: "s0000585",
			Province:    "ON",
		},
	})

	if got.enabled {
		t.Fatalf("enabled = true, want false")
	}
}

func TestResolvePollenConfigDefaultsAndCapsDays(t *testing.T) {
	got := resolvePollenConfig(config.CollectorsConfig{
		PollenGoogle: &config.GooglePollenConfig{
			Enabled:      true,
			APIKey:       " test-key ",
			Days:         9,
			LanguageCode: " en-CA ",
		},
	}, 43.45, -80.49, "America/Toronto")

	if !got.enabled {
		t.Fatalf("enabled = false, want true")
	}
	if got.apiKey != "test-key" {
		t.Fatalf("apiKey = %q, want test-key", got.apiKey)
	}
	if got.opts.Interval != 12*time.Hour {
		t.Fatalf("interval = %s, want 12h", got.opts.Interval)
	}
	if got.opts.Days != 5 {
		t.Fatalf("days = %d, want 5", got.opts.Days)
	}
	if got.opts.LanguageCode != "en-CA" {
		t.Fatalf("language = %q, want en-CA", got.opts.LanguageCode)
	}
}

func TestResolvePollenConfigDisabledWithoutAPIKey(t *testing.T) {
	got := resolvePollenConfig(config.CollectorsConfig{
		PollenGoogle: &config.GooglePollenConfig{Enabled: true},
	}, 43.45, -80.49, "America/Toronto")

	if got.enabled {
		t.Fatalf("enabled = true, want false")
	}
}
