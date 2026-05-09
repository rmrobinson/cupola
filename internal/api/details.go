package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/rmrobinson/cupola/internal/domain"
)

func (h *Handler) getDetail(w http.ResponseWriter, r *http.Request) {
	dt := domain.DomainType(chi.URLParam(r, "domain"))
	id := r.URL.Query().Get("id")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	detail, ok := h.detailFor(dt, id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

func (h *Handler) detailFor(dt domain.DomainType, id string) (domain.Detail, bool) {
	state := h.store.Get(dt)
	if state == nil {
		return domain.Detail{}, false
	}

	switch dt {
	case domain.DomainWeatherAlerts:
		s, ok := state.(domain.WeatherAlerts)
		if !ok {
			return domain.Detail{}, false
		}
		for _, a := range s.Alerts {
			if a.ID != id {
				continue
			}
			return domain.Detail{
				Domain:      string(dt),
				ID:          a.ID,
				Title:       a.Title,
				Severity:    string(a.Severity),
				Description: a.Summary,
				Fields: []domain.DetailField{
					fieldTime("onset", &a.Onset),
					fieldTime("expires", &a.Expires),
				},
				SourceURL: safeSourceURL(a.SourceURL),
			}, true
		}

	case domain.DomainTransitAlerts:
		s, ok := state.(domain.TransitAlerts)
		if !ok {
			return domain.Detail{}, false
		}
		for _, a := range s.Alerts {
			if a.ID != id {
				continue
			}
			fields := []domain.DetailField{
				{Key: "agency_id", Value: a.AgencyID},
				fieldTime("starts_at", a.StartsAt),
				fieldTime("ends_at", a.EndsAt),
			}
			if len(a.AffectedRoutes) > 0 {
				fields = append(fields, domain.DetailField{Key: "affected_routes", Value: strings.Join(a.AffectedRoutes, ", ")})
			}
			return domain.Detail{
				Domain:      string(dt),
				ID:          a.ID,
				Title:       a.Title,
				Subtitle:    a.AgencyID,
				Severity:    string(a.Severity),
				Description: a.Description,
				Fields:      compactFields(fields),
			}, true
		}

	case domain.DomainMunicipalAlerts:
		s, ok := state.(domain.MunicipalAlerts)
		if !ok {
			return domain.Detail{}, false
		}
		for _, a := range s.Alerts {
			if a.ID != id {
				continue
			}
			fields := []domain.DetailField{
				{Key: "source_id", Value: a.SourceID},
				{Key: "alert_type", Value: a.AlertType},
				fieldStringPtr("area", a.Area),
				fieldTime("starts_at", a.StartsAt),
				fieldTime("ends_at", a.EndsAt),
				fieldTime("published_at", &a.PublishedAt),
			}
			sourceURL := ""
			if a.URL != nil {
				sourceURL = safeSourceURL(*a.URL)
			}
			return domain.Detail{
				Domain:      string(dt),
				ID:          a.ID,
				Title:       a.Title,
				Subtitle:    valueOrEmpty(a.Area),
				Severity:    string(a.Severity),
				Description: a.Description,
				Fields:      compactFields(fields),
				SourceURL:   sourceURL,
			}, true
		}

	case domain.DomainFlagStatus:
		s, ok := state.(domain.FlagStatus)
		if !ok || id != "current" {
			return domain.Detail{}, false
		}
		return domain.Detail{
			Domain:      string(dt),
			ID:          id,
			Title:       "flag.status",
			Description: valueOrEmpty(s.Reason),
			Fields: compactFields([]domain.DetailField{
				fieldBool("at_half_mast", s.AtHalfMast),
				fieldTime("since", s.Since),
				fieldTime("until", s.Until),
			}),
			SourceURL: safeSourceURL(s.SourceURL),
		}, true

	case domain.DomainTrafficIncidents:
		s, ok := state.(domain.TrafficIncidents)
		if !ok {
			return domain.Detail{}, false
		}
		for _, inc := range s.Incidents {
			if inc.ID != id {
				continue
			}
			var loc *domain.DetailLocation
			if !inc.ApproximateLocation && (inc.Lat != 0 || inc.Lon != 0) {
				loc = &domain.DetailLocation{Lat: inc.Lat, Lon: inc.Lon}
			}
			locationValue := "specific"
			if inc.ApproximateLocation {
				locationValue = inc.LocationLabel
				if locationValue != "" {
					locationValue += " (approximate; not shown on map)"
				} else {
					locationValue = "approximate; not shown on map"
				}
			}
			return domain.Detail{
				Domain:      string(dt),
				ID:          inc.ID,
				Title:       inc.Type,
				Subtitle:    inc.RoadName,
				Severity:    inc.Severity,
				Description: inc.Description,
				Fields: compactFields([]domain.DetailField{
					{Key: "road_name", Value: inc.RoadName},
					{Key: "type", Value: inc.Type},
					{Key: "location", Value: locationValue},
					fieldTime("starts_at", inc.StartsAt),
					fieldTime("ends_at", inc.EndsAt),
				}),
				SourceURL: safeSourceURL(inc.SourceURL),
				Location:  loc,
			}, true
		}

	case domain.DomainWaterwayConditions:
		s, ok := state.(domain.WaterwayConditions)
		if !ok {
			return domain.Detail{}, false
		}
		for _, g := range s.Gauges {
			if g.ID != id {
				continue
			}
			return domain.Detail{
				Domain:      string(dt),
				ID:          g.ID,
				Title:       g.Name,
				Subtitle:    g.WaterwayName,
				Severity:    g.AdvisoryStatus,
				Description: valueOrEmpty(g.AdvisoryText),
				Fields: compactFields([]domain.DetailField{
					{Key: "waterway_name", Value: g.WaterwayName},
					fieldFloatPtr("level_m", g.LevelM, "m"),
					fieldFloatPtr("flow_cms", g.FlowCMS, "m3_per_s"),
					fieldFloatPtr("temp_c", g.TempC, "celsius"),
					{Key: "advisory_status", Value: g.AdvisoryStatus},
					fieldTime("updated_at", &g.UpdatedAt),
				}),
				SourceURL: safeSourceURL(g.SourceURL),
				Location:  &domain.DetailLocation{Lat: g.Lat, Lon: g.Lon},
			}, true
		}
	}

	return domain.Detail{}, false
}

func fieldStringPtr(key string, value *string) domain.DetailField {
	if value == nil {
		return domain.DetailField{}
	}
	return domain.DetailField{Key: key, Value: *value}
}

func fieldTime(key string, value *time.Time) domain.DetailField {
	if value == nil || value.IsZero() {
		return domain.DetailField{}
	}
	return domain.DetailField{Key: key, Value: value.Format(time.RFC3339)}
}

func fieldBool(key string, value bool) domain.DetailField {
	return domain.DetailField{Key: key, Value: value}
}

func fieldFloatPtr(key string, value *float64, unit string) domain.DetailField {
	if value == nil {
		return domain.DetailField{}
	}
	return domain.DetailField{Key: key, Value: *value, Unit: unit}
}

func compactFields(fields []domain.DetailField) []domain.DetailField {
	out := make([]domain.DetailField, 0, len(fields))
	for _, f := range fields {
		if f.Key != "" && f.Value != nil && f.Value != "" {
			out = append(out, f)
		}
	}
	return out
}

func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func safeSourceURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return raw
}
