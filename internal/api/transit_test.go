package api

import (
	"testing"

	"github.com/rmrobinson/cupola/internal/store"
)

func TestValidateTransitAgencyConfigAllowsStaticOnlyEnabledAgency(t *testing.T) {
	err := validateTransitAgencyConfig(store.TransitAgencyConfig{
		ID:                    "static-only",
		Enabled:               true,
		GTFSStaticURLs:        []string{"https://example.com/static.zip"},
		GTFSRTTripUpdatesURLs: []string{},
	}, true)
	if err != nil {
		t.Fatalf("validateTransitAgencyConfig() error = %v", err)
	}
}

func TestValidateTransitAgencyConfigStillRequiresStaticURLsForEnabledAgency(t *testing.T) {
	err := validateTransitAgencyConfig(store.TransitAgencyConfig{
		ID:                    "no-static",
		Enabled:               true,
		GTFSRTTripUpdatesURLs: []string{"https://example.com/trips.pb"},
	}, true)
	if err == nil {
		t.Fatal("validateTransitAgencyConfig() error = nil, want error")
	}
}
