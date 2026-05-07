package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// domainEvent is the JSON shape for domain state updates on the SSE stream.
type domainEvent struct {
	Domain string             `json:"domain"`
	Ts     int64              `json:"ts"`
	Data   domain.DomainState `json:"data"`
}

// systemEvent is the JSON shape for collector health events on the SSE stream.
type systemEvent struct {
	Domain      string `json:"domain"` // always "system"
	Ts          int64  `json:"ts"`
	CollectorID string `json:"collector_id"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
}

func (h *Handler) getStream(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering

	updates, unsub := h.store.Subscribe()
	defer func() {
		unsub()
		if sessionID != "" {
			if dropped := h.subs.CloseSession(sessionID); dropped > 0 {
				log.Printf("session %s disconnected: dropped %d subscriptions", sessionLogPrefix(sessionID), dropped)
			}
		}
	}()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case u, ok := <-updates:
			if !ok {
				return
			}
			if err := h.writeSSEUpdate(w, u); err != nil {
				return
			}
			flusher.Flush()

		case <-heartbeat.C:
			// SSE comment keeps the connection alive through proxies
			fmt.Fprintf(w, ":\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

func (h *Handler) writeSSEUpdate(w http.ResponseWriter, u store.Update) error {
	var payload []byte
	var err error

	switch {
	case u.State != nil:
		payload, err = json.Marshal(domainEvent{
			Domain: string(u.State.DomainType()),
			Ts:     u.State.StateUpdatedAt().Unix(),
			Data:   u.State,
		})
	case u.System != nil:
		payload, err = json.Marshal(systemEvent{
			Domain:      "system",
			Ts:          time.Now().Unix(),
			CollectorID: u.System.CollectorID,
			Status:      u.System.Status,
			Message:     u.System.Message,
		})
	default:
		return nil
	}

	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	return err
}

func sessionLogPrefix(sessionID string) string {
	if len(sessionID) <= 8 {
		return sessionID
	}
	return sessionID[:8]
}
