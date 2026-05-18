package store

import (
	"testing"
	"time"
)

func TestStateStoreRetainsSystemHealth(t *testing.T) {
	s := NewStateStore()
	errAt := time.Date(2026, time.May, 8, 12, 0, 0, 0, time.UTC)
	okAt := errAt.Add(time.Minute)

	s.PublishSystem(SystemEvent{
		CollectorID: "collector-a",
		Status:      "error",
		Message:     "failed",
		At:          errAt,
	})

	snap, ok := s.GetSystem("collector-a")
	if !ok {
		t.Fatal("expected retained system snapshot")
	}
	if snap.Status != "error" || snap.Message != "failed" || !snap.LastEventAt.Equal(errAt) {
		t.Fatalf("unexpected error snapshot: %+v", snap)
	}
	if !snap.LastSuccessAt.IsZero() {
		t.Fatalf("LastSuccessAt should be zero after error: %v", snap.LastSuccessAt)
	}

	s.PublishSystem(SystemEvent{
		CollectorID: "collector-a",
		Status:      "ok",
		At:          okAt,
	})

	snap, ok = s.GetSystem("collector-a")
	if !ok {
		t.Fatal("expected retained system snapshot after ok")
	}
	if snap.Status != "ok" || snap.Message != "" || !snap.LastEventAt.Equal(okAt) || !snap.LastSuccessAt.Equal(okAt) {
		t.Fatalf("unexpected ok snapshot: %+v", snap)
	}
}
