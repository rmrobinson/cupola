package api

import (
	"encoding/json"
	"net/http"
	"sort"
)

func (h *Handler) getDomains(w http.ResponseWriter, r *http.Request) {
	domains := h.registry.Domains()
	sort.Slice(domains, func(i, j int) bool {
		return string(domains[i]) < string(domains[j])
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"domains": domains})
}
