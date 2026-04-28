package astro

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// Collector computes astronomical data locally (no network required).
// It recalculates at startup and at the start of each new local day.
type Collector struct {
	lat, lon float64
	tz       *time.Location

	stateStore *store.StateStore
	mu         sync.RWMutex
	state      domain.Astro
}

func New(lat, lon float64, timezone string, stateStore *store.StateStore) *Collector {
	loc, err := time.LoadLocation(timezone)
	if err != nil || loc == nil {
		log.Printf("[ephem] unknown timezone %q, using UTC", timezone)
		loc = time.UTC
	}
	return &Collector{lat: lat, lon: lon, tz: loc, stateStore: stateStore}
}

func (c *Collector) ID() string                { return "ephem" }
func (c *Collector) Domain() domain.DomainType { return domain.DomainAstro }

func (c *Collector) Start(ctx context.Context) error {
	c.compute(time.Now())
	go c.loop(ctx)
	return nil
}

func (c *Collector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// loop waits until the next local midnight + 1 minute, then recomputes.
func (c *Collector) loop(ctx context.Context) {
	for {
		now := time.Now().In(c.tz)
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 1, 0, 0, c.tz)
		select {
		case <-ctx.Done():
			return
		case t := <-time.After(time.Until(next)):
			c.compute(t)
		}
	}
}

func (c *Collector) compute(now time.Time) {
	date := now.UTC()

	// Solar times (elevation -0.833° accounts for refraction + solar disc radius)
	rise, riseOK := sunTime(date, c.lat, c.lon, -0.833, true)
	set, setOK := sunTime(date, c.lat, c.lon, -0.833, false)
	noon := solarNoon(date, c.lon)
	dawn, dawnOK := sunTime(date, c.lat, c.lon, -6.0, true)
	dusk, duskOK := sunTime(date, c.lat, c.lon, -6.0, false)

	// Use noon as a sensible fallback for polar conditions
	if !riseOK {
		rise = noon
	}
	if !setOK {
		set = noon
	}
	if !dawnOK {
		dawn = noon
	}
	if !duskOK {
		dusk = noon
	}

	phase := moonPhase(toJD(date))

	a := domain.Astro{
		StateBase:        domain.StateBase{UpdatedAt: now},
		Sunrise:          rise,
		Sunset:           set,
		SolarNoon:        noon,
		CivilDawn:        dawn,
		CivilDusk:        dusk,
		MoonPhase:        phase,
		MoonPhaseName:    moonPhaseName(phase),
		MoonIllumination: moonIllumination(phase),
		// MoonRise / MoonSet require iterative ephemeris; deferred to a later phase.
	}

	c.mu.Lock()
	c.state = a
	c.mu.Unlock()

	c.stateStore.Set(a)
	log.Printf("[ephem] computed for %s: sunrise=%s sunset=%s moon=%.2f (%s)",
		now.In(c.tz).Format("2006-01-02"),
		rise.In(c.tz).Format("15:04"),
		set.In(c.tz).Format("15:04"),
		phase, moonPhaseName(phase),
	)
}
