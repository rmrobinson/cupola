package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

func TestGetDetailTrafficIncident(t *testing.T) {
	st := store.NewStateStore()
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	st.Set(domain.TrafficIncidents{
		StateBase: domain.StateBase{UpdatedAt: now},
		Incidents: []domain.TrafficIncident{{
			ID:          "abc",
			Type:        "collision",
			Severity:    "moderate",
			Lat:         43.45,
			Lon:         -80.49,
			Description: "Crash on the shoulder",
			RoadName:    "King St",
			SourceURL:   "https://511on.ca",
		}},
	})

	h := &Handler{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/details/traffic.incidents?id=abc", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"title":"collision"`) ||
		!strings.Contains(body, `"subtitle":"King St"`) ||
		!strings.Contains(body, `"key":"road_name"`) ||
		!strings.Contains(body, `"source_url":"https://511on.ca"`) {
		t.Fatalf("response missing expected detail fields: %s", body)
	}
}

func TestGetDetailFlagCurrent(t *testing.T) {
	st := store.NewStateStore()
	reason := "Official mourning"
	st.Set(domain.FlagStatus{
		StateBase:  domain.StateBase{UpdatedAt: time.Now().UTC()},
		AtHalfMast: true,
		Reason:     &reason,
		SourceURL:  "https://example.com/flag",
	})

	h := &Handler{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/details/flag.status?id=current", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"title":"flag.status"`) ||
		!strings.Contains(body, `"key":"at_half_mast","value":true`) ||
		!strings.Contains(body, `"description":"Official mourning"`) {
		t.Fatalf("response missing expected flag detail fields: %s", body)
	}
}

func TestGetDetailWaterwayGauge(t *testing.T) {
	st := store.NewStateStore()
	level := 1.234
	st.Set(domain.WaterwayConditions{
		StateBase: domain.StateBase{UpdatedAt: time.Now().UTC()},
		Gauges: []domain.WaterwayGauge{{
			ID:             "grca_bridgeport",
			Name:           "Bridgeport",
			WaterwayName:   "Grand River",
			Lat:            43.469,
			Lon:            -80.536,
			LevelM:         &level,
			AdvisoryStatus: "none",
			UpdatedAt:      time.Now().UTC(),
			SourceURL:      "https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/central-grand-flows/",
		}},
	})

	h := &Handler{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/details/waterway.conditions?id=grca_bridgeport", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"title":"Bridgeport"`) ||
		!strings.Contains(body, `"subtitle":"Grand River"`) ||
		!strings.Contains(body, `"key":"level_m","value":1.234,"unit":"m"`) ||
		!strings.Contains(body, `"source_url":"https://www.grandriver.ca/our-watershed/river-data/river-and-stream-flows/central-grand-flows/"`) {
		t.Fatalf("response missing expected waterway detail fields: %s", body)
	}
}

func TestGetDetailSupportsSlashContainingID(t *testing.T) {
	st := store.NewStateStore()
	alertID := "grca.flood:/news/posts/flood-message"
	st.Set(domain.MunicipalAlerts{
		StateBase: domain.StateBase{UpdatedAt: time.Now().UTC()},
		Alerts: []domain.MunicipalAlert{{
			ID:          alertID,
			SourceID:    "grca.flood",
			Title:       "Flood message",
			Description: "River conditions update",
			Severity:    domain.SeverityWarning,
		}},
	})

	h := &Handler{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/details/municipal.alerts?id="+url.QueryEscape(alertID), nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"id":"`+alertID+`"`) {
		t.Fatalf("response missing slash-containing id: %s", rr.Body.String())
	}
}

func TestGetDetailDropsUnsafeSourceURL(t *testing.T) {
	st := store.NewStateStore()
	st.Set(domain.TrafficIncidents{
		Incidents: []domain.TrafficIncident{{
			ID:        "abc",
			Type:      "hazard",
			SourceURL: "javascript:alert(1)",
		}},
	})

	h := &Handler{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/details/traffic.incidents?id=abc", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "source_url") {
		t.Fatalf("unsafe source_url should be omitted: %s", rr.Body.String())
	}
}

func TestGetDetailUnknownIDReturnsNotFound(t *testing.T) {
	st := store.NewStateStore()
	st.Set(domain.TrafficIncidents{Incidents: []domain.TrafficIncident{{ID: "abc"}}})

	h := &Handler{store: st}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/details/traffic.incidents?id=missing", nil)
	rr := httptest.NewRecorder()
	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}
