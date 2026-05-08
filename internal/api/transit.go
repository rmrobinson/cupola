package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rmrobinson/cupola/internal/collector/gtfsrt"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

var transitAgencyIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type agencyInfo struct {
	ID string `json:"id"`
}

type routeInfo struct {
	ID        string `json:"id"`
	ShortName string `json:"short_name"`
	LongName  string `json:"long_name"`
}

type stopInfo struct {
	ID         string  `json:"id"`
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	DistanceKm float64 `json:"distance_km"`
}

func (h *Handler) getTransitAgencies(w http.ResponseWriter, r *http.Request) {
	agencies := h.agencies.List()
	out := make([]agencyInfo, 0, len(agencies))
	for _, ag := range agencies {
		out = append(out, agencyInfo{ID: ag.ID})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *Handler) getTransitRoutes(w http.ResponseWriter, r *http.Request) {
	ag := h.findAgency(chi.URLParam(r, "agencyID"))
	if ag == nil {
		http.NotFound(w, r)
		return
	}
	routes := ag.Schedule.AllRoutes()
	out := make([]routeInfo, 0, len(routes))
	for _, rt := range routes {
		out = append(out, routeInfo{ID: rt.ID, ShortName: rt.ShortName, LongName: rt.LongName})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *Handler) getTransitStops(w http.ResponseWriter, r *http.Request) {
	ag := h.findAgency(chi.URLParam(r, "agencyID"))
	if ag == nil {
		http.NotFound(w, r)
		return
	}
	stops := ag.Schedule.StopsForRoute(chi.URLParam(r, "routeID"))
	out := make([]stopInfo, 0, len(stops))
	for _, st := range stops {
		out = append(out, stopInfo{
			ID:         st.ID,
			Code:       st.Code,
			Name:       st.Name,
			Lat:        st.Lat,
			Lon:        st.Lon,
			DistanceKm: haversineKm(h.homeLat, h.homeLon, st.Lat, st.Lon),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DistanceKm < out[j].DistanceKm })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

type geoJSONMultiLineString struct {
	Type        string         `json:"type"`
	Coordinates [][][2]float64 `json:"coordinates"`
}

type routeShapeResponse struct {
	RouteID  string                 `json:"route_id"`
	Color    string                 `json:"color"`
	Geometry geoJSONMultiLineString `json:"geometry"`
}

func (h *Handler) getTransitRouteShape(w http.ResponseWriter, r *http.Request) {
	ag := h.findAgency(chi.URLParam(r, "agencyID"))
	if ag == nil {
		http.NotFound(w, r)
		return
	}
	if !ag.Schedule.HasShapes() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no shape data available"})
		return
	}
	routeID := chi.URLParam(r, "routeID")
	lines, color := ag.Schedule.ShapesForRoute(routeID)
	if len(lines) == 0 {
		http.NotFound(w, r)
		return
	}

	// Convert [lat,lon] pairs to GeoJSON [lon,lat] order.
	coords := make([][][2]float64, len(lines))
	for i, line := range lines {
		coords[i] = make([][2]float64, len(line))
		for j, pt := range line {
			coords[i][j] = [2]float64{pt[1], pt[0]} // lon, lat
		}
	}

	cssColor := ""
	if color != "" {
		cssColor = "#" + strings.ToUpper(color)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routeShapeResponse{
		RouteID: routeID,
		Color:   cssColor,
		Geometry: geoJSONMultiLineString{
			Type:        "MultiLineString",
			Coordinates: coords,
		},
	})
}

func (h *Handler) findAgency(id string) *gtfsrt.Agency {
	return h.agencies.Get(id)
}

func (h *Handler) listTransitAgencyConfigs(w http.ResponseWriter, r *http.Request) {
	cfgs, err := h.db.ListTransitAgencies()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfgs)
}

func (h *Handler) getTransitAgencyConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.db.GetTransitAgency(chi.URLParam(r, "agencyID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func (h *Handler) createTransitAgencyConfig(w http.ResponseWriter, r *http.Request) {
	var cfg store.TransitAgencyConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	cfg = normalizeTransitAgencyConfig(cfg)
	if err := validateTransitAgencyConfig(cfg, true); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.db.CreateTransitAgency(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := h.agencies.Reload(cfg.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.agencies.RefreshStaticAsync(cfg.ID)
	h.notifyTransitCollectors()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	created, _ := h.db.GetTransitAgency(cfg.ID)
	json.NewEncoder(w).Encode(created)
}

func (h *Handler) updateTransitAgencyConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "agencyID")
	existing, err := h.db.GetTransitAgency(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.NotFound(w, r)
		return
	}

	var patch transitAgencyConfigPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if patch.ID != nil && *patch.ID != id {
		http.Error(w, "agency id is immutable", http.StatusBadRequest)
		return
	}

	cfg := *existing
	if patch.Enabled != nil {
		cfg.Enabled = *patch.Enabled
	}
	if patch.GTFSStaticURLs != nil {
		cfg.GTFSStaticURLs = *patch.GTFSStaticURLs
	}
	if patch.GTFSRTTripUpdatesURLs != nil {
		cfg.GTFSRTTripUpdatesURLs = *patch.GTFSRTTripUpdatesURLs
	}
	if patch.GTFSRTVehiclePositionsURLs != nil {
		cfg.GTFSRTVehiclePositionsURLs = *patch.GTFSRTVehiclePositionsURLs
	}
	if patch.GTFSRTAlertsURL != nil {
		cfg.GTFSRTAlertsURL = *patch.GTFSRTAlertsURL
	}
	cfg = normalizeTransitAgencyConfig(cfg)
	if err := validateTransitAgencyConfig(cfg, false); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.db.UpdateTransitAgency(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.agencies.Reload(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.agencies.RefreshStaticAsync(id)
	h.notifyTransitCollectors()
	w.Header().Set("Content-Type", "application/json")
	updated, _ := h.db.GetTransitAgency(id)
	json.NewEncoder(w).Encode(updated)
}

func (h *Handler) deleteTransitAgencyConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "agencyID")
	if existing, err := h.db.GetTransitAgency(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if existing == nil {
		http.NotFound(w, r)
		return
	}
	if err := h.agencies.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.notifyTransitCollectors()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) notifyTransitCollectors() {
	h.registry.NotifySubscription(domain.DomainTransitArrivals)
	h.registry.NotifySubscription(domain.DomainTransitVehicles)
	h.registry.NotifySubscription(domain.DomainTransitAlerts)
}

func validateTransitAgencyConfig(cfg store.TransitAgencyConfig, requireID bool) error {
	if requireID && !transitAgencyIDRe.MatchString(cfg.ID) {
		return errString("id must be 1-64 characters and contain only letters, numbers, underscore, or hyphen")
	}
	if !requireID && cfg.ID != "" && !transitAgencyIDRe.MatchString(cfg.ID) {
		return errString("invalid agency id")
	}
	if cfg.Enabled {
		if len(cfg.GTFSStaticURLs) == 0 {
			return errString("enabled agencies require at least one gtfs_static_urls entry")
		}
		if len(cfg.GTFSRTTripUpdatesURLs) == 0 {
			return errString("enabled agencies require at least one gtfs_rt_trip_updates_urls entry")
		}
	}
	urls := make([]string, 0, len(cfg.GTFSStaticURLs)+len(cfg.GTFSRTTripUpdatesURLs)+len(cfg.GTFSRTVehiclePositionsURLs))
	urls = append(urls, cfg.GTFSStaticURLs...)
	urls = append(urls, cfg.GTFSRTTripUpdatesURLs...)
	urls = append(urls, cfg.GTFSRTVehiclePositionsURLs...)
	for _, raw := range urls {
		if err := validateHTTPURL(raw); err != nil {
			return err
		}
	}
	if cfg.GTFSRTAlertsURL != "" {
		if err := validateHTTPURL(cfg.GTFSRTAlertsURL); err != nil {
			return err
		}
	}
	return nil
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errString("feed URLs must be absolute http or https URLs")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

type transitAgencyConfigPatch struct {
	ID                         *string   `json:"id"`
	Enabled                    *bool     `json:"enabled"`
	GTFSStaticURLs             *[]string `json:"gtfs_static_urls"`
	GTFSRTTripUpdatesURLs      *[]string `json:"gtfs_rt_trip_updates_urls"`
	GTFSRTVehiclePositionsURLs *[]string `json:"gtfs_rt_vehicle_positions_urls"`
	GTFSRTAlertsURL            *string   `json:"gtfs_rt_alerts_url"`
}

func normalizeTransitAgencyConfig(cfg store.TransitAgencyConfig) store.TransitAgencyConfig {
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.GTFSStaticURLs = normalizeURLList(cfg.GTFSStaticURLs)
	cfg.GTFSRTTripUpdatesURLs = normalizeURLList(cfg.GTFSRTTripUpdatesURLs)
	cfg.GTFSRTVehiclePositionsURLs = normalizeURLList(cfg.GTFSRTVehiclePositionsURLs)
	cfg.GTFSRTAlertsURL = strings.TrimSpace(cfg.GTFSRTAlertsURL)
	return cfg
}

func normalizeURLList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		out = append(out, strings.TrimSpace(raw))
	}
	return out
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	lat1r := lat1 * math.Pi / 180
	lat2r := lat2 * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1r)*math.Cos(lat2r)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
