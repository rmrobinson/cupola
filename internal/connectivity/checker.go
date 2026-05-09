package connectivity

import (
	"context"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/store"
)

const (
	DefaultCheckURL = "http://connectivitycheck.gstatic.com/generate_204"
	collectorID     = "connectivity"
	defaultInterval = 30 * time.Second
)

// Checker periodically probes a URL to determine whether the public internet
// is reachable. It publishes SSE system events on transitions and exposes
// IsUp() for collectors to gate their fetch loops.
type Checker struct {
	checkURL   string
	interval   time.Duration
	stateStore *store.StateStore
	client     *http.Client

	mu         sync.RWMutex
	up         bool
	forcedDown bool
}

func New(checkURL string, interval time.Duration, stateStore *store.StateStore) *Checker {
	if checkURL == "" {
		checkURL = DefaultCheckURL
	}
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Checker{
		checkURL:   checkURL,
		interval:   interval,
		stateStore: stateStore,
		client:     &http.Client{Timeout: 10 * time.Second},
		up:         true, // optimistic: assume up until first probe
	}
}

// Start launches the background probe loop. It returns immediately; the loop
// runs until ctx is cancelled. An initial "ok" event is published synchronously
// so the connectivity entry is always visible in the admin from the moment the
// checker starts — before the first probe completes.
func (c *Checker) Start(ctx context.Context) {
	c.stateStore.PublishSystem(store.SystemEvent{
		CollectorID: collectorID, Status: "ok",
	})
	go c.run(ctx)
}

// IsUp reports whether internet is reachable, considering both the last probe
// result and any active force-down override.
func (c *Checker) IsUp() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.forcedDown && c.up
}

// IsForced reports whether a manual force-down override is currently active.
func (c *Checker) IsForced() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.forcedDown
}

// SetForceDown enables or clears a manual override that forces the checker to
// report internet as down, regardless of actual probe results. Publishes an SSE
// system event on change so collectors and the frontend react immediately.
func (c *Checker) SetForceDown(forced bool) {
	c.mu.Lock()
	prev := c.forcedDown
	c.forcedDown = forced
	realUp := c.up
	c.mu.Unlock()

	if forced == prev {
		return
	}
	if forced {
		log.Printf("[connectivity] force-down enabled (test mode)")
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: collectorID, Status: "caution", Message: "Forced offline (test mode)",
		})
	} else {
		log.Printf("[connectivity] force-down cleared")
		if realUp {
			c.stateStore.PublishSystem(store.SystemEvent{CollectorID: collectorID, Status: "ok"})
		} else {
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: collectorID, Status: "caution", Message: "No internet connectivity",
			})
		}
	}
}

func (c *Checker) run(ctx context.Context) {
	c.probe(ctx)
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.probe(ctx)
		}
	}
}

func (c *Checker) probe(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.checkURL, nil)
	if err != nil {
		return
	}
	resp, err := c.client.Do(req)
	if err == nil {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 256)) //nolint:errcheck
		resp.Body.Close()
	}

	up := err == nil

	c.mu.Lock()
	prev := c.up
	c.up = up
	forced := c.forcedDown
	c.mu.Unlock()

	if up == prev || forced {
		return // no transition, or forced state overrides probe events
	}
	if up {
		log.Printf("[connectivity] internet restored")
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: collectorID, Status: "ok",
		})
	} else {
		log.Printf("[connectivity] internet down: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: collectorID, Status: "caution", Message: "No internet connectivity",
		})
	}
}
