package notes

import (
	"context"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// Collector loads notes from SQLite and keeps the state store current.
// The API write handlers call Refresh after every mutation so that the SSE
// stream reflects the change immediately.
type Collector struct {
	db         *store.SQLiteStore
	stateStore *store.StateStore
	mu         sync.RWMutex
	state      domain.Notes
}

func New(db *store.SQLiteStore, stateStore *store.StateStore) *Collector {
	return &Collector{db: db, stateStore: stateStore}
}

func (c *Collector) ID() string                { return "notes" }
func (c *Collector) Domain() domain.DomainType { return domain.DomainNotes }

// Start loads the initial state from SQLite and publishes it.
func (c *Collector) Start(_ context.Context) error {
	return c.refresh()
}

func (c *Collector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Refresh reloads all notes from SQLite and pushes an SSE update.
func (c *Collector) Refresh() error {
	return c.refresh()
}

func (c *Collector) refresh() error {
	notes, err := c.db.ListNotes()
	if err != nil {
		return err
	}

	s := domain.Notes{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Notes:     notes,
	}

	c.mu.Lock()
	c.state = s
	c.mu.Unlock()

	c.stateStore.Set(s)
	return nil
}
