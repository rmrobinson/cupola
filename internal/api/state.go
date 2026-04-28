package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rmrobinson/cupola/internal/domain"
)

func (h *Handler) getState(w http.ResponseWriter, r *http.Request) {
	dt := domain.DomainType(chi.URLParam(r, "domain"))
	state := h.store.Get(dt)
	if state == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}
