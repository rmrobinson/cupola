package wastecollection

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

type extraCollectionEntry struct {
	Date      string `json:"date"`
	DayOfWeek string `json:"day_of_week"`
	Type      string `json:"type"`
}

type scheduleEntry struct {
	Date             string                 `json:"date"`
	DayOfWeek        string                 `json:"day_of_week"`
	Collections      []string               `json:"collections"`
	ExtraCollections []extraCollectionEntry `json:"extra_collections"`
}

type scheduleFile struct {
	Schedule []scheduleEntry `json:"schedule"`
}

// Collector reads a local JSON schedule file and publishes which waste types
// are collected in the current calendar week.
type Collector struct {
	dataPath   string
	weekStart  time.Weekday
	stateStore *store.StateStore

	mu       sync.RWMutex
	schedule []scheduleEntry
	state    domain.WasteCollection
	wake     chan struct{}
}

func New(dataPath string, weekStart time.Weekday, stateStore *store.StateStore) *Collector {
	return &Collector{
		dataPath:   dataPath,
		weekStart:  weekStart,
		stateStore: stateStore,
		wake:       make(chan struct{}, 1),
	}
}

func (c *Collector) ID() string                { return "waste.collection" }
func (c *Collector) Domain() domain.DomainType { return domain.DomainWasteCollection }

func (c *Collector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Collector) Start(ctx context.Context) error {
	if err := c.load(); err != nil {
		return err
	}
	c.publish()

	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.publish()
			case <-c.wake:
				c.publish()
			}
		}
	}()
	return nil
}

func (c *Collector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Collector) load() error {
	raw, err := os.ReadFile(c.dataPath)
	if err != nil {
		return err
	}
	var sf scheduleFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		return err
	}
	for _, e := range sf.Schedule {
		entryDate, err := time.ParseInLocation("2006-01-02", e.Date, time.Local)
		if err != nil {
			continue
		}
		ws := currentWeekStart(entryDate, c.weekStart)
		we := ws.AddDate(0, 0, 7)
		for _, ex := range e.ExtraCollections {
			exd, err := time.ParseInLocation("2006-01-02", ex.Date, time.Local)
			if err != nil {
				log.Printf("[waste.collection] entry %s: extra collection %q has invalid date %q", e.Date, ex.Type, ex.Date)
				continue
			}
			if exd.Before(ws) || !exd.Before(we) {
				log.Printf("[waste.collection] entry %s: extra collection %q date %s falls outside its week [%s, %s)",
					e.Date, ex.Type, ex.Date, ws.Format("2006-01-02"), we.Format("2006-01-02"))
			}
		}
	}
	c.mu.Lock()
	c.schedule = sf.Schedule
	c.mu.Unlock()
	log.Printf("[waste.collection] loaded %d entries from %s", len(sf.Schedule), c.dataPath)
	return nil
}

func (c *Collector) publish() {
	state := c.compute()
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
}

func (c *Collector) compute() domain.WasteCollection {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	ws := currentWeekStart(today, c.weekStart)
	we := ws.AddDate(0, 0, 7)

	c.mu.RLock()
	schedule := c.schedule
	c.mu.RUnlock()

	for _, e := range schedule {
		d, err := time.ParseInLocation("2006-01-02", e.Date, time.Local)
		if err != nil {
			continue
		}
		if !d.Before(ws) && d.Before(we) {
			var extras []domain.ExtraCollection
			for _, ex := range e.ExtraCollections {
				exd, err := time.ParseInLocation("2006-01-02", ex.Date, time.Local)
				if err != nil {
					continue
				}
				if exd.Before(ws) || !exd.Before(we) {
					continue
				}
				extras = append(extras, domain.ExtraCollection{
					Date:      ex.Date,
					DayOfWeek: ex.DayOfWeek,
					Type:      ex.Type,
					IsToday:   exd.Equal(today),
				})
			}
			return domain.WasteCollection{
				StateBase:        domain.StateBase{UpdatedAt: now.UTC()},
				Date:             e.Date,
				DayOfWeek:        e.DayOfWeek,
				Collections:      e.Collections,
				IsToday:          d.Equal(today),
				ExtraCollections: extras,
			}
		}
	}
	return domain.WasteCollection{
		StateBase: domain.StateBase{UpdatedAt: now.UTC()},
	}
}

// currentWeekStart returns midnight of the most recent occurrence of startDay
// on or before t.
func currentWeekStart(t time.Time, startDay time.Weekday) time.Time {
	days := int(t.Weekday()) - int(startDay)
	if days < 0 {
		days += 7
	}
	start := t.AddDate(0, 0, -days)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
}

// ParseWeekday converts a weekday name string to time.Weekday, defaulting to
// time.Sunday for unrecognised values.
func ParseWeekday(s string) time.Weekday {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "monday":
		return time.Monday
	case "tuesday":
		return time.Tuesday
	case "wednesday":
		return time.Wednesday
	case "thursday":
		return time.Thursday
	case "friday":
		return time.Friday
	case "saturday":
		return time.Saturday
	default:
		return time.Sunday
	}
}
