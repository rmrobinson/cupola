package api

import "net/http"

func (h *Handler) getTile(w http.ResponseWriter, r *http.Request) {
	if h.tileHandler == nil {
		http.NotFound(w, r)
		return
	}
	h.tileHandler.ServeHTTP(w, r)
}

// getTileFile serves the raw .pmtiles file for direct browser access via HTTP range requests.
// The protomaps-leaflet client uses this to fetch only the tile data it needs.
func (h *Handler) getTileFile(w http.ResponseWriter, r *http.Request) {
	if h.tileHandler == nil {
		http.NotFound(w, r)
		return
	}
	h.tileHandler.ServeFile(w, r)
}
