package enovapower

import (
	"net/http"
	"testing"
)

func TestBuildTitle(t *testing.T) {
	tests := []struct {
		cause   string
		planned bool
		want    string
	}{
		{"", false, "Enova Power Unplanned Outage"},
		{"unknown", false, "Enova Power Unplanned Outage"},
		{"Unknown", false, "Enova Power Unplanned Outage"},
		{"UNKNOWN", false, "Enova Power Unplanned Outage"},
		{"under investigation", false, "Enova Power Unplanned Outage"},
		{"Under Investigation", false, "Enova Power Unplanned Outage"},
		{"Equipment Failure", false, "Enova Power Unplanned Equipment Failure"},
		{"Equipment Failure", true, "Enova Power Planned Equipment Failure"},
		{"", true, "Enova Power Planned Outage"},
	}
	for _, tt := range tests {
		got := buildTitle(omsCase{DescCause: tt.cause}, tt.planned)
		if got != tt.want {
			t.Errorf("buildTitle(%q, planned=%v) = %q, want %q", tt.cause, tt.planned, got, tt.want)
		}
	}
}

func TestBuildDescription(t *testing.T) {
	tests := []struct {
		c    omsCase
		want string
	}{
		{omsCase{}, ""},
		// PublicMsg takes priority over DescCause
		{omsCase{PublicMsg: "Maintenance work", DescCause: "Equipment Failure"}, "Maintenance work"},
		// Meaningful DescCause used when PublicMsg absent
		{omsCase{DescCause: "Equipment Failure"}, "Equipment Failure"},
		// Suppressed causes produce no text
		{omsCase{DescCause: "Unknown"}, ""},
		{omsCase{DescCause: "under investigation"}, ""},
		// CurCust > 0
		{omsCase{CurCust: "15"}, "15 customers affected"},
		// CurCust = 0 falls back to InitCust
		{omsCase{CurCust: "0", InitCust: "42"}, "42 customers initially affected"},
		// Both zero — no customer line
		{omsCase{CurCust: "0", InitCust: "0"}, ""},
		// RestRange appended
		{omsCase{CurCust: "5", RestRange: "Within 2 hours"}, "5 customers affected. Estimated restore: Within 2 hours"},
		// All parts combined
		{omsCase{PublicMsg: "Work ongoing", CurCust: "10", RestRange: "1 hour"}, "Work ongoing. 10 customers affected. Estimated restore: 1 hour"},
	}
	for _, tt := range tests {
		got := buildDescription(tt.c)
		if got != tt.want {
			t.Errorf("buildDescription(%+v) = %q, want %q", tt.c, got, tt.want)
		}
	}
}

func TestParseCoordList(t *testing.T) {
	tests := []struct {
		input string
		want  [][]float64
	}{
		{"", nil},
		// Odd field count → nil
		{"43.44", nil},
		// Single pair: OMS is lat,lon → stored as [lon, lat]
		{"43.44,-80.55", [][]float64{{-80.55, 43.44}}},
		// Two pairs
		{"43.44,-80.55,43.45,-80.56", [][]float64{{-80.55, 43.44}, {-80.56, 43.45}}},
		// Whitespace tolerance
		{" 43.44 , -80.55 ", [][]float64{{-80.55, 43.44}}},
	}
	for _, tt := range tests {
		got := parseCoordList(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseCoordList(%q): got %d points, want %d", tt.input, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i][0] != tt.want[i][0] || got[i][1] != tt.want[i][1] {
				t.Errorf("parseCoordList(%q)[%d] = %v, want %v", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestCasesToAlertsUsesLatestDuplicateSerial(t *testing.T) {
	alerts := casesToAlerts(&http.Client{}, []omsCase{
		{
			Serial:    "12256",
			Planned:   "0",
			DescCause: "Vehicle",
			CurCust:   "3",
			CoordList: "43.423176,-80.431173,43.424198,-80.424435,43.425000,-80.425000",
		},
		{
			Serial:    "12256",
			Planned:   "0",
			DescCause: "Vehicle",
			CurCust:   "1",
			CoordList: "43.423354,-80.431432,43.423354,-80.430722",
		},
		{
			Serial:    "12258",
			Planned:   "0",
			DescCause: "Vehicle",
			CurCust:   "6",
		},
	})

	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2", len(alerts))
	}
	if alerts[0].ID != "enova.power:12256" {
		t.Fatalf("first alert ID = %q, want enova.power:12256", alerts[0].ID)
	}
	if alerts[0].Description != "Vehicle. 1 customers affected" {
		t.Fatalf("deduped alert description = %q, want latest record", alerts[0].Description)
	}
	if len(alerts[0].Polygon) != 2 {
		t.Fatalf("deduped alert polygon has %d points, want latest 2-point polygon", len(alerts[0].Polygon))
	}
}

func TestParseLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test")
	}
	p := &Parser{}
	alerts, err := p.Parse("https://oms.enovapower.com/Outages/")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	t.Logf("got %d alerts", len(alerts))
	for i, a := range alerts {
		area := ""
		if a.Area != nil {
			area = *a.Area
		}
		t.Logf("  [%d] %s | %s | area=%q | desc=%q | poly_pts=%d",
			i, a.AlertType, a.Title, area, a.Description, len(a.Polygon))
		if len(a.Polygon) > 0 {
			t.Logf("      first=[%.6f,%.6f] last=[%.6f,%.6f]",
				a.Polygon[0][0], a.Polygon[0][1],
				a.Polygon[len(a.Polygon)-1][0], a.Polygon[len(a.Polygon)-1][1])
		}
	}
}
