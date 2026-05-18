package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rmrobinson/cupola/internal/domain"
)

type subscriptionRequest struct {
	WidgetID  string         `json:"widget_id"`
	SessionID string         `json:"session_id"`
	Domain    string         `json:"domain"`
	Params    map[string]any `json:"params"`
}

func (h *Handler) createSubscription(w http.ResponseWriter, r *http.Request) {
	var req subscriptionRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.WidgetID == "" || req.SessionID == "" || req.Domain == "" {
		http.Error(w, "widget_id, session_id, and domain are required", http.StatusBadRequest)
		return
	}
	h.subs.Register(req.SessionID, req.WidgetID, domain.DomainType(req.Domain), req.Params)
	h.registry.NotifySubscription(domain.DomainType(req.Domain))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	h.subs.Deregister(chi.URLParam(r, "widgetID"))
	w.WriteHeader(http.StatusNoContent)
}
