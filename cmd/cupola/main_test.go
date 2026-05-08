package main

import (
	"testing"

	"github.com/rmrobinson/cupola/internal/config"
)

func TestValidateYAMLTransitAgencyAllowsStaticOnlyAgency(t *testing.T) {
	err := validateYAMLTransitAgency(config.TransitAgencyConfig{
		ID:                    "static-only",
		GTFSStaticURLs:        []string{"https://example.com/static.zip"},
		GTFSRTTripUpdatesURLs: []string{},
	})
	if err != nil {
		t.Fatalf("validateYAMLTransitAgency() error = %v", err)
	}
}

func TestValidateYAMLTransitAgencyStillRequiresStaticURLs(t *testing.T) {
	err := validateYAMLTransitAgency(config.TransitAgencyConfig{
		ID:                    "no-static",
		GTFSRTTripUpdatesURLs: []string{"https://example.com/trips.pb"},
	})
	if err == nil {
		t.Fatal("validateYAMLTransitAgency() error = nil, want error")
	}
}
