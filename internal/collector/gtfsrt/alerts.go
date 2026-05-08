package gtfsrt

import (
	"context"
	"html"
	"log"
	"regexp"
	"strings"
	"time"

	pb "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// AlertsCollector polls GTFS-RT service alert feeds and publishes transit.alerts.
type AlertsCollector struct {
	agencies AgencySource
	state    *store.StateStore
	interval time.Duration
	wake     chan struct{}
}

func (c *AlertsCollector) ID() string                { return "gtfsrt.alerts" }
func (c *AlertsCollector) Domain() domain.DomainType { return domain.DomainTransitAlerts }
func (c *AlertsCollector) State() domain.DomainState { return c.state.Get(c.Domain()) }

func (c *AlertsCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *AlertsCollector) Start(ctx context.Context) error {
	go func() {
		c.fetch()
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.fetch()
			case <-c.wake:
				c.fetch()
			}
		}
	}()
	return nil
}

func (c *AlertsCollector) fetch() {
	var alerts []domain.TransitAlert
	seen := make(map[string]bool)

	for _, ag := range c.agencies.List() {
		if ag.AlertsURL == "" {
			continue
		}
		feed, err := fetchFeed(ag.AlertsURL)
		if err != nil {
			log.Printf("[gtfsrt] alerts %s: %v", ag.AlertsURL, err)
			continue
		}

		for _, entity := range feed.GetEntity() {
			alert := entity.GetAlert()
			if alert == nil {
				continue
			}

			id := ag.ID + ":" + entity.GetId()
			if seen[id] {
				continue
			}
			seen[id] = true

			rawDesc := translatedText(alert.GetDescriptionText())
			title := extractTitle(rawDesc)
			desc := stripHTML(rawDesc)

			var routes []string
			for _, e := range alert.GetInformedEntity() {
				if rid := e.GetRouteId(); rid != "" {
					routes = append(routes, rid)
				}
			}

			var startsAt, endsAt *time.Time
			for _, ap := range alert.GetActivePeriod() {
				if s := ap.GetStart(); s > 0 {
					t := time.Unix(int64(s), 0)
					if startsAt == nil || t.Before(*startsAt) {
						startsAt = &t
					}
				}
				if e := ap.GetEnd(); e > 0 {
					t := time.Unix(int64(e), 0)
					if endsAt == nil || t.After(*endsAt) {
						endsAt = &t
					}
				}
			}

			alerts = append(alerts, domain.TransitAlert{
				ID:             id,
				AgencyID:       ag.ID,
				Title:          title,
				Description:    desc,
				Severity:       severityFromEffect(alert.GetEffect()),
				AffectedRoutes: routes,
				StartsAt:       startsAt,
				EndsAt:         endsAt,
			})
		}
	}

	c.state.Set(domain.TransitAlerts{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Alerts:    alerts,
	})
}

func translatedText(ts *pb.TranslatedString) string {
	if ts == nil {
		return ""
	}
	for _, t := range ts.GetTranslation() {
		if t.GetLanguage() == "en" || t.GetLanguage() == "" {
			return t.GetText()
		}
	}
	if len(ts.GetTranslation()) > 0 {
		return ts.GetTranslation()[0].GetText()
	}
	return ""
}

func extractTitle(raw string) string {
	// GRT embeds the title in a leading <strong> tag.
	if i := strings.Index(raw, "<strong>"); i >= 0 {
		if j := strings.Index(raw[i:], "</strong>"); j >= 0 {
			return stripHTML(raw[i+8 : i+j])
		}
	}
	// Fall back to the first sentence of the plain text.
	plain := stripHTML(raw)
	if k := strings.IndexAny(plain, ".\n"); k > 0 {
		return strings.TrimSpace(plain[:k+1])
	}
	if len(plain) > 100 {
		return plain[:100] + "…"
	}
	return plain
}

func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

func severityFromEffect(e pb.Alert_Effect) domain.AlertSeverity {
	switch e {
	case pb.Alert_NO_SERVICE:
		return domain.SeverityWarning
	case pb.Alert_SIGNIFICANT_DELAYS:
		return domain.SeverityWatch
	default:
		return domain.SeverityInfo
	}
}
