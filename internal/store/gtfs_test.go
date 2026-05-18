package store

import (
	"testing"
	"time"
)

func TestHasActiveServiceAllowsCalendarDateOnlyAddition(t *testing.T) {
	s, err := NewGTFSSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewGTFSSQLiteStore() error = %v", err)
	}
	if err := s.ReplaceGTFSAgency(
		"test",
		[]GTFSStopTime{{RouteID: "R1", TripID: "T1", StopID: "S1", Headsign: "Downtown", ServiceID: "EXTRA", StopSequence: 1, DepartureSecs: 9 * 3600}},
		nil,
		[]GTFSServiceException{{ServiceID: "EXTRA", Date: "20260105", Added: true}},
		nil,
	); err != nil {
		t.Fatalf("ReplaceGTFSAgency() error = %v", err)
	}

	ok, err := s.HasActiveService("test", time.Date(2026, time.January, 5, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("HasActiveService() error = %v", err)
	}
	if !ok {
		t.Fatal("HasActiveService() = false, want true")
	}
}
