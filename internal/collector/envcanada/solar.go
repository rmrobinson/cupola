package envcanada

import (
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

// Data source: NOAA Space Weather Prediction Center planetary Kp index.
// This is the authoritative international source for geomagnetic activity.
// Space Weather Canada derives regional forecasts from the same NOAA data.
const noaaKpURL = "https://services.swpc.noaa.gov/products/noaa-planetary-k-index.json"

// selectSolarRegion returns the Space Weather Canada region (1–7) for a lat/lon.
//
//	1 BC Coast, 2 BC Interior/Prairies West, 3 Prairies East/MB,
//	4 Ontario, 5 Quebec, 6 Atlantic, 7 North (lat > 60)
func selectSolarRegion(lat, lon float64) int {
	if lat > 60 {
		return 7
	}
	switch {
	case lon < -120:
		return 1
	case lon < -110:
		return 2
	case lon < -95:
		return 3
	case lon < -74:
		return 4
	case lon < -60:
		return 5
	case lon < -52:
		return 6
	default:
		return 4
	}
}

func kpDesc(kp float64) string {
	switch {
	case kp < 2:
		return "quiet"
	case kp < 4:
		return "unsettled"
	case kp < 5:
		return "active"
	case kp < 6:
		return "minor storm"
	case kp < 7:
		return "moderate storm"
	case kp < 8:
		return "strong storm"
	case kp < 9:
		return "severe storm"
	default:
		return "extreme storm"
	}
}

// auroraViewable returns true when the Kp index suggests aurora at the given latitude.
// Formula: aurora oval extends to approximately (67 - kp*2.5)° geographic latitude.
func auroraViewable(kp, lat float64) bool {
	return kp >= 5 && lat >= 67-kp*2.5
}

// sharedSolar holds the parsed state from one fetch and is shared by both
// SolarCurrentCollector and SolarForecastCollector.
type sharedSolar struct {
	mu       sync.RWMutex
	current  domain.SolarWeatherCurrent
	forecast domain.SolarWeatherForecast
}

// SolarCurrentCollector drives the shared fetch and publishes solar.weather.current.
type SolarCurrentCollector struct {
	shared     *sharedSolar
	region     int
	lat        float64
	interval   time.Duration
	stateStore *store.StateStore
	netCheck   func() bool
	wake       chan struct{}
}

func (c *SolarCurrentCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

// SolarForecastCollector is a lightweight companion that publishes solar.weather.forecast
// whenever SolarCurrentCollector refreshes the shared state.
type SolarForecastCollector struct {
	shared         *sharedSolar
	stateStore     *store.StateStore
	requestRefresh func()
}

// NewSolarCollectors creates a pair of collectors that share one NOAA SWPC fetch per cycle.
func NewSolarCollectors(lat, lon float64, interval time.Duration, stateStore *store.StateStore, overrideRegion *int) (*SolarCurrentCollector, *SolarForecastCollector) {
	region := selectSolarRegion(lat, lon)
	if overrideRegion != nil {
		region = *overrideRegion
	}
	shared := &sharedSolar{}
	cur := &SolarCurrentCollector{
		shared:     shared,
		region:     region,
		lat:        lat,
		interval:   interval,
		stateStore: stateStore,
		wake:       make(chan struct{}, 1),
	}
	fc := &SolarForecastCollector{shared: shared, stateStore: stateStore, requestRefresh: cur.OnSubscription}
	return cur, fc
}

// ── SolarCurrentCollector ─────────────────────────────────────────────────────

func (c *SolarCurrentCollector) ID() string                { return "envcanada.solar" }
func (c *SolarCurrentCollector) Domain() domain.DomainType { return domain.DomainSolarWeatherCurrent }

func (c *SolarCurrentCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *SolarCurrentCollector) Start(ctx context.Context) error {
	go func() {
		if c.netCheck == nil || c.netCheck() {
			if err := c.fetch(); err != nil {
				log.Printf("[envcanada.solar] initial fetch: %v", err)
			}
		}
		c.loop(ctx)
	}()
	return nil
}

func (c *SolarCurrentCollector) State() domain.DomainState {
	c.shared.mu.RLock()
	defer c.shared.mu.RUnlock()
	return c.shared.current
}

func (c *SolarCurrentCollector) loop(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.fetchIfReady()
		case <-c.wake:
			c.fetchIfReady()
		}
	}
}

func (c *SolarCurrentCollector) fetchIfReady() {
	if c.netCheck != nil && !c.netCheck() {
		return
	}
	if err := c.fetch(); err != nil {
		log.Printf("[envcanada.solar] fetch: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: c.ID(), Status: "error", Message: err.Error(),
		})
	} else {
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	}
}

// ── SolarForecastCollector ────────────────────────────────────────────────────

func (c *SolarForecastCollector) ID() string                    { return "envcanada.solar.forecast" }
func (c *SolarForecastCollector) Domain() domain.DomainType     { return domain.DomainSolarWeatherForecast }
func (c *SolarForecastCollector) Start(_ context.Context) error { return nil }

func (c *SolarForecastCollector) OnSubscription() {
	if c.requestRefresh != nil {
		c.requestRefresh()
	}
}

func (c *SolarForecastCollector) State() domain.DomainState {
	c.shared.mu.RLock()
	defer c.shared.mu.RUnlock()
	return c.shared.forecast
}

// ── NOAA SWPC fetch ───────────────────────────────────────────────────────────

// fetch downloads the NOAA planetary Kp index and handles both response shapes
// that SWPC has returned over time:
//
//	2-D array:  [["time_tag","Kp",...], ["2026-04-27 00:00:00","1.33",...], ...]
//	Object array: [{"time_tag":"...","Kp":"1.33",...}, ...]
func (c *SolarCurrentCollector) fetch() error {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(noaaKpURL)
	if err != nil {
		return fmt.Errorf("get NOAA Kp: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get NOAA Kp: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	// Decode the outer array generically so we can handle either inner shape.
	var outer []json.RawMessage
	if err := json.Unmarshal(body, &outer); err != nil {
		preview := string(body)
		if len(preview) > 400 {
			preview = preview[:400] + "…"
		}
		return fmt.Errorf("parse JSON: %w\nresponse: %s", err, preview)
	}
	if len(outer) < 2 {
		return fmt.Errorf("NOAA Kp: too few rows (%d)", len(outer))
	}

	// Find the last data row and extract Kp, skipping the header row
	// (which is either a string array or an object without a numeric Kp).
	kp, err := extractKpFromRows(outer)
	if err != nil {
		return err
	}

	// Build 3-period forecast from the last 3 rows (each covers 3 h).
	now := time.Now().UTC()
	aurora := auroraViewable(kp, c.lat)

	current := domain.SolarWeatherCurrent{
		StateBase:      domain.StateBase{UpdatedAt: now},
		KpIndex:        kp,
		KpDescription:  kpDesc(kp),
		AuroraViewable: aurora,
		Region:         c.region,
	}

	// Build a simple look-ahead forecast from the last few rows in outer.
	var periods []domain.SolarForecastPeriod
	start := len(outer) - 4
	if start < 1 {
		start = 1
	}
	for i, raw := range outer[start:] {
		fKp, err := extractKpFromRow(raw)
		if err != nil || fKp <= 0 {
			continue
		}
		s := now.Add(time.Duration(i) * 3 * time.Hour)
		periods = append(periods, domain.SolarForecastPeriod{
			StartsAt:       s,
			EndsAt:         s.Add(3 * time.Hour),
			KpExpected:     fKp,
			KpDescription:  kpDesc(fKp),
			AuroraViewable: auroraViewable(fKp, c.lat),
		})
	}

	forecast := domain.SolarWeatherForecast{
		StateBase: domain.StateBase{UpdatedAt: now},
		Periods:   periods,
	}

	c.shared.mu.Lock()
	c.shared.current = current
	c.shared.forecast = forecast
	c.shared.mu.Unlock()

	c.stateStore.Set(current)
	c.stateStore.Set(forecast)
	log.Printf("[envcanada.solar] updated: Kp=%.2f (%s) region=%d aurora=%v",
		kp, kpDesc(kp), c.region, aurora)
	return nil
}

// extractKpFromRows iterates outer (newest-last), skipping the header, and
// returns the Kp value from the last row that successfully parses.
func extractKpFromRows(outer []json.RawMessage) (float64, error) {
	for i := len(outer) - 1; i >= 0; i-- {
		kp, err := extractKpFromRow(outer[i])
		if err == nil && kp >= 0 {
			return kp, nil
		}
	}
	return 0, fmt.Errorf("no valid Kp value found in NOAA response (%d rows)", len(outer))
}

// extractKpFromRow handles both shapes a NOAA SWPC row can take:
//
//	Array:  ["2026-04-27 00:00:00", "1.33", ...]   → Kp at index 1
//	Object: {"time_tag":"...","Kp":"1.33",...}      → Kp from "Kp"/"kp"/"kp_index" key
func extractKpFromRow(raw json.RawMessage) (float64, error) {
	// Try array form first.
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil && len(arr) >= 2 {
		s := strings.Trim(string(arr[1]), `"`)
		kp, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return kp, nil
		}
	}

	// Try object form.
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return 0, fmt.Errorf("row is neither array nor object")
	}
	for _, key := range []string{"Kp", "kp", "kp_index", "KP"} {
		if v, ok := obj[key]; ok {
			s := strings.Trim(string(v), `"`)
			kp, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return kp, nil
			}
		}
	}
	return 0, fmt.Errorf("kp key not found in object row")
}
