package envcanada

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// AlertsCollector polls the Environment Canada weather alerts RSS feed for the
// nearest reporting station. Station discovery is shared with ForecastCollector
// via the package-level sync.Once cache in station.go.
type AlertsCollector struct {
	userLat    float64
	userLon    float64
	interval   time.Duration
	stateStore *store.StateStore
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.WeatherAlerts
}

func NewAlertsCollector(lat, lon float64, interval time.Duration, stateStore *store.StateStore) *AlertsCollector {
	return &AlertsCollector{
		userLat:    lat,
		userLon:    lon,
		interval:   interval,
		stateStore: stateStore,
	}
}

func (c *AlertsCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *AlertsCollector) ID() string                { return "envcanada.alerts" }
func (c *AlertsCollector) Domain() domain.DomainType { return domain.DomainWeatherAlerts }

func (c *AlertsCollector) Start(ctx context.Context) error {
	go func() {
		stLat, stLon, err := getNearestStation(c.userLat, c.userLon)
		if err != nil {
			log.Printf("[envcanada.alerts] station discovery: %v", err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
			return
		}
		url := stationRSSURL("alerts", stLat, stLon)
		if c.netCheck == nil || c.netCheck() {
			if err := c.fetch(url); err != nil {
				log.Printf("[envcanada.alerts] initial fetch: %v", err)
			}
		}
		c.loop(ctx, url)
	}()
	return nil
}

func (c *AlertsCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *AlertsCollector) loop(ctx context.Context, url string) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if c.netCheck != nil && !c.netCheck() {
				continue
			}
			if err := c.fetch(url); err != nil {
				log.Printf("[envcanada.alerts] fetch: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
			}
		}
	}
}

func (c *AlertsCollector) fetch(url string) error {
	feed, err := fetchAtom(url)
	if err != nil {
		return err
	}

	var alerts []domain.WeatherAlert
	now := time.Now().UTC()

	for _, e := range feed.Entries {
		if e.Category.Term != "Warnings and Watches" {
			continue
		}
		title := strings.TrimSpace(e.Title)
		if strings.Contains(strings.ToLower(title), "no watches or warnings") {
			continue
		}

		onset := parseAtomTime(e.Updated)
		if onset.IsZero() {
			onset = now
		}
		alerts = append(alerts, domain.WeatherAlert{
			ID:        e.ID,
			Title:     title,
			Severity:  alertSeverity(title),
			Onset:     onset,
			Expires:   onset.Add(24 * time.Hour),
			Summary:   stripHTML(e.Summary),
			SourceURL: e.Link.Href,
		})
	}
	if alerts == nil {
		alerts = []domain.WeatherAlert{}
	}

	state := domain.WeatherAlerts{
		StateBase: domain.StateBase{UpdatedAt: now},
		Alerts:    alerts,
	}

	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[envcanada.alerts] updated: %d active alerts", len(alerts))
	return nil
}

func alertSeverity(title string) domain.AlertSeverity {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "emergency"):
		return domain.SeverityEmergency
	case strings.Contains(lower, "warning"):
		return domain.SeverityWarning
	case strings.Contains(lower, "watch"):
		return domain.SeverityWatch
	default:
		return domain.SeverityInfo
	}
}

// fmtCoord formats a float coordinate without trailing zeros for use in RSS URLs.
func fmtCoord(f float64) string {
	return fmt.Sprintf("%v", f)
}
