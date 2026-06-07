package envcanada

import (
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// CurrentConditionsCollector polls the Environment Canada weather Atom feed for
// the current-conditions entry at the nearest reporting station.
type CurrentConditionsCollector struct {
	userLat    float64
	userLon    float64
	station    StationOverride
	interval   time.Duration
	stateStore *store.StateStore
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.WeatherCurrent
	wake       chan struct{}
}

func NewCurrentConditionsCollector(lat, lon float64, interval time.Duration, stateStore *store.StateStore, station StationOverride) *CurrentConditionsCollector {
	return &CurrentConditionsCollector{
		userLat:    lat,
		userLon:    lon,
		station:    station,
		interval:   interval,
		stateStore: stateStore,
		wake:       make(chan struct{}, 1),
	}
}

func (c *CurrentConditionsCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *CurrentConditionsCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *CurrentConditionsCollector) ID() string                { return "envcanada.current" }
func (c *CurrentConditionsCollector) Domain() domain.DomainType { return domain.DomainWeatherCurrent }

func (c *CurrentConditionsCollector) Start(ctx context.Context) error {
	go func() {
		var url string
		if c.netCheck == nil || c.netCheck() {
			resolved, err := c.resolveURL()
			if err != nil {
				log.Printf("[envcanada.current] station discovery: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				url = resolved
			}
		}
		if url != "" {
			if err := c.fetch(url); err != nil {
				log.Printf("[envcanada.current] initial fetch: %v", err)
			}
		}
		c.loop(ctx, url)
	}()
	return nil
}

func (c *CurrentConditionsCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *CurrentConditionsCollector) loop(ctx context.Context, url string) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			url = c.fetchIfReady(url)
		case <-c.wake:
			url = c.fetchIfReady(url)
		}
	}
}

func (c *CurrentConditionsCollector) fetchIfReady(url string) string {
	if c.netCheck != nil && !c.netCheck() {
		return url
	}
	if url == "" {
		resolved, err := c.resolveURL()
		if err != nil {
			log.Printf("[envcanada.current] station discovery: %v", err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
			return url
		}
		url = resolved
	}
	if err := c.fetch(url); err != nil {
		log.Printf("[envcanada.current] fetch: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: c.ID(), Status: "error", Message: err.Error(),
		})
	} else {
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	}
	return url
}

func (c *CurrentConditionsCollector) resolveURL() (string, error) {
	stLat, stLon, err := resolveStation(c.userLat, c.userLon, c.station)
	if err != nil {
		return "", err
	}
	return stationRSSURL("weather", stLat, stLon), nil
}

func (c *CurrentConditionsCollector) fetch(url string) error {
	feed, err := fetchAtom(url)
	if err != nil {
		return err
	}
	state, err := parseCurrentConditionsFeed(feed, time.Now())
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[envcanada.current] %.1fC (feels %.1fC) hum=%.0f%% wind=%.1fkm/h dir=%ddeg gust=%.1fkm/h pres=%.1fhPa",
		state.Temperature, state.FeelsLike, state.Humidity,
		state.WindSpeed, state.WindDirection, state.WindGust, state.Pressure)
	return nil
}

func parseCurrentConditionsFeed(feed *atomFeed, now time.Time) (domain.WeatherCurrent, error) {
	if feed == nil {
		return domain.WeatherCurrent{}, fmt.Errorf("missing Atom feed")
	}
	for _, e := range feed.Entries {
		if !isCurrentConditionsEntry(e) {
			continue
		}
		return parseCurrentConditionsEntry(e, now), nil
	}
	return domain.WeatherCurrent{}, fmt.Errorf("no current conditions entry found")
}

func isCurrentConditionsEntry(e atomEntry) bool {
	category := strings.ToLower(strings.TrimSpace(e.Category.Term))
	return strings.Contains(category, "current condition") ||
		strings.Contains(strings.ToLower(e.Title), "current conditions")
}

func parseCurrentConditionsEntry(e atomEntry, now time.Time) domain.WeatherCurrent {
	fields := currentConditionFields(e.Summary)
	state := domain.WeatherCurrent{StateBase: domain.StateBase{UpdatedAt: now}}

	if condition := firstNonEmpty(fields, "Condition"); condition != "" {
		state.Condition = condition
	} else if condition := conditionFromCurrentTitle(e.Title); condition != "" {
		state.Condition = condition
	}
	if v, ok := parseNumberField(firstNonEmpty(fields, "Temperature")); ok {
		state.Temperature = v
	} else if v, ok := temperatureFromCurrentTitle(e.Title); ok {
		state.Temperature = v
	}
	if v, ok := parseNumberField(firstNonEmpty(fields, "Humidity", "Relative Humidity")); ok {
		state.Humidity = v
	}
	if v, ok := parsePressure(firstNonEmpty(fields, "Pressure / Tendency", "Pressure")); ok {
		state.Pressure = v
	}
	if v, ok := parseNumberField(firstNonEmpty(fields, "Visibility")); ok {
		state.Visibility = v
	}
	if v, ok := parseNumberField(firstNonEmpty(fields, "Humidex")); ok {
		state.FeelsLike = v
	} else if v, ok := parseNumberField(firstNonEmpty(fields, "Wind Chill")); ok {
		state.FeelsLike = v
	}
	state.WindDirection, state.WindSpeed, state.WindGust = parseCurrentWind(firstNonEmpty(fields, "Wind"))

	return state
}

var brRe = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)

func currentConditionFields(summary string) map[string]string {
	s := brRe.ReplaceAllString(summary, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	out := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		label, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		label = strings.Join(strings.Fields(label), " ")
		value = strings.TrimSpace(value)
		if label != "" && value != "" {
			out[label] = value
		}
	}
	return out
}

func firstNonEmpty(fields map[string]string, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(fields[name]); v != "" {
			return v
		}
	}
	return ""
}

var (
	numberRe = regexp.MustCompile(`[-+]?\d+(?:\.\d+)?`)
	gustRe   = regexp.MustCompile(`(?i)\bgust(?:ing)?\s+([-+]?\d+(?:\.\d+)?)`)
)

func parseNumberField(s string) (float64, bool) {
	m := numberRe.FindString(s)
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(m, 64)
	return v, err == nil
}

func parsePressure(s string) (float64, bool) {
	v, ok := parseNumberField(s)
	if !ok {
		return 0, false
	}
	if strings.Contains(strings.ToLower(s), "kpa") {
		v *= 10
	}
	return v, true
}

func parseCurrentWind(s string) (direction int, speed, gust float64) {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" || strings.Contains(lower, "calm") {
		return 0, 0, 0
	}
	tokens := strings.Fields(strings.ReplaceAll(s, "\u00a0", " "))
	if len(tokens) > 0 {
		direction = compassDegrees(tokens[0])
	}
	if m := gustRe.FindStringSubmatch(s); len(m) == 2 {
		gust, _ = strconv.ParseFloat(m[1], 64)
	}
	if v, ok := parseNumberField(s); ok {
		speed = v
	}
	return direction, speed, gust
}

func compassDegrees(s string) int {
	s = strings.ToUpper(strings.Trim(strings.TrimSpace(s), "."))
	switch s {
	case "N", "NORTH":
		return 0
	case "NNE":
		return 23
	case "NE", "NORTHEAST":
		return 45
	case "ENE":
		return 68
	case "E", "EAST":
		return 90
	case "ESE":
		return 113
	case "SE", "SOUTHEAST":
		return 135
	case "SSE":
		return 158
	case "S", "SOUTH":
		return 180
	case "SSW":
		return 203
	case "SW", "SOUTHWEST":
		return 225
	case "WSW":
		return 248
	case "W", "WEST":
		return 270
	case "WNW":
		return 293
	case "NW", "NORTHWEST":
		return 315
	case "NNW":
		return 338
	default:
		return 0
	}
}

func conditionFromCurrentTitle(title string) string {
	_, rest, ok := strings.Cut(title, ":")
	if !ok {
		return ""
	}
	condition, _, _ := strings.Cut(strings.TrimSpace(rest), ",")
	return strings.TrimSpace(condition)
}

func temperatureFromCurrentTitle(title string) (float64, bool) {
	return parseNumberField(title)
}
