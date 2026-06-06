package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rmrobinson/cupola/internal/collector"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

type testCollector struct {
	id     string
	domain domain.DomainType
}

func (c testCollector) ID() string                  { return c.id }
func (c testCollector) Domain() domain.DomainType   { return c.domain }
func (c testCollector) Start(context.Context) error { return nil }
func (c testCollector) State() domain.DomainState   { return nil }

func TestExportProfileReturnsDashboardEnvelope(t *testing.T) {
	db, err := store.NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	profile := store.Profile{
		ID:          "home",
		Name:        "Home",
		Layout:      "landscape",
		GridVersion: 2,
		Widgets: []store.WidgetConfig{{
			ID:   "notes",
			Type: "shared-notes",
			Pos:  store.WidgetPos{Col: 0, Row: 0, W: 8, H: 8},
		}},
	}
	if err := db.UpsertProfile(&profile); err != nil {
		t.Fatalf("UpsertProfile() error = %v", err)
	}

	h := &Handler{db: db, registry: collector.NewRegistry()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/home/export", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var export dashboardExport
	if err := json.NewDecoder(rr.Body).Decode(&export); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if export.Kind != dashboardExportKind || export.Version != dashboardExportVersion {
		t.Fatalf("unexpected export metadata: %#v", export)
	}
	if export.Profile.ID != "home" || export.Profile.Widgets[0].Type != "shared-notes" {
		t.Fatalf("unexpected profile export: %#v", export.Profile)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "cupola-dashboard-home.json") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
}

func TestValidateProfileImportClassifiesUniversalAndMissingDomainWidgets(t *testing.T) {
	reg := collector.NewRegistry()
	reg.Register(testCollector{id: "astro", domain: domain.DomainAstro})
	h := &Handler{registry: reg}

	export := dashboardExport{
		Kind:    dashboardExportKind,
		Version: dashboardExportVersion,
		Profile: store.Profile{
			Name:   "Imported",
			Layout: "landscape",
			Widgets: []store.WidgetConfig{
				{ID: "clock", Type: "clock", Pos: store.WidgetPos{W: 5, H: 2}},
				{ID: "weather", Type: "weather-current", Pos: store.WidgetPos{W: 4, H: 4}},
			},
		},
	}
	body, _ := json.Marshal(importValidationRequest{Export: export})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/import/validate", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp importValidationResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode validation: %v", err)
	}
	if resp.Widgets[0].Status != "ok" {
		t.Fatalf("clock status = %q, want ok", resp.Widgets[0].Status)
	}
	if resp.Widgets[1].Status != "missing_domain" {
		t.Fatalf("weather status = %q, want missing_domain", resp.Widgets[1].Status)
	}
}

func TestValidateProfileImportClassifiesAggregateWeatherWidgetPartialDomains(t *testing.T) {
	reg := collector.NewRegistry()
	reg.Register(testCollector{id: "weather", domain: domain.DomainWeatherCurrent})
	h := &Handler{registry: reg}

	export := dashboardExport{
		Kind:    dashboardExportKind,
		Version: dashboardExportVersion,
		Profile: store.Profile{
			Name:   "Imported",
			Layout: "landscape",
			Widgets: []store.WidgetConfig{
				{ID: "current", Type: "weather-current-aggregate", Pos: store.WidgetPos{W: 6, H: 4}},
				{ID: "forecast", Type: "weather-forecast-aggregate", Pos: store.WidgetPos{W: 8, H: 7}},
			},
		},
	}
	body, _ := json.Marshal(importValidationRequest{Export: export})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/import/validate", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp importValidationResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode validation: %v", err)
	}
	if resp.Widgets[0].Status != "config_warning" {
		t.Fatalf("current aggregate status = %q, want config_warning", resp.Widgets[0].Status)
	}
	if resp.Widgets[1].Status != "missing_domain" {
		t.Fatalf("forecast aggregate status = %q, want missing_domain", resp.Widgets[1].Status)
	}
}

func TestImportProfileSkipsUnavailableAndUnselectedWidgets(t *testing.T) {
	db, err := store.NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	h := &Handler{db: db, registry: collector.NewRegistry()}
	export := dashboardExport{
		Kind:    dashboardExportKind,
		Version: dashboardExportVersion,
		Profile: store.Profile{
			Name:        "Imported",
			Layout:      "landscape",
			GridVersion: 2,
			Widgets: []store.WidgetConfig{
				{ID: "clock", Type: "clock", Pos: store.WidgetPos{W: 5, H: 2}},
				{ID: "weather", Type: "weather-current", Pos: store.WidgetPos{W: 4, H: 4}},
				{ID: "notes", Type: "shared-notes", Pos: store.WidgetPos{W: 8, H: 8}},
			},
		},
	}
	body, _ := json.Marshal(importRequest{
		Export:    export,
		Name:      "Copy",
		WidgetIDs: []string{"clock", "weather"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/import", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var resp importResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if len(resp.Profile.Widgets) != 1 || resp.Profile.Widgets[0].Type != "clock" {
		t.Fatalf("imported widgets = %#v, want only clock", resp.Profile.Widgets)
	}
	if resp.Profile.ID == "" || resp.Profile.ID == "Imported" {
		t.Fatalf("unexpected imported profile id: %q", resp.Profile.ID)
	}
}
