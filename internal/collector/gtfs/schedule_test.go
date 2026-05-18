package gtfs

import "testing"

func TestNewScheduleHasNoRoutes(t *testing.T) {
	s := New()
	if s.HasRoutes() {
		t.Fatal("HasRoutes() = true, want false for a fresh schedule")
	}
}
