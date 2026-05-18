package store

import "testing"

func TestTransitAgencyCRUD(t *testing.T) {
	s, err := NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer func() { _ = s.db.Close() }()

	cfg := TransitAgencyConfig{
		ID:                         "grt",
		Enabled:                    true,
		GTFSStaticURLs:             []string{"https://example.com/static.zip"},
		GTFSRTTripUpdatesURLs:      []string{"https://example.com/trips.pb"},
		GTFSRTVehiclePositionsURLs: []string{"https://example.com/vehicles.pb"},
		GTFSRTAlertsURL:            "https://example.com/alerts.pb",
	}
	if err := s.CreateTransitAgency(cfg); err != nil {
		t.Fatalf("CreateTransitAgency() error = %v", err)
	}

	got, err := s.GetTransitAgency("grt")
	if err != nil {
		t.Fatalf("GetTransitAgency() error = %v", err)
	}
	if got == nil || got.ID != "grt" || !got.Enabled || got.GTFSRTAlertsURL != cfg.GTFSRTAlertsURL {
		t.Fatalf("GetTransitAgency() = %#v", got)
	}
	if len(got.GTFSStaticURLs) != 1 || got.GTFSStaticURLs[0] != cfg.GTFSStaticURLs[0] {
		t.Fatalf("GTFSStaticURLs = %#v", got.GTFSStaticURLs)
	}

	got.Enabled = false
	got.GTFSRTAlertsURL = ""
	if err := s.UpdateTransitAgency(*got); err != nil {
		t.Fatalf("UpdateTransitAgency() error = %v", err)
	}
	got, err = s.GetTransitAgency("grt")
	if err != nil {
		t.Fatalf("GetTransitAgency() after update error = %v", err)
	}
	if got.Enabled || got.GTFSRTAlertsURL != "" {
		t.Fatalf("updated agency = %#v", got)
	}

	if err := s.DeleteTransitAgency("grt"); err != nil {
		t.Fatalf("DeleteTransitAgency() error = %v", err)
	}
	got, err = s.GetTransitAgency("grt")
	if err != nil {
		t.Fatalf("GetTransitAgency() after delete error = %v", err)
	}
	if got != nil {
		t.Fatalf("GetTransitAgency() after delete = %#v", got)
	}
}
