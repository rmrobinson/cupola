package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

const dashboardExportKind = "cupola.dashboard.export"
const dashboardExportVersion = 1

type dashboardExport struct {
	Kind       string        `json:"kind"`
	Version    int           `json:"version"`
	ExportedAt time.Time     `json:"exported_at"`
	Source     exportSource  `json:"source"`
	Profile    store.Profile `json:"profile"`
}

type exportSource struct {
	App string `json:"app"`
}

type importValidationRequest struct {
	Export dashboardExport `json:"export"`
}

type importRequest struct {
	Export      dashboardExport `json:"export"`
	Name        string          `json:"name"`
	WidgetIDs   []string        `json:"widget_ids"`
	WidgetTypes []string        `json:"widget_types"`
}

type importValidationResponse struct {
	ProfileName string               `json:"profile_name"`
	Layout      string               `json:"layout"`
	GridVersion int                  `json:"grid_version,omitempty"`
	Widgets     []widgetImportStatus `json:"widgets"`
	CanImport   bool                 `json:"can_import"`
}

type widgetImportStatus struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Label           string   `json:"label"`
	Status          string   `json:"status"`
	RequiredDomains []string `json:"required_domains,omitempty"`
	MissingDomains  []string `json:"missing_domains,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

type importResponse struct {
	Profile store.Profile        `json:"profile"`
	Skipped []widgetImportStatus `json:"skipped,omitempty"`
}

type widgetImportMeta struct {
	Universal bool
	Domains   []domain.DomainType
}

var widgetImportRegistry = map[string]widgetImportMeta{
	"clock":          {Universal: true},
	"shared-notes":   {Universal: true},
	"moon-phase":     {Universal: true},
	"sunrise-sunset": {Universal: true},

	"weather-current":         {Domains: []domain.DomainType{domain.DomainWeatherCurrent}},
	"weather-forecast":        {Domains: []domain.DomainType{domain.DomainWeatherForecast}},
	"weather-air-quality":     {Domains: []domain.DomainType{domain.DomainWeatherAirQuality}},
	"weather-rainfall":        {Domains: []domain.DomainType{domain.DomainWeatherCurrent}},
	"solar-activity":          {Domains: []domain.DomainType{domain.DomainSolarWeatherCurrent}},
	"solar-forecast":          {Domains: []domain.DomainType{domain.DomainSolarWeatherForecast}},
	"transit":                 {Domains: []domain.DomainType{domain.DomainTransitArrivals}},
	"transit-station":         {Domains: []domain.DomainType{domain.DomainTransitArrivals}},
	"radar-map":               {Domains: []domain.DomainType{domain.DomainTrafficIncidents, domain.DomainTransitVehicles, domain.DomainAircraft}},
	"traffic-incidents":       {Domains: []domain.DomainType{domain.DomainTrafficIncidents}},
	"traffic-cameras":         {Domains: []domain.DomainType{domain.DomainTrafficCameras}},
	"traffic-road-conditions": {Domains: []domain.DomainType{domain.DomainTrafficRoadConditions}},
	"aircraft":                {Domains: []domain.DomainType{domain.DomainAircraft}},
	"flag-status":             {Domains: []domain.DomainType{domain.DomainFlagStatus}},
	"alerts":                  {Domains: []domain.DomainType{domain.DomainWeatherAlerts, domain.DomainTransitAlerts, domain.DomainMunicipalAlerts}},
	"municipal-events":        {Domains: []domain.DomainType{domain.DomainMunicipalEvents}},
	"waterway":                {Domains: []domain.DomainType{domain.DomainWaterwayConditions}},
	"waste-collection":        {Domains: []domain.DomainType{domain.DomainWasteCollection}},
	"shared-rss":              {Domains: []domain.DomainType{domain.DomainFeeds}},
}

func (h *Handler) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.db.ListProfiles()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profiles)
}

func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.db.GetProfile(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (h *Handler) createProfile(w http.ResponseWriter, r *http.Request) {
	var p store.Profile
	if !decodeJSONBody(w, r, &p) {
		return
	}
	if p.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := h.db.UpsertProfile(&p); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) exportProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.db.GetProfile(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.NotFound(w, r)
		return
	}
	export := dashboardExport{
		Kind:       dashboardExportKind,
		Version:    dashboardExportVersion,
		ExportedAt: time.Now().UTC(),
		Source:     exportSource{App: "cupola"},
		Profile:    *p,
	}
	filename := "cupola-dashboard-" + slugifyFilename(p.Name) + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(export)
}

func (h *Handler) validateProfileImport(w http.ResponseWriter, r *http.Request) {
	export, ok := h.decodeDashboardExport(w, r)
	if !ok {
		return
	}
	resp := h.validateDashboardExport(export)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) importProfile(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var req importRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	export := req.Export
	if !validDashboardExport(export) {
		http.Error(w, "invalid dashboard export", http.StatusBadRequest)
		return
	}

	validation := h.validateDashboardExport(export)
	selected := selectedWidgetSet(req.WidgetIDs, req.WidgetTypes)
	profile := export.Profile
	profile.ID = slugifyProfileID(firstNonEmpty(req.Name, profile.Name+" Imported")) + fmt.Sprintf("-%d", time.Now().UnixMilli())
	profile.Name = firstNonEmpty(req.Name, profile.Name+" Imported")

	statusByID := make(map[string]widgetImportStatus, len(validation.Widgets))
	for _, st := range validation.Widgets {
		statusByID[st.ID] = st
	}

	kept := make([]store.WidgetConfig, 0, len(profile.Widgets))
	skipped := make([]widgetImportStatus, 0)
	for _, widget := range profile.Widgets {
		st := statusByID[widget.ID]
		if st.ID == "" {
			st = widgetImportStatus{ID: widget.ID, Type: widget.Type, Label: humanWidgetLabel(widget.Type), Status: "missing_widget_type"}
		}
		if st.Status == "missing_widget_type" || st.Status == "missing_domain" || !selected(widget) {
			skipped = append(skipped, st)
			continue
		}
		kept = append(kept, widget)
	}
	profile.Widgets = kept

	if err := h.db.UpsertProfile(&profile); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(importResponse{Profile: profile, Skipped: skipped})
}

func (h *Handler) deleteProfile(w http.ResponseWriter, r *http.Request) {
	if err := h.db.DeleteProfile(chi.URLParam(r, "id")); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) decodeDashboardExport(w http.ResponseWriter, r *http.Request) (dashboardExport, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return dashboardExport{}, false
	}
	var req importValidationRequest
	if err := json.Unmarshal(body, &req); err == nil && req.Export.Kind != "" {
		if !validDashboardExport(req.Export) {
			http.Error(w, "invalid dashboard export", http.StatusBadRequest)
			return dashboardExport{}, false
		}
		return req.Export, true
	}
	var export dashboardExport
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&export); err != nil || !validDashboardExport(export) {
		http.Error(w, "invalid dashboard export", http.StatusBadRequest)
		return dashboardExport{}, false
	}
	return export, true
}

func validDashboardExport(export dashboardExport) bool {
	return export.Kind == dashboardExportKind &&
		export.Version == dashboardExportVersion &&
		export.Profile.Name != "" &&
		export.Profile.Layout != ""
}

func (h *Handler) validateDashboardExport(export dashboardExport) importValidationResponse {
	available := make(map[domain.DomainType]bool)
	for _, d := range h.registry.Domains() {
		available[d] = true
	}
	widgets := make([]widgetImportStatus, 0, len(export.Profile.Widgets))
	canImport := false
	for _, widget := range export.Profile.Widgets {
		st := validateImportedWidget(widget, available)
		widgets = append(widgets, st)
		if st.Status == "ok" || st.Status == "config_warning" {
			canImport = true
		}
	}
	return importValidationResponse{
		ProfileName: export.Profile.Name,
		Layout:      export.Profile.Layout,
		GridVersion: export.Profile.GridVersion,
		Widgets:     widgets,
		CanImport:   canImport,
	}
}

func validateImportedWidget(widget store.WidgetConfig, available map[domain.DomainType]bool) widgetImportStatus {
	st := widgetImportStatus{ID: widget.ID, Type: widget.Type, Label: humanWidgetLabel(widget.Type), Status: "ok"}
	meta, ok := widgetImportRegistry[widget.Type]
	if !ok {
		st.Status = "missing_widget_type"
		st.Warnings = []string{"This Cupola version does not know this widget type."}
		return st
	}
	if meta.Universal {
		return st
	}
	present := 0
	for _, d := range meta.Domains {
		ds := string(d)
		st.RequiredDomains = append(st.RequiredDomains, ds)
		if available[d] {
			present++
		} else {
			st.MissingDomains = append(st.MissingDomains, ds)
		}
	}
	if present == 0 {
		st.Status = "missing_domain"
		st.Warnings = []string{"Required data is not available on this Cupola instance."}
	} else if len(st.MissingDomains) > 0 {
		st.Status = "config_warning"
		st.Warnings = []string{"Some optional data used by this widget is not available on this Cupola instance."}
	}
	return st
}

func selectedWidgetSet(ids, types []string) func(store.WidgetConfig) bool {
	idSet := make(map[string]bool, len(ids))
	typeSet := make(map[string]bool, len(types))
	for _, id := range ids {
		idSet[id] = true
	}
	for _, t := range types {
		typeSet[t] = true
	}
	if ids == nil && types == nil {
		return func(store.WidgetConfig) bool { return true }
	}
	return func(w store.WidgetConfig) bool {
		return idSet[w.ID] || typeSet[w.Type]
	}
}

func slugifyFilename(s string) string {
	slug := slugifyProfileID(s)
	if slug == "" {
		return "dashboard"
	}
	return slug
}

func slugifyProfileID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = strings.Trim(re.ReplaceAllString(s, "-"), "-")
	if s == "" {
		return "dashboard"
	}
	return s
}

func humanWidgetLabel(t string) string {
	parts := strings.Split(t, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
