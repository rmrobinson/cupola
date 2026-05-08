package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rmrobinson/cupola/internal/collector/gtfsrt"
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

func TestTransitRouteShapeReturnsServiceUnavailableBeforeStaticMetadataLoads(t *testing.T) {
	db, err := store.NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}

	if err := db.CreateTransitAgency(store.TransitAgencyConfig{
		ID:             "test",
		Enabled:        true,
		GTFSStaticURLs: []string{"https://example.com/static.zip"},
	}); err != nil {
		t.Fatalf("CreateTransitAgency() error = %v", err)
	}

	agencies, err := gtfsrt.NewAgencyManager(db, t.TempDir())
	if err != nil {
		t.Fatalf("NewAgencyManager() error = %v", err)
	}

	h := &Handler{agencies: agencies}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transit/agencies/test/routes/1/shape", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusServiceUnavailable, rr.Body.String())
	}
}
