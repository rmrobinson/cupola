package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
)

type adminCollectorInfo struct {
	ID            string         `json:"id"`
	Domain        string         `json:"domain"`
	Status        string         `json:"status"`
	Message       string         `json:"message,omitempty"`
	LastUpdatedAt *time.Time     `json:"last_updated_at,omitempty"`
	LastEventAt   *time.Time     `json:"last_event_at,omitempty"`
	LastSuccessAt *time.Time     `json:"last_success_at,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func (h *Handler) getAdminCollectors(w http.ResponseWriter, r *http.Request) {
	collectors := h.registry.Collectors()
	out := make([]adminCollectorInfo, 0, len(collectors))
	for _, c := range collectors {
		info := adminCollectorInfo{
			ID:     c.ID(),
			Domain: string(c.Domain()),
			Status: "unknown",
		}

		if state := h.store.Get(c.Domain()); state != nil {
			updated := state.StateUpdatedAt()
			if !updated.IsZero() {
				info.LastUpdatedAt = &updated
				info.Status = "ok"
			}
		}

		if snap, ok := h.store.GetSystem(c.ID()); ok {
			info.Status = snap.Status
			info.Message = snap.Message
			if !snap.LastEventAt.IsZero() {
				info.LastEventAt = &snap.LastEventAt
			}
			if !snap.LastSuccessAt.IsZero() {
				info.LastSuccessAt = &snap.LastSuccessAt
			}
		}

		if c.Domain() == domain.DomainTransitArrivals {
			info.Metadata = map[string]any{
				"agencies": h.agencies.Stats(),
			}
		}

		out = append(out, info)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *Handler) getAdminPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, h.frontend, "admin.html")
}
