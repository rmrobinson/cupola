package store

import (
	"encoding/json"
	"sync"

	"github.com/rmrobinson/cupola/internal/domain"
)

// subKey returns a canonical string for a (domain, params) pair.
// encoding/json marshals map keys in sorted order (Go 1.12+), so this is
// deterministic regardless of param insertion order.
func subKey(dt domain.DomainType, params map[string]any) string {
	if len(params) == 0 {
		return string(dt)
	}
	b, err := json.Marshal(params)
	if err != nil {
		return string(dt)
	}
	return string(dt) + ":" + string(b)
}

type subscription struct {
	widgetID  string
	sessionID string
	domain    domain.DomainType
	params    map[string]any
	key       string
}

// SubscriptionManager tracks widget subscriptions by session and by domain+params.
// It is the authoritative source for which (domain, params) combinations have
// active consumers — parameterised collectors query ActiveParams to decide what
// data to fetch.
type SubscriptionManager struct {
	mu        sync.Mutex
	byWidget  map[string]subscription // widgetID → subscription
	bySession map[string][]string     // sessionID → []widgetID
	refCount  map[string]int          // subKey → active subscription count
}

func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		byWidget:  make(map[string]subscription),
		bySession: make(map[string][]string),
		refCount:  make(map[string]int),
	}
}

// Register adds or replaces a widget subscription.
func (m *SubscriptionManager) Register(sessionID, widgetID string, dt domain.DomainType, params map[string]any) {
	key := subKey(dt, params)
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.byWidget[widgetID]; ok {
		m.decRef(existing.key)
		m.removeFromSession(existing.sessionID, widgetID)
	}

	m.byWidget[widgetID] = subscription{
		widgetID:  widgetID,
		sessionID: sessionID,
		domain:    dt,
		params:    params,
		key:       key,
	}
	m.bySession[sessionID] = append(m.bySession[sessionID], widgetID)
	m.refCount[key]++
}

// Deregister removes a single widget subscription (widget removed from canvas).
func (m *SubscriptionManager) Deregister(widgetID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deregisterLocked(widgetID)
}

// CloseSession removes all subscriptions belonging to sessionID and returns
// the number of subscriptions dropped. Called when an SSE connection closes.
func (m *SubscriptionManager) CloseSession(sessionID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	widgets := m.bySession[sessionID]
	for _, wid := range widgets {
		if sub, ok := m.byWidget[wid]; ok {
			m.decRef(sub.key)
			delete(m.byWidget, wid)
		}
	}
	delete(m.bySession, sessionID)
	return len(widgets)
}

// ActiveCount returns the current ref count for a (domain, params) pair.
func (m *SubscriptionManager) ActiveCount(dt domain.DomainType, params map[string]any) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refCount[subKey(dt, params)]
}

// ActiveParams returns every distinct params map that has at least one active
// subscriber for the given domain. Parameterised collectors call this to decide
// what combinations to fetch.
func (m *SubscriptionManager) ActiveParams(dt domain.DomainType) []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]struct{})
	var out []map[string]any
	for _, sub := range m.byWidget {
		if sub.domain != dt {
			continue
		}
		if _, dup := seen[sub.key]; dup {
			continue
		}
		seen[sub.key] = struct{}{}
		out = append(out, sub.params)
	}
	return out
}

func (m *SubscriptionManager) deregisterLocked(widgetID string) {
	sub, ok := m.byWidget[widgetID]
	if !ok {
		return
	}
	m.decRef(sub.key)
	m.removeFromSession(sub.sessionID, widgetID)
	delete(m.byWidget, widgetID)
}

func (m *SubscriptionManager) decRef(key string) {
	m.refCount[key]--
	if m.refCount[key] <= 0 {
		delete(m.refCount, key)
	}
}

func (m *SubscriptionManager) removeFromSession(sessionID, widgetID string) {
	s := m.bySession[sessionID]
	for i, wid := range s {
		if wid == widgetID {
			m.bySession[sessionID] = append(s[:i], s[i+1:]...)
			break
		}
	}
	if len(m.bySession[sessionID]) == 0 {
		delete(m.bySession, sessionID)
	}
}
