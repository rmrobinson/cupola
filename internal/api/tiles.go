package api

import "net/http"

func (h *Handler) getTile(w http.ResponseWriter, r *http.Request) {
	if h.tileHandler == nil {
		http.NotFound(w, r)
		return
	}
	h.tileHandler.ServeHTTP(w, r)
}
