package envcanada

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// ForecastCollector polls the Environment Canada weather forecast RSS feed for
// the nearest reporting station to the configured location.
// Station discovery (fetching all 13 province pages) is performed once on
// first Start() and the result is shared with AlertsCollector via a package-
// level sync.Once cache.
type ForecastCollector struct {
	userLat    float64
	userLon    float64
	interval   time.Duration
	stateStore *store.StateStore
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.WeatherForecast
}

func NewForecastCollector(lat, lon float64, interval time.Duration, stateStore *store.StateStore) *ForecastCollector {
	return &ForecastCollector{
		userLat:    lat,
		userLon:    lon,
		interval:   interval,
		stateStore: stateStore,
	}
}

func (c *ForecastCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *ForecastCollector) ID() string                { return "envcanada.forecast" }
func (c *ForecastCollector) Domain() domain.DomainType { return domain.DomainWeatherForecast }

func (c *ForecastCollector) Start(ctx context.Context) error {
	go func() {
		stLat, stLon, err := getNearestStation(c.userLat, c.userLon)
		if err != nil {
			log.Printf("[envcanada.forecast] station discovery: %v", err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
			return
		}
		url := stationRSSURL("weather", stLat, stLon)
		if c.netCheck == nil || c.netCheck() {
			if err := c.fetch(url); err != nil {
				log.Printf("[envcanada.forecast] initial fetch: %v", err)
			}
		}
		c.loop(ctx, url)
	}()
	return nil
}

func (c *ForecastCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *ForecastCollector) loop(ctx context.Context, url string) {
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
				log.Printf("[envcanada.forecast] fetch: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
			}
		}
	}
}

func (c *ForecastCollector) fetch(url string) error {
	feed, err := fetchAtom(url)
	if err != nil {
		return err
	}

	var periods []domain.ForecastPeriod
	var issueTime time.Time

	for _, e := range feed.Entries {
		if e.Category.Term != "Weather Forecasts" {
			continue
		}
		if issueTime.IsZero() {
			issueTime = parseAtomTime(e.Updated)
		}
		periods = append(periods, parseForecastEntry(e, len(periods), issueTime))
	}

	if issueTime.IsZero() {
		issueTime = time.Now()
	}

	now := time.Now()
	state := domain.WeatherForecast{
		StateBase: domain.StateBase{UpdatedAt: now},
		Periods:   periods,
	}

	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[envcanada.forecast] updated: %d periods", len(periods))
	return nil
}

// ── Forecast title parsing ────────────────────────────────────────────────────
//
// Title format: "{label}: {condition}. {High|Low} {temp}[. POP {n}%]."
// Examples:
//
//	"Monday night: Chance of showers. Low 12. POP 30%"
//	"Tuesday: Showers. High 19."
//	"Thursday night: Clear. Low minus 1."
//	"Saturday: Chance of showers. High plus 5. POP 30%"

var (
	highRe = regexp.MustCompile(`(?i)\bHigh\s+((?:minus |plus )?(?:\d+|zero))\b`)
	lowRe  = regexp.MustCompile(`(?i)\bLow\s+((?:minus |plus )?(?:\d+|zero))\b`)
	popRe  = regexp.MustCompile(`(?i)\bPOP\s+(\d+)%`)
	hiLoRe = regexp.MustCompile(`(?i)\s*[,.]?\s*\b(?:High|Low)\b`)
)

func parseForecastEntry(e atomEntry, idx int, issueTime time.Time) domain.ForecastPeriod {
	title := e.Title
	label, rest, _ := strings.Cut(title, ": ")
	label = strings.TrimSpace(label)
	isNight := strings.Contains(strings.ToLower(label), "night")

	condition := strings.TrimSpace(rest)
	if loc := hiLoRe.FindStringIndex(rest); loc != nil {
		condition = strings.Trim(rest[:loc[0]], ". ")
	}

	var high, low *float64
	if m := highRe.FindStringSubmatch(title); len(m) >= 2 && !isNight {
		v := parseTemp(m[1])
		high = &v
	}
	if m := lowRe.FindStringSubmatch(title); len(m) >= 2 && isNight {
		v := parseTemp(m[1])
		low = &v
	}

	pop := 0
	if m := popRe.FindStringSubmatch(title); len(m) >= 2 {
		pop, _ = strconv.Atoi(m[1])
	}

	start := issueTime.Add(time.Duration(idx) * 12 * time.Hour)
	return domain.ForecastPeriod{
		StartsAt:     start,
		EndsAt:       start.Add(12 * time.Hour),
		Label:        label,
		High:         high,
		Low:          low,
		Condition:    condition,
		PrecipChance: pop,
		Summary:      stripHTML(e.Summary),
	}
}

func parseTemp(s string) float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "zero":
		return 0
	case strings.HasPrefix(s, "minus "):
		v, _ := strconv.ParseFloat(strings.TrimPrefix(s, "minus "), 64)
		return -v
	case strings.HasPrefix(s, "plus "):
		v, _ := strconv.ParseFloat(strings.TrimPrefix(s, "plus "), 64)
		return v
	default:
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
}

// ── Shared Atom helpers ───────────────────────────────────────────────────────

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string `xml:"title"`
	Summary   string `xml:"summary"`
	Updated   string `xml:"updated"`
	Published string `xml:"published"`
	Category  struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
	Link struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	ID string `xml:"id"`
}

func fetchAtom(url string) (*atomFeed, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}
	return &feed, nil
}

func parseAtomTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, strings.TrimSpace(s))
	return t
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	for old, repl := range map[string]string{
		"&amp;": "&", "&lt;": "<", "&gt;": ">", "&deg;": "°", "&#176;": "°",
	} {
		s = strings.ReplaceAll(s, old, repl)
	}
	return strings.Join(strings.Fields(s), " ")
}
