package traffic511

import (
	"testing"

	"github.com/rmrobinson/cupola/internal/domain"
)

func TestDedupeTrafficIncidentsKeepsFirstDuplicateID(t *testing.T) {
	incidents := []domain.TrafficIncident{
		{ID: "a", RoadName: "FIRST"},
		{ID: "b", RoadName: "SECOND"},
		{ID: "a", RoadName: "DUPLICATE"},
		{ID: "c", RoadName: "THIRD"},
	}

	got, duplicateCount := dedupeTrafficIncidents(incidents)
	if duplicateCount != 1 {
		t.Fatalf("expected 1 duplicate, got %d", duplicateCount)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 incidents, got %d: %+v", len(got), got)
	}
	if got[0].RoadName != "FIRST" || got[1].RoadName != "SECOND" || got[2].RoadName != "THIRD" {
		t.Fatalf("dedupe did not preserve order and first occurrence: %+v", got)
	}
}

func TestDedupeTrafficIncidentsPreservesEmptyIDs(t *testing.T) {
	incidents := []domain.TrafficIncident{
		{RoadName: "NO ID 1"},
		{ID: "a", RoadName: "WITH ID"},
		{RoadName: "NO ID 2"},
		{ID: "a", RoadName: "DUPLICATE"},
	}

	got, duplicateCount := dedupeTrafficIncidents(incidents)
	if duplicateCount != 1 {
		t.Fatalf("expected 1 duplicate, got %d", duplicateCount)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 incidents, got %d: %+v", len(got), got)
	}
	if got[0].RoadName != "NO ID 1" || got[1].RoadName != "WITH ID" || got[2].RoadName != "NO ID 2" {
		t.Fatalf("dedupe should preserve empty-ID incidents: %+v", got)
	}
}

func TestDedupeTrafficIncidentsReturnsOriginalWhenNoDuplicates(t *testing.T) {
	incidents := []domain.TrafficIncident{
		{ID: "a", RoadName: "FIRST"},
		{ID: "b", RoadName: "SECOND"},
	}

	got, duplicateCount := dedupeTrafficIncidents(incidents)
	if duplicateCount != 0 {
		t.Fatalf("expected 0 duplicates, got %d", duplicateCount)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected incidents: %+v", got)
	}
}
