package municipal

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/config"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// EventsParser parses an HTTP response body into municipal events.
type EventsParser interface {
	Parse(r io.Reader) ([]domain.MunicipalEvent, error)
}

// AlertsParser parses an HTTP response body into municipal alerts.
type AlertsParser interface {
	Parse(r io.Reader) ([]domain.MunicipalAlert, error)
}

var (
	parserMu      sync.RWMutex
	eventsParsers = map[string]func() EventsParser{}
	alertsParsers = map[string]func() AlertsParser{}
)

// RegisterEventsParser registers a named events-parser factory.
// Intended to be called from parser sub-packages in init().
func RegisterEventsParser(name string, factory func() EventsParser) {
	parserMu.Lock()
	eventsParsers[name] = factory
	parserMu.Unlock()
}

// RegisterAlertsParser registers a named alerts-parser factory.
// Intended to be called from parser sub-packages in init().
func RegisterAlertsParser(name string, factory func() AlertsParser) {
	parserMu.Lock()
	alertsParsers[name] = factory
	parserMu.Unlock()
}

// ── Events collector ───────────────────────────────────────────────────────────

type eventsSource struct {
	cfg    config.MunicipalConfig
	parser EventsParser
}

// EventsCollector polls one or more HTTP sources and aggregates their output
// into the domain.MunicipalEvents state.
type EventsCollector struct {
	sources    []eventsSource
	stateStore *store.StateStore
	mu         sync.RWMutex
	items      map[string][]domain.MunicipalEvent // sourceID → events
}

func NewEventsCollector(cfgs []config.MunicipalConfig, stateStore *store.StateStore) *EventsCollector {
	c := &EventsCollector{
		stateStore: stateStore,
		items:      make(map[string][]domain.MunicipalEvent),
	}
	parserMu.RLock()
	defer parserMu.RUnlock()
	for _, cfg := range cfgs {
		factory, ok := eventsParsers[cfg.Parser]
		if !ok {
			log.Printf("[municipal.events] unknown parser %q for source %s — skipping", cfg.Parser, cfg.ID)
			continue
		}
		c.sources = append(c.sources, eventsSource{cfg: cfg, parser: factory()})
	}
	return c
}

func (c *EventsCollector) ID() string                { return "municipal.events" }
func (c *EventsCollector) Domain() domain.DomainType { return domain.DomainMunicipalEvents }

func (c *EventsCollector) Start(ctx context.Context) error {
	for _, s := range c.sources {
		go c.runSource(ctx, s)
	}
	return nil
}

func (c *EventsCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buildState()
}

func (c *EventsCollector) runSource(ctx context.Context, s eventsSource) {
	if err := c.fetchSource(s); err != nil {
		log.Printf("[municipal.events] %s initial fetch: %v", s.cfg.ID, err)
	}
	interval := s.cfg.PollInterval.Duration
	if interval == 0 {
		interval = 30 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.fetchSource(s); err != nil {
				log.Printf("[municipal.events] %s fetch: %v", s.cfg.ID, err)
			}
		}
	}
}

func (c *EventsCollector) fetchSource(s eventsSource) error {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(s.cfg.URL)
	if err != nil {
		return fmt.Errorf("get %s: %w", s.cfg.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %d", s.cfg.URL, resp.StatusCode)
	}

	events, err := s.parser.Parse(resp.Body)
	if err != nil {
		return fmt.Errorf("parse %s: %w", s.cfg.URL, err)
	}
	for i := range events {
		events[i].SourceID = s.cfg.ID
	}

	c.mu.Lock()
	c.items[s.cfg.ID] = events
	state := c.buildState()
	c.mu.Unlock()

	c.stateStore.Set(state)
	log.Printf("[municipal.events] %s updated: %d events", s.cfg.ID, len(events))
	return nil
}

func (c *EventsCollector) buildState() domain.MunicipalEvents {
	var all []domain.MunicipalEvent
	for _, events := range c.items {
		all = append(all, events...)
	}
	if all == nil {
		all = []domain.MunicipalEvent{}
	}
	return domain.MunicipalEvents{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Events:    all,
	}
}

// ── Alerts collector ───────────────────────────────────────────────────────────

type alertsSource struct {
	cfg    config.MunicipalConfig
	parser AlertsParser
}

// AlertsCollector polls one or more HTTP sources and aggregates their output
// into the domain.MunicipalAlerts state.
type AlertsCollector struct {
	sources    []alertsSource
	stateStore *store.StateStore
	mu         sync.RWMutex
	items      map[string][]domain.MunicipalAlert // sourceID → alerts
}

func NewAlertsCollector(cfgs []config.MunicipalConfig, stateStore *store.StateStore) *AlertsCollector {
	c := &AlertsCollector{
		stateStore: stateStore,
		items:      make(map[string][]domain.MunicipalAlert),
	}
	parserMu.RLock()
	defer parserMu.RUnlock()
	for _, cfg := range cfgs {
		factory, ok := alertsParsers[cfg.Parser]
		if !ok {
			log.Printf("[municipal.alerts] unknown parser %q for source %s — skipping", cfg.Parser, cfg.ID)
			continue
		}
		c.sources = append(c.sources, alertsSource{cfg: cfg, parser: factory()})
	}
	return c
}

func (c *AlertsCollector) ID() string                { return "municipal.alerts" }
func (c *AlertsCollector) Domain() domain.DomainType { return domain.DomainMunicipalAlerts }

func (c *AlertsCollector) Start(ctx context.Context) error {
	for _, s := range c.sources {
		go c.runSource(ctx, s)
	}
	return nil
}

func (c *AlertsCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buildState()
}

func (c *AlertsCollector) runSource(ctx context.Context, s alertsSource) {
	if err := c.fetchSource(s); err != nil {
		log.Printf("[municipal.alerts] %s initial fetch: %v", s.cfg.ID, err)
	}
	interval := s.cfg.PollInterval.Duration
	if interval == 0 {
		interval = 15 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.fetchSource(s); err != nil {
				log.Printf("[municipal.alerts] %s fetch: %v", s.cfg.ID, err)
			}
		}
	}
}

func (c *AlertsCollector) fetchSource(s alertsSource) error {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(s.cfg.URL)
	if err != nil {
		return fmt.Errorf("get %s: %w", s.cfg.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %d", s.cfg.URL, resp.StatusCode)
	}

	alerts, err := s.parser.Parse(resp.Body)
	if err != nil {
		return fmt.Errorf("parse %s: %w", s.cfg.URL, err)
	}
	for i := range alerts {
		alerts[i].SourceID = s.cfg.ID
	}

	c.mu.Lock()
	c.items[s.cfg.ID] = alerts
	state := c.buildState()
	c.mu.Unlock()

	c.stateStore.Set(state)
	log.Printf("[municipal.alerts] %s updated: %d alerts", s.cfg.ID, len(alerts))
	return nil
}

func (c *AlertsCollector) buildState() domain.MunicipalAlerts {
	var all []domain.MunicipalAlert
	for _, alerts := range c.items {
		all = append(all, alerts...)
	}
	if all == nil {
		all = []domain.MunicipalAlert{}
	}
	return domain.MunicipalAlerts{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Alerts:    all,
	}
}
