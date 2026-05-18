package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
)

// connectivityChecker is the subset of connectivity.Checker the admin API needs.
type connectivityChecker interface {
	SetForceDown(bool)
	IsForced() bool
}

type adminCollectorInfo struct {
	ID            string         `json:"id"`
	Domain        string         `json:"domain"`
	Status        string         `json:"status"`
	Message       string         `json:"message,omitempty"`
	LastUpdatedAt *time.Time     `json:"last_updated_at,omitempty"`
	LastEventAt   *time.Time     `json:"last_event_at,omitempty"`
	LastSuccessAt *time.Time     `json:"last_success_at,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Forced        *bool          `json:"forced,omitempty"` // non-nil only for the connectivity entry
}

func (h *Handler) getAdminCollectors(w http.ResponseWriter, r *http.Request) {
	collectors := h.registry.Collectors()
	registryIDs := make(map[string]bool, len(collectors))
	out := make([]adminCollectorInfo, 0, len(collectors))

	for _, c := range collectors {
		registryIDs[c.ID()] = true
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

	// Include system-event entries not tied to a registered collector (sub-sources,
	// connectivity checker, etc.) so the admin page shows their health too.
	for _, snap := range h.store.ListSystem() {
		if registryIDs[snap.CollectorID] {
			continue
		}
		info := adminCollectorInfo{
			ID:     snap.CollectorID,
			Status: snap.Status,
		}
		if info.Status == "" {
			info.Status = "unknown"
		}
		info.Message = snap.Message
		if !snap.LastEventAt.IsZero() {
			info.LastEventAt = &snap.LastEventAt
		}
		if !snap.LastSuccessAt.IsZero() {
			info.LastSuccessAt = &snap.LastSuccessAt
		}
		if snap.CollectorID == "connectivity" && h.connectivity != nil {
			forced := h.connectivity.IsForced()
			info.Forced = &forced
		}
		out = append(out, info)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (h *Handler) patchConnectivity(w http.ResponseWriter, r *http.Request) {
	if h.connectivity == nil {
		http.Error(w, "connectivity checker not available", http.StatusNotFound)
		return
	}
	var body struct {
		ForcedDown bool `json:"forced_down"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	h.connectivity.SetForceDown(body.ForcedDown)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getAdminPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, h.frontend, "admin.html")
}
