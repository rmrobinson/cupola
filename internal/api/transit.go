package api

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rmrobinson/cupola/internal/collector/gtfsrt"
)

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
	out := make([]agencyInfo, 0, len(h.agencies))
	for _, ag := range h.agencies {
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
	Type        string        `json:"type"`
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
	for _, ag := range h.agencies {
		if ag.ID == id {
			return ag
		}
	}
	return nil
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
