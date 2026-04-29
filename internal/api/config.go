package api

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		CountryCode string  `json:"country_code,omitempty"`
	}{Lat: h.homeLat, Lon: h.homeLon, CountryCode: h.countryCode})
}
