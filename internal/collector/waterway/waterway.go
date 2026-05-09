package waterway

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/collector/municipal"
	"github.com/rmrobinson/cupola/internal/config"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// GaugeSource fetches the current readings for all gauges from a waterway data provider.
type GaugeSource interface {
	AllGauges(ctx context.Context) ([]domain.WaterwayGauge, error)
}

var (
	sourceMu     sync.RWMutex
	gaugeSources = map[string]func() GaugeSource{}
)

// RegisterGaugeSource registers a named GaugeSource factory.
// Call from parser sub-package init() functions.
func RegisterGaugeSource(name string, factory func() GaugeSource) {
	sourceMu.Lock()
	gaugeSources[name] = factory
	sourceMu.Unlock()
}

type sourceEntry struct {
	cfg    config.WaterwayConfig
	source GaugeSource
}

// Collector polls one or more waterway gauge sources and publishes the
// combined result as domain.WaterwayConditions. When a gauge's AdvisoryStatus
// matches an entry in the source config's AlertOn list the gauge is also
// promoted into municipal.alerts via alertsColl (may be nil).
type Collector struct {
	sources    []sourceEntry
	stateStore *store.StateStore
	alertsColl *municipal.AlertsCollector
	netCheck   func() bool
	mu         sync.RWMutex
	gauges     map[string][]domain.WaterwayGauge // sourceID → gauges
}

func (c *Collector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func NewCollector(
	cfgs []config.WaterwayConfig,
	ss *store.StateStore,
	ac *municipal.AlertsCollector,
) *Collector {
	c := &Collector{
		stateStore: ss,
		alertsColl: ac,
		gauges:     make(map[string][]domain.WaterwayGauge),
	}
	sourceMu.RLock()
	defer sourceMu.RUnlock()
	for _, cfg := range cfgs {
		factory, ok := gaugeSources[cfg.Parser]
		if !ok {
			log.Printf("[waterway] unknown parser %q for source %s — skipping", cfg.Parser, cfg.ID)
			continue
		}
		c.sources = append(c.sources, sourceEntry{cfg: cfg, source: factory()})
	}
	return c
}

func (c *Collector) ID() string                { return "waterway.conditions" }
func (c *Collector) Domain() domain.DomainType { return domain.DomainWaterwayConditions }

func (c *Collector) Start(ctx context.Context) error {
	for _, s := range c.sources {
		go c.runSource(ctx, s)
	}
	return nil
}

func (c *Collector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.buildState()
}

func (c *Collector) runSource(ctx context.Context, s sourceEntry) {
	if c.netCheck == nil || c.netCheck() {
		if err := c.fetchSource(ctx, s); err != nil {
			log.Printf("[waterway] %s initial fetch: %v", s.cfg.ID, err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.sourceID(s.cfg.ID), Status: "error", Message: err.Error(),
			})
		}
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
			if c.netCheck != nil && !c.netCheck() {
				continue
			}
			if err := c.fetchSource(ctx, s); err != nil {
				log.Printf("[waterway] %s fetch: %v", s.cfg.ID, err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.sourceID(s.cfg.ID), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.sourceID(s.cfg.ID), Status: "ok"})
			}
		}
	}
}

func (c *Collector) fetchSource(ctx context.Context, s sourceEntry) error {
	gauges, err := s.source.AllGauges(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.gauges[s.cfg.ID] = gauges
	state := c.buildState()
	c.mu.Unlock()

	c.stateStore.Set(state)
	log.Printf("[waterway] %s updated: %d gauges", s.cfg.ID, len(gauges))

	if c.alertsColl != nil && len(s.cfg.AlertOn) > 0 {
		c.promoteAlerts(s.cfg.ID, gauges, s.cfg.AlertOn)
	}
	return nil
}

func (c *Collector) sourceID(sourceID string) string {
	return c.ID() + ":" + sourceID
}

func (c *Collector) promoteAlerts(sourceID string, gauges []domain.WaterwayGauge, alertOn []string) {
	alertSet := make(map[string]bool, len(alertOn))
	for _, a := range alertOn {
		alertSet[a] = true
	}

	var alerts []domain.MunicipalAlert
	for _, g := range gauges {
		if !alertSet[g.AdvisoryStatus] {
			continue
		}
		var text string
		if g.AdvisoryText != nil {
			text = *g.AdvisoryText
		}
		sev := domain.SeverityInfo
		switch g.AdvisoryStatus {
		case "warning":
			sev = domain.SeverityWarning
		case "emergency":
			sev = domain.SeverityEmergency
		case "watch":
			sev = domain.SeverityWatch
		}
		area := g.Name
		alerts = append(alerts, domain.MunicipalAlert{
			ID:          "waterway:" + g.ID,
			SourceID:    "waterway:" + sourceID,
			Title:       g.Name + " — " + g.AdvisoryStatus,
			Description: text,
			AlertType:   "flood",
			Severity:    sev,
			Area:        &area,
			PublishedAt: g.UpdatedAt,
		})
	}
	c.alertsColl.SetSourceAlerts("waterway:"+sourceID, alerts)
}

func (c *Collector) buildState() domain.WaterwayConditions {
	var all []domain.WaterwayGauge
	for _, gauges := range c.gauges {
		all = append(all, gauges...)
	}
	if all == nil {
		all = []domain.WaterwayGauge{}
	}
	return domain.WaterwayConditions{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Gauges:    all,
	}
}
