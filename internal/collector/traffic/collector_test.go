package traffic

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

func TestNewSourcesBuildsConfigured511Provider(t *testing.T) {
	sources, skipped := NewSources([]SourceSpec{{
		ID:                "ab511",
		Type:              "511",
		Province:          "AB",
		PublicURL:         "https://511.alberta.ca",
		IncidentsURL:      "https://example.test/ab/events",
		CamerasURL:        "https://example.test/ab/cameras",
		RoadConditionsURL: "https://example.test/ab/roads",
	}})

	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped sources: %v", skipped)
	}
	if len(sources.Incidents) != 1 || len(sources.Cameras) != 1 || len(sources.RoadConditions) != 1 {
		t.Fatalf("unexpected source counts: incidents=%d cameras=%d road=%d",
			len(sources.Incidents), len(sources.Cameras), len(sources.RoadConditions))
	}
	if sources.Incidents[0].ID() != "ab511" ||
		sources.Cameras[0].ID() != "ab511" ||
		sources.RoadConditions[0].ID() != "ab511" {
		t.Fatalf("configured 511 source ID not preserved")
	}
}

func TestNewSourcesSkips511ProviderWithoutURLs(t *testing.T) {
	sources, skipped := NewSources([]SourceSpec{{
		ID:       "ab511",
		Type:     "511",
		Province: "AB",
	}})

	if len(sources.Incidents) != 0 || len(sources.Cameras) != 0 || len(sources.RoadConditions) != 0 {
		t.Fatalf("expected no sources, got %+v", sources)
	}
	if len(skipped) != 1 {
		t.Fatalf("expected one skipped source, got %v", skipped)
	}
}
