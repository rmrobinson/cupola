package envcanada

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// HourlyForecastCollector polls the Environment Canada hourly forecast page
// for the nearest reporting station and parses its embedded Vue SSR state.
type HourlyForecastCollector struct {
	userLat    float64
	userLon    float64
	interval   time.Duration
	stateStore *store.StateStore
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.WeatherHourlyForecast
}

func NewHourlyForecastCollector(lat, lon float64, interval time.Duration, stateStore *store.StateStore) *HourlyForecastCollector {
	return &HourlyForecastCollector{
		userLat:    lat,
		userLon:    lon,
		interval:   interval,
		stateStore: stateStore,
	}
}

func (c *HourlyForecastCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *HourlyForecastCollector) ID() string { return "envcanada.hourly_forecast" }
func (c *HourlyForecastCollector) Domain() domain.DomainType {
	return domain.DomainWeatherForecastHourly
}

func (c *HourlyForecastCollector) Start(ctx context.Context) error {
	go func() {
		stLat, stLon, err := getNearestStation(c.userLat, c.userLon)
		if err != nil {
			log.Printf("[envcanada.hourly_forecast] station discovery: %v", err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
			return
		}
		url := stationHourlyForecastURL(stLat, stLon)
		if c.netCheck == nil || c.netCheck() {
			if err := c.fetch(url); err != nil {
				log.Printf("[envcanada.hourly_forecast] initial fetch: %v", err)
			}
		}
		c.loop(ctx, url)
	}()
	return nil
}

func (c *HourlyForecastCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *HourlyForecastCollector) loop(ctx context.Context, url string) {
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
				log.Printf("[envcanada.hourly_forecast] fetch: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
			}
		}
	}
}

func (c *HourlyForecastCollector) fetch(url string) error {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	hours, err := parseHourlyForecastHTML(body)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	c.mu.RLock()
	previous := c.state.Hours
	c.mu.RUnlock()
	hours = retainActivePreviousHour(hours, previous, now)

	state := domain.WeatherHourlyForecast{
		StateBase: domain.StateBase{UpdatedAt: now},
		Hours:     hours,
	}

	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[envcanada.hourly_forecast] updated: %d hours", len(hours))
	return nil
}

func retainActivePreviousHour(hours, previous []domain.HourlyForecastPeriod, now time.Time) []domain.HourlyForecastPeriod {
	if len(previous) == 0 {
		return hours
	}
	for _, prev := range previous {
		if now.Before(prev.StartsAt) || !now.Before(prev.EndsAt) {
			continue
		}
		for _, h := range hours {
			if h.StartsAt.Equal(prev.StartsAt) {
				return hours
			}
		}
		out := make([]domain.HourlyForecastPeriod, 0, len(hours)+1)
		out = append(out, prev)
		out = append(out, hours...)
		return out
	}
	return hours
}

type hourlyInitialState struct {
	Location struct {
		Hourly    []hourlyPeriod `json:"hourly"`
		Locations map[string]struct {
			Hourly []hourlyPeriod `json:"hourly"`
		} `json:"location"`
	} `json:"location"`
}

type hourlyPeriod struct {
	EpochTime   int64  `json:"epochTime"`
	Condition   string `json:"condition"`
	Precip      string `json:"precip"`
	WindDir     string `json:"windDir"`
	IconCode    string `json:"iconCode"`
	Temperature struct {
		Metric string `json:"metric"`
	} `json:"temperature"`
	FeelsLike struct {
		Metric string `json:"metric"`
	} `json:"feelsLike"`
	WindSpeed struct {
		Metric string `json:"metric"`
	} `json:"windSpeed"`
	WindGust struct {
		Metric string `json:"metric"`
	} `json:"windGust"`
	Humidex struct {
		Metric string `json:"metric"`
	} `json:"humidex"`
	WindChill struct {
		Metric string `json:"metric"`
	} `json:"windChill"`
	UV struct {
		Index string `json:"index"`
	} `json:"uv"`
}

func parseHourlyForecastHTML(body []byte) ([]domain.HourlyForecastPeriod, error) {
	raw, err := extractInitialStateJSON(body)
	if err != nil {
		return nil, err
	}

	var state hourlyInitialState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("parse hourly SSR JSON: %w", err)
	}
	hourly := state.Location.Hourly
	if len(hourly) == 0 {
		for _, loc := range state.Location.Locations {
			if len(loc.Hourly) > 0 {
				hourly = loc.Hourly
				break
			}
		}
	}
	if len(hourly) == 0 {
		return nil, fmt.Errorf("hourly forecast has no periods")
	}

	hours := make([]domain.HourlyForecastPeriod, 0, len(hourly))
	for _, h := range hourly {
		if h.EpochTime == 0 {
			return nil, fmt.Errorf("hourly forecast period missing epochTime")
		}
		start := time.Unix(h.EpochTime, 0).UTC()
		period := domain.HourlyForecastPeriod{
			StartsAt:      start,
			EndsAt:        start.Add(time.Hour),
			Condition:     strings.TrimSpace(h.Condition),
			Temperature:   metricFloatPtr(h.Temperature.Metric),
			FeelsLike:     metricFloatPtr(h.FeelsLike.Metric),
			PrecipChance:  intPtr(h.Precip),
			WindDirection: strings.TrimSpace(h.WindDir),
			WindSpeed:     metricFloatPtr(h.WindSpeed.Metric),
			WindGust:      metricFloatPtr(h.WindGust.Metric),
			Humidex:       metricFloatPtr(h.Humidex.Metric),
			WindChill:     metricFloatPtr(h.WindChill.Metric),
			UVIndex:       metricFloatPtr(h.UV.Index),
			IconURL:       hourlyIconURL(h.IconCode),
		}
		hours = append(hours, period)
	}
	return hours, nil
}

func extractInitialStateJSON(body []byte) ([]byte, error) {
	markers := [][]byte{
		[]byte("window.__INITIAL_STATE__="),
		[]byte("window.__INITIAL_STATE__ ="),
		[]byte("__INITIAL_STATE__="),
		[]byte("__INITIAL_STATE__ ="),
	}
	for _, marker := range markers {
		if i := bytes.Index(body, marker); i >= 0 {
			start := i + len(marker)
			for start < len(body) && (body[start] == ' ' || body[start] == '\n' || body[start] == '\t') {
				start++
			}
			return decodeJSONObject(body[start:])
		}
	}

	hourly := bytes.Index(body, []byte(`"hourly"`))
	if hourly < 0 {
		return nil, fmt.Errorf("missing hourly SSR state")
	}
	start := bytes.LastIndex(body[:hourly], []byte("{"))
	if start < 0 {
		return nil, fmt.Errorf("missing hourly SSR state")
	}
	return decodeJSONObject(body[start:])
}

func decodeJSONObject(body []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse hourly SSR JSON: %w", err)
	}
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return nil, fmt.Errorf("hourly SSR state is not a JSON object")
	}
	return raw, nil
}

func metricFloatPtr(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func intPtr(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

func hourlyIconURL(iconCode string) string {
	iconCode = strings.TrimSpace(iconCode)
	if iconCode == "" {
		return ""
	}
	return "https://weather.gc.ca/weathericons/small/" + iconCode + ".png"
}
