package ecowitt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// Collector polls the Ecowitt GW2000 local HTTP API.
// The device exposes live sensor readings at GET /get_livedata_info in the
// following structure (verified against firmware on the actual device):
//
//	{"common_list":[{"id":"0x02","val":"20.8","unit":"C"}, ...],
//	 "piezoRain":  [{"id":"0x0E","val":"0.0 mm/Hr"}, ...],
//	 "wh25":       [{"intemp":"21.9","inhumi":"45%","rel":"979.2 hPa",...}]}
type Collector struct {
	url        string
	interval   time.Duration
	stateStore *store.StateStore
	mu         sync.RWMutex
	state      domain.WeatherCurrent
}

func New(baseURL string, interval time.Duration, stateStore *store.StateStore) *Collector {
	return &Collector{
		url:        strings.TrimRight(baseURL, "/") + "/get_livedata_info",
		interval:   interval,
		stateStore: stateStore,
	}
}

func (c *Collector) ID() string                { return "ecowitt.current" }
func (c *Collector) Domain() domain.DomainType { return domain.DomainWeatherCurrent }

func (c *Collector) Start(ctx context.Context) error {
	go func() {
		if err := c.fetch(); err != nil {
			log.Printf("[ecowitt.current] initial fetch: %v", err)
		}
		c.loop(ctx)
	}()
	return nil
}

func (c *Collector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Collector) loop(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.fetch(); err != nil {
				log.Printf("[ecowitt.current] fetch: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
			}
		}
	}
}

// ── JSON types ────────────────────────────────────────────────────────────────

type liveDataResponse struct {
	CommonList []sensorItem `json:"common_list"`
	PiezoRain  []sensorItem `json:"piezoRain"`
	WH25       []wh25Item   `json:"wh25"`
}

// sensorItem is one entry in the common_list or piezoRain arrays.
type sensorItem struct {
	ID   string `json:"id"`
	Val  string `json:"val"`
	Unit string `json:"unit"`
}

// wh25Item is the indoor/base-station sensor block.
type wh25Item struct {
	InTemp string `json:"intemp"`
	InHumi string `json:"inhumi"`
	Abs    string `json:"abs"` // absolute pressure, e.g. "979.2 hPa"
	Rel    string `json:"rel"` // relative pressure
	Unit   string `json:"unit"`
}

// ── Sensor ID table ───────────────────────────────────────────────────────────
//
// IDs observed from the actual device at /get_livedata_info:
//   0x02 → outdoor temperature (°C)
//   0x07 → outdoor humidity (%)
//   0x03 → dew point (°C)
//   0x0A → wind direction (°)
//   0x0B → wind speed (km/h)
//   0x0C → wind gust (km/h)
//   0x15 → solar radiation (Klux)
//   0x17 → UV index
//
// piezoRain IDs confirmed from device at /get_livedata_info:
//   0x0E → rain rate (mm/Hr)
//   0x0D → rain event (mm)
//   0x7C → rain past 24h (mm)
//   0x11 → rain weekly (mm)
//   0x12 → rain monthly (mm)
//   0x13 → rain yearly (mm)

const (
	idOutdoorTemp = "0x02"
	idHumidity    = "0x07"
	idDewPoint    = "0x03"
	idWindDir     = "0x0A"
	idWindSpeed   = "0x0B"
	idWindGust    = "0x0C"
	idSolar       = "0x15"
	idUVI         = "0x17"
	idRainRate    = "0x0E"
	idRainEvent   = "0x0D"
	idRainDaily   = "0x7C"
	idRainWeekly  = "0x11"
	idRainMonthly = "0x12"
	idRainYearly  = "0x13"
)

// ── Fetch ─────────────────────────────────────────────────────────────────────

func (c *Collector) fetch() error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(c.url)
	if err != nil {
		return fmt.Errorf("get %s: %w", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("get %s: status %d", c.url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	var data liveDataResponse
	if err := json.Unmarshal(body, &data); err != nil {
		preview := string(body)
		if len(preview) > 400 {
			preview = preview[:400] + "…"
		}
		return fmt.Errorf("parse JSON: %w\nresponse: %s", err, preview)
	}

	// Index sensors by ID for O(1) lookup.
	common := indexSensors(data.CommonList)
	piezo := indexSensors(data.PiezoRain)

	temp := parseVal(common[idOutdoorTemp])
	hum := parseVal(common[idHumidity])
	dewPoint := parseVal(common[idDewPoint])
	wind := parseVal(common[idWindSpeed])
	solar := parseVal(common[idSolar]) // Klux

	rainRate := parseVal(piezo[idRainRate])
	state := domain.WeatherCurrent{
		StateBase:     domain.StateBase{UpdatedAt: time.Now()},
		Temperature:   temp,
		Humidity:      hum,
		WindSpeed:     wind,
		WindGust:      parseVal(common[idWindGust]),
		WindDirection: int(parseVal(common[idWindDir])),
		UV:            parseVal(common[idUVI]),
		Precipitation: rainRate,
		RainEvent:     parseVal(piezo[idRainEvent]),
		RainDaily:     parseVal(piezo[idRainDaily]),
		RainWeekly:    parseVal(piezo[idRainWeekly]),
		RainMonthly:   parseVal(piezo[idRainMonthly]),
		RainYearly:    parseVal(piezo[idRainYearly]),
		Condition:     deriveCondition(solar, rainRate),
		FeelsLike:     feelsLike(temp, dewPoint, wind),
	}

	// Pressure from the wh25 indoor/base module.
	if len(data.WH25) > 0 {
		state.Pressure = parseVal(data.WH25[0].Rel)
	}

	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[ecowitt.current] %.1f°C (feels %.1f°C) hum=%.0f%% wind=%.1fkm/h dir=%d° UV=%.0f pres=%.1fhPa",
		state.Temperature, state.FeelsLike, state.Humidity,
		state.WindSpeed, state.WindDirection, state.UV, state.Pressure)
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// indexSensors builds a map from sensor ID to its raw value string.
func indexSensors(items []sensorItem) map[string]string {
	m := make(map[string]string, len(items))
	for _, it := range items {
		m[it.ID] = it.Val
	}
	return m
}

// parseVal extracts the leading numeric value from a string such as
// "20.8", "38%", "9.00 km/h", "979.2 hPa", "44.94 Klux".
func parseVal(s string) float64 {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for i, r := range s {
		if r == '-' && i == 0 {
			b.WriteRune(r)
		} else if r == '.' || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			break
		}
	}
	v, _ := strconv.ParseFloat(b.String(), 64)
	return v
}

// deriveCondition infers a sky condition from solar radiation (Klux) and rain rate (mm/h).
func deriveCondition(solarKlux, rainMmhr float64) string {
	if rainMmhr > 0 {
		return "rain"
	}
	switch {
	case solarKlux > 30:
		return "clear"
	case solarKlux > 10:
		return "partly cloudy"
	case solarKlux > 1:
		return "cloudy"
	default:
		return "overcast"
	}
}

// feelsLike computes apparent temperature using Environment Canada's wind chill
// and humidex formulas. Humidex is based on dew point/vapour pressure, not
// relative humidity.
func feelsLike(tempC, dewPointC, windKmh float64) float64 {
	if tempC <= 0 && windKmh > 4.8 {
		v := math.Pow(windKmh, 0.16)
		return 13.12 + 0.6215*tempC - 11.37*v + 0.3965*tempC*v
	}
	if tempC >= 20 {
		return math.Max(tempC, humidex(tempC, dewPointC))
	}
	return tempC
}

func humidex(tempC, dewPointC float64) float64 {
	dewPointK := dewPointC + 273.15
	if dewPointK <= 0 {
		return tempC
	}
	vapourPressure := 6.11 * math.Exp(5417.7530*(1/273.16-1/dewPointK))
	return tempC + 0.5555*(vapourPressure-10)
}
