package traffic

import (
	"strings"
	"testing"

	"github.com/rmrobinson/cupola/internal/domain"
)

func TestParseRegionWaterlooClosuresBuildsTrafficIncidentsAndEmergencyAlerts(t *testing.T) {
	body := `{
		"type": "FeatureCollection",
		"features": [
			{
				"type": "Feature",
				"id": 1,
				"geometry": {"type": "LineString", "coordinates": [[-80.1, 43.1], [-80.2, 43.2]]},
				"properties": {
					"OBJECTID": 1,
					"GlobalID": "{ABC-123}",
					"STREET_NAME": "Kressler Rd",
					"STREET_FROM": "Erbsville Rd",
					"STREET_TO": "Benjamin Rd",
					"MUNICIPALITY": "Woolwich",
					"DATE_FROM": 1780574400000,
					"DATE_TO": null,
					"REASON": "Emergency",
					"STATUS": "Closed",
					"CLOSURE_SCHEDULED": "Existing",
					"DETAILS": "Watermain issue",
					"DETOUR": "Use local roads",
					"ORGANIZATION": "Region of Waterloo",
					"CONTACT": "Transportation",
					"OPEN_STATUS": "Road Closed",
					"Description": "Kressler Rd from Erbsville Rd to Benjamin Rd",
					"EMERGENCY_REASON": "Washout",
					"Create_Date": 1780401600000
				}
			},
			{
				"type": "Feature",
				"id": 2,
				"geometry": {"type": "MultiLineString", "coordinates": [[[-80.3, 43.3], [-80.4, 43.4]]]},
				"properties": {
					"OBJECTID": 2,
					"GlobalID": "{DEF-456}",
					"STREET_NAME": "King St",
					"MUNICIPALITY": "Waterloo",
					"DATE_FROM": 1780574400000,
					"DATE_TO": 1780660800000,
					"REASON": "Utility Work",
					"STATUS": "Lane Reduced",
					"CLOSURE_SCHEDULED": "Existing",
					"DETAILS": "Expect delays",
					"OPEN_STATUS": "Road Closed",
					"Description": "King St near Erb St"
				}
			}
		]
	}`

	incidents, alerts, err := parseRegionWaterlooClosures(strings.NewReader(body), "https://example.test/RegionalClosures/MapServer")
	if err != nil {
		t.Fatalf("parseRegionWaterlooClosures() error = %v", err)
	}
	if len(incidents) != 2 {
		t.Fatalf("got %d incidents, want 2: %+v", len(incidents), incidents)
	}
	if incidents[0].ID != "region-waterloo-roadclosures:abc-123" {
		t.Fatalf("unexpected first ID: %q", incidents[0].ID)
	}
	if incidents[0].Severity != "major" || incidents[0].Type != "closure" {
		t.Fatalf("unexpected emergency incident: %+v", incidents[0])
	}
	if incidents[0].Lat != 43.1 || incidents[0].Lon != -80.1 {
		t.Fatalf("first coordinate not exposed for sorting: %+v", incidents[0])
	}
	if len(incidents[0].Lines) != 1 || len(incidents[0].Lines[0]) != 2 {
		t.Fatalf("LineString geometry not preserved: %+v", incidents[0].Lines)
	}
	if incidents[1].Severity != "moderate" {
		t.Fatalf("lane reduction severity = %q, want moderate", incidents[1].Severity)
	}
	if len(incidents[1].Lines) != 1 || incidents[1].Lines[0][0][0] != -80.3 {
		t.Fatalf("MultiLineString geometry not preserved: %+v", incidents[1].Lines)
	}

	if len(alerts) != 1 {
		t.Fatalf("got %d promoted alerts, want 1: %+v", len(alerts), alerts)
	}
	if alerts[0].ID != "region.waterloo.roadclosures:abc-123" ||
		alerts[0].Severity != domain.SeverityWarning ||
		alerts[0].AlertType != "road-closure" {
		t.Fatalf("unexpected promoted alert: %+v", alerts[0])
	}
	if alerts[0].PublishedAt.IsZero() {
		t.Fatalf("expected Create_Date to become PublishedAt: %+v", alerts[0])
	}
}

func TestParseRegionWaterlooClosuresSkipsNonClosureRecords(t *testing.T) {
	body := `{
		"type": "FeatureCollection",
		"features": [
			{
				"type": "Feature",
				"geometry": {"type": "LineString", "coordinates": [[-80.1, 43.1]]},
				"properties": {"OBJECTID": 1, "STATUS": "No Closure", "OPEN_STATUS": "Road Closed"}
			},
			{
				"type": "Feature",
				"geometry": {"type": "LineString", "coordinates": [[-80.2, 43.2]]},
				"properties": {"OBJECTID": 2, "STATUS": "Closed", "OPEN_STATUS": "Road Reopened"}
			},
			{
				"type": "Feature",
				"geometry": {"type": "LineString", "coordinates": [[-80.3, 43.3]]},
				"properties": {"OBJECTID": 3, "STATUS": "Closed", "OPEN_STATUS": "Postponed"}
			},
			{
				"type": "Feature",
				"geometry": {"type": "LineString", "coordinates": [[-80.4, 43.4]]},
				"properties": {"OBJECTID": 4, "STATUS": "Closed", "OPEN_STATUS": "Cancelled"}
			}
		]
	}`

	incidents, alerts, err := parseRegionWaterlooClosures(strings.NewReader(body), "https://example.test")
	if err != nil {
		t.Fatalf("parseRegionWaterlooClosures() error = %v", err)
	}
	if len(incidents) != 0 || len(alerts) != 0 {
		t.Fatalf("expected all records skipped, got incidents=%+v alerts=%+v", incidents, alerts)
	}
}

func TestParseRegionWaterlooClosuresDedupesByStableID(t *testing.T) {
	body := `{
		"type": "FeatureCollection",
		"features": [
			{
				"type": "Feature",
				"geometry": {"type": "LineString", "coordinates": [[-80.1, 43.1]]},
				"properties": {
					"OBJECTID": 1,
					"GlobalID": "{ABC-123}",
					"STREET_NAME": "Kressler Rd",
					"REASON": "Emergency",
					"STATUS": "Closed",
					"OPEN_STATUS": "Road Closed"
				}
			},
			{
				"type": "Feature",
				"geometry": {"type": "LineString", "coordinates": [[-80.2, 43.2]]},
				"properties": {
					"OBJECTID": 2,
					"GlobalID": "{ABC-123}",
					"STREET_NAME": "Duplicate",
					"REASON": "Emergency",
					"STATUS": "Closed",
					"OPEN_STATUS": "Road Closed"
				}
			}
		]
	}`

	incidents, alerts, err := parseRegionWaterlooClosures(strings.NewReader(body), "https://example.test")
	if err != nil {
		t.Fatalf("parseRegionWaterlooClosures() error = %v", err)
	}
	if len(incidents) != 1 {
		t.Fatalf("expected duplicate incident to be dropped, got %+v", incidents)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected duplicate promoted alert to be dropped, got %+v", alerts)
	}
	if incidents[0].RoadName != "Kressler Rd" || alerts[0].Area == nil || *alerts[0].Area != "Kressler Rd" {
		t.Fatalf("dedupe did not keep first record: incidents=%+v alerts=%+v", incidents, alerts)
	}
}
