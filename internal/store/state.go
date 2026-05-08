package store

import (
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
)

// SystemEvent carries a collector health notification published to SSE subscribers.
type SystemEvent struct {
	CollectorID string
	Status      string // "ok" or "error"
	Message     string // non-empty on error
	At          time.Time
}

// SystemSnapshot is the retained health state for a collector.
type SystemSnapshot struct {
	CollectorID   string    `json:"collector_id"`
	Status        string    `json:"status"`
	Message       string    `json:"message,omitempty"`
	LastEventAt   time.Time `json:"last_event_at"`
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
}

// Update is delivered to SSE subscribers. Exactly one of State and System is non-nil.
type Update struct {
	State  domain.DomainState
	System *SystemEvent
}

// StateStore is a thread-safe in-memory store for the current DomainState per DomainType.
// It fans out every change to all active SSE subscribers.
type StateStore struct {
	mu   sync.RWMutex
	data map[domain.DomainType]domain.DomainState

	system map[string]SystemSnapshot

	subsMu sync.Mutex
	subs   map[int]chan Update
	nextID int
}

func NewStateStore() *StateStore {
	return &StateStore{
		data:   make(map[domain.DomainType]domain.DomainState),
		system: make(map[string]SystemSnapshot),
		subs:   make(map[int]chan Update),
	}
}

// Set stores state and fans out a notification to all subscribers.
func (s *StateStore) Set(state domain.DomainState) {
	s.mu.Lock()
	s.data[state.DomainType()] = state
	s.mu.Unlock()
	s.fan(Update{State: state})
}

// Get returns the current state for a domain, or nil if not yet populated.
func (s *StateStore) Get(dt domain.DomainType) domain.DomainState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[dt]
}

// PublishSystem fans out a collector health event to all SSE subscribers.
func (s *StateStore) PublishSystem(e SystemEvent) {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	s.mu.Lock()
	snap := s.system[e.CollectorID]
	snap.CollectorID = e.CollectorID
	snap.Status = e.Status
	snap.Message = e.Message
	snap.LastEventAt = e.At
	if e.Status == "ok" {
		snap.LastSuccessAt = e.At
	}
	s.system[e.CollectorID] = snap
	s.mu.Unlock()
	s.fan(Update{System: &e})
}

// GetSystem returns the retained health state for collectorID.
func (s *StateStore) GetSystem(collectorID string) (SystemSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.system[collectorID]
	return snap, ok
}

// Subscribe returns a channel that receives state updates and an unsubscribe
// function. The caller must invoke unsubscribe when done to avoid leaking the
// channel.
func (s *StateStore) Subscribe() (<-chan Update, func()) {
	ch := make(chan Update, 32)
	s.subsMu.Lock()
	id := s.nextID
	s.nextID++
	s.subs[id] = ch
	s.subsMu.Unlock()

	return ch, func() {
		s.subsMu.Lock()
		delete(s.subs, id)
		close(ch)
		s.subsMu.Unlock()
	}
}

func (s *StateStore) fan(u Update) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- u:
		default:
			// subscriber too slow; drop this update
		}
	}
}
