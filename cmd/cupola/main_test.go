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
