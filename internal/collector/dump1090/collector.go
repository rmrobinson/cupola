// Package dump1090 polls a local dump1090 or readsb HTTP API for ADS-B aircraft positions.
// Supports both dump1090-mutability (altitude/speed fields) and dump1090-fa/readsb
// (alt_baro/gs fields) JSON formats.
package dump1090

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// Collector polls a dump1090/readsb HTTP API and publishes aircraft domain state.
type Collector struct {
	url        string
	interval   time.Duration
	radiusKM   float64 // 0 = no geographic filter
	siteLat    float64
	siteLon    float64
	stateStore *store.StateStore
}

// New creates a Collector. baseURL is the root of the dump1090/readsb HTTP server
// (e.g. "http://192.168.1.10:8080"). siteLat/siteLon and radiusKM are used to
// filter aircraft to within a bounding circle; pass radiusKM=0 to disable filtering.
func New(baseURL string, interval time.Duration, siteLat, siteLon, radiusKM float64, stateStore *store.StateStore) *Collector {
	return &Collector{
		url:        strings.TrimRight(baseURL, "/") + "/data/aircraft.json",
		interval:   interval,
		radiusKM:   radiusKM,
		siteLat:    siteLat,
		siteLon:    siteLon,
		stateStore: stateStore,
	}
}

func (c *Collector) ID() string                { return "dump1090" }
func (c *Collector) Domain() domain.DomainType { return domain.DomainAircraft }
func (c *Collector) State() domain.DomainState { return c.stateStore.Get(c.Domain()) }

func (c *Collector) Start(ctx context.Context) error {
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
			}
		}
	}()
	return nil
}

// ── JSON types ────────────────────────────────────────────────────────────────

type aircraftResponse struct {
	Aircraft []aircraftJSON `json:"aircraft"`
}

// aircraftJSON handles both dump1090-fa/readsb and dump1090-mutability formats.
type aircraftJSON struct {
	Hex      string  `json:"hex"`
	Flight   string  `json:"flight"`
	Squawk   string  `json:"squawk"`
	Category string  `json:"category"` // ICAO ADS-B emitter category, e.g. "A3"
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Track    float64 `json:"track"`
	VertRate *int    `json:"vert_rate"` // mutability
	BaroRate *int    `json:"baro_rate"` // fa/readsb
	Seen     float64 `json:"seen"`
	SeenPos  float64 `json:"seen_pos"`

	// Ground speed: dump1090-fa uses "gs", mutability uses "speed".
	GS    float64 `json:"gs"`
	Speed float64 `json:"speed"`

	// Altitude: dump1090-fa uses "alt_baro" (int or "ground"), mutability uses "altitude" (int).
	AltBaro  json.RawMessage `json:"alt_baro"`
	Altitude *int            `json:"altitude"`
}

// altFt extracts barometric altitude in feet from the two possible field formats.
// Returns (0, true) when the aircraft is on the ground.
func (a *aircraftJSON) altFt() (int, bool) {
	if a.Altitude != nil {
		return *a.Altitude, false
	}
	if len(a.AltBaro) > 0 {
		if string(a.AltBaro) == `"ground"` {
			return 0, true
		}
		var alt int
		if err := json.Unmarshal(a.AltBaro, &alt); err == nil {
			return alt, false
		}
	}
	return 0, false
}

// ── Fetch ─────────────────────────────────────────────────────────────────────

func (c *Collector) fetch() {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(c.url)
	if err != nil {
		log.Printf("[dump1090] get %s: %v", c.url, err)
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "error", Message: err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		log.Printf("[dump1090] %s", msg)
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "error", Message: msg})
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[dump1090] read: %v", err)
		return
	}

	var data aircraftResponse
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("[dump1090] parse: %v", err)
		return
	}

	var targets []domain.AircraftTarget
	for _, a := range data.Aircraft {
		if a.Lat == 0 && a.Lon == 0 {
			continue // no position
		}
		if a.SeenPos > 60 {
			continue // stale position
		}
		if c.radiusKM > 0 && haversineKM(c.siteLat, c.siteLon, a.Lat, a.Lon) > c.radiusKM {
			continue // outside configured radius
		}

		alt, onGround := a.altFt()
		t := domain.AircraftTarget{
			ICAO:      strings.ToUpper(strings.TrimSpace(a.Hex)),
			Category:  a.Category,
			Lat:       a.Lat,
			Lon:       a.Lon,
			AltFt:     alt,
			OnGround:  onGround,
			UpdatedAt: time.Now(),
		}

		if f := strings.TrimSpace(a.Flight); f != "" {
			t.Flight = &f
		}
		if s := strings.TrimSpace(a.Squawk); s != "" {
			t.Squawk = &s
		}
		if a.Track != 0 {
			v := a.Track
			t.Track = &v
		}
		// Ground speed: prefer gs (fa/readsb), fall back to speed (mutability).
		if gs := firstNonZero(a.GS, a.Speed); gs > 0 {
			t.SpeedKts = &gs
		}
		// Vertical rate: prefer vert_rate (mutability), fall back to baro_rate (fa/readsb).
		if a.VertRate != nil {
			t.VertRate = a.VertRate
		} else if a.BaroRate != nil {
			t.VertRate = a.BaroRate
		}

		targets = append(targets, t)
	}

	c.stateStore.Set(domain.Aircraft{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Aircraft:  targets,
	})
	c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	log.Printf("[dump1090] %d aircraft in range", len(targets))
}

func firstNonZero(a, b float64) float64 {
	if a != 0 {
		return a
	}
	return b
}

// haversineKM returns the great-circle distance in km between two lat/lon points.
func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
