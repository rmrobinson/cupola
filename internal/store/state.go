package store

import (
	"sync"

	"github.com/rmrobinson/cupola/internal/domain"
)

// SystemEvent carries a collector health notification published to SSE subscribers.
type SystemEvent struct {
	CollectorID string
	Status      string // "ok" or "error"
	Message     string // non-empty on error
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

	subsMu sync.Mutex
	subs   map[int]chan Update
	nextID int
}

func NewStateStore() *StateStore {
	return &StateStore{
		data: make(map[domain.DomainType]domain.DomainState),
		subs: make(map[int]chan Update),
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
	s.fan(Update{System: &e})
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
