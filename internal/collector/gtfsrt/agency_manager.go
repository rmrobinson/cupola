package gtfsrt

import (
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/rmrobinson/cupola/internal/collector/gtfs"
	"github.com/rmrobinson/cupola/internal/store"
)

type AgencySource interface {
	List() []*Agency
	Get(id string) *Agency
	RefreshStatic(id string) error
}

type AgencyManager struct {
	mu             sync.RWMutex
	agencies       map[string]*Agency
	db             *store.SQLiteStore
	cacheDir       string
	refreshLocksMu sync.Mutex
	refreshLocks   map[string]*sync.Mutex
}

type AgencyStats struct {
	ID                   string             `json:"id"`
	Enabled              bool               `json:"enabled"`
	GTFSStaticURLs       int                `json:"gtfs_static_urls"`
	GTFSRTTripUpdates    int                `json:"gtfs_rt_trip_updates_urls"`
	GTFSRTVehicleUpdates int                `json:"gtfs_rt_vehicle_positions_urls"`
	GTFSRTAlerts         int                `json:"gtfs_rt_alerts_urls"`
	Schedule             gtfs.ScheduleStats `json:"schedule"`
}

func NewAgencyManager(db *store.SQLiteStore, cacheDir string) (*AgencyManager, error) {
	m := &AgencyManager{
		agencies:     make(map[string]*Agency),
		db:           db,
		cacheDir:     cacheDir,
		refreshLocks: make(map[string]*sync.Mutex),
	}
	if err := m.ReloadAll(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *AgencyManager) ReloadAll() error {
	cfgs, err := m.db.ListTransitAgencies()
	if err != nil {
		return err
	}
	next := make(map[string]*Agency)
	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		next[cfg.ID] = agencyFromConfig(cfg)
	}
	m.mu.Lock()
	m.agencies = next
	m.mu.Unlock()
	return nil
}

func (m *AgencyManager) Reload(id string) error {
	cfg, err := m.db.GetTransitAgency(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg == nil || !cfg.Enabled {
		delete(m.agencies, id)
		return nil
	}
	existing := m.agencies[id]
	next := agencyFromConfig(*cfg)
	if existing != nil {
		next.Schedule = existing.Schedule
	}
	m.agencies[id] = next
	return nil
}

func (m *AgencyManager) List() []*Agency {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Agency, 0, len(m.agencies))
	for _, ag := range m.agencies {
		out = append(out, ag)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *AgencyManager) Get(id string) *Agency {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.agencies[id]
}

func (m *AgencyManager) Stats() []AgencyStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]AgencyStats, 0, len(m.agencies))
	for _, ag := range m.agencies {
		alerts := 0
		if ag.AlertsURL != "" {
			alerts = 1
		}
		out = append(out, AgencyStats{
			ID:                   ag.ID,
			Enabled:              true,
			GTFSStaticURLs:       len(ag.StaticURLs),
			GTFSRTTripUpdates:    len(ag.TripUpdatesURLs),
			GTFSRTVehicleUpdates: len(ag.VehiclePositionsURLs),
			GTFSRTAlerts:         alerts,
			Schedule:             ag.Schedule.Stats(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *AgencyManager) RefreshStatic(id string) error {
	return m.withRefreshLock(id, func() error {
		ag := m.Get(id)
		if ag == nil {
			return nil
		}
		return gtfs.LoadAndPersist(ag.Schedule, ag.ID, ag.StaticURLs, m.cacheDir, m.db)
	})
}

func (m *AgencyManager) RefreshStaticAsync(id string) {
	go func() {
		if err := m.RefreshStatic(id); err != nil {
			log.Printf("[gtfsrt] %s: static refresh: %v", id, err)
		}
	}()
}

func (m *AgencyManager) Delete(id string) error {
	m.mu.Lock()
	old := m.agencies[id]
	delete(m.agencies, id)
	m.mu.Unlock()

	err := m.withRefreshLock(id, func() error {
		if err := m.db.DeleteTransitAgency(id); err != nil {
			return fmt.Errorf("delete transit agency row: %w", err)
		}
		if err := m.db.DeleteGTFSAgency(id); err != nil {
			return fmt.Errorf("delete gtfs timetable cache: %w", err)
		}
		if err := gtfs.DeleteZipCache(m.cacheDir, id); err != nil {
			return fmt.Errorf("delete gtfs zip cache: %w", err)
		}
		return nil
	})
	m.removeRefreshLock(id)
	if err != nil && old != nil {
		m.mu.Lock()
		m.agencies[id] = old
		m.mu.Unlock()
	}
	return err
}

func (m *AgencyManager) withRefreshLock(id string, fn func() error) error {
	m.refreshLocksMu.Lock()
	l := m.refreshLocks[id]
	if l == nil {
		l = &sync.Mutex{}
		m.refreshLocks[id] = l
	}
	m.refreshLocksMu.Unlock()

	l.Lock()
	defer l.Unlock()
	return fn()
}

func (m *AgencyManager) removeRefreshLock(id string) {
	m.refreshLocksMu.Lock()
	delete(m.refreshLocks, id)
	m.refreshLocksMu.Unlock()
}

func agencyFromConfig(cfg store.TransitAgencyConfig) *Agency {
	return &Agency{
		ID:                   cfg.ID,
		StaticURLs:           append([]string(nil), cfg.GTFSStaticURLs...),
		TripUpdatesURLs:      append([]string(nil), cfg.GTFSRTTripUpdatesURLs...),
		VehiclePositionsURLs: append([]string(nil), cfg.GTFSRTVehiclePositionsURLs...),
		AlertsURL:            cfg.GTFSRTAlertsURL,
		Schedule:             gtfs.New(),
	}
}
