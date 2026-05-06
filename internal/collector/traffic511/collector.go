package traffic511

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

const (
	eventsURL   = "https://511on.ca/api/v2/get/event?format=json&lang=en"
	camerasURL  = "https://511on.ca/api/v2/get/Cameras?format=json&lang=en"
	roadCondURL = "https://511on.ca/api/v3/get/RoadConditions?format=json&lang=en"

	defaultIncidentInterval = 15 * time.Minute
	defaultCameraInterval   = 24 * time.Hour
)

var client = &http.Client{Timeout: 30 * time.Second}

// NewCollectors returns incidents, cameras, and road-conditions collectors for ON511.
// incidentInterval applies to both incidents and road conditions; cameraInterval applies to cameras.
func NewCollectors(incidentInterval, cameraInterval time.Duration, stateStore *store.StateStore) (*IncidentsCollector, *CamerasCollector, *RoadConditionsCollector) {
	if incidentInterval == 0 {
		incidentInterval = defaultIncidentInterval
	}
	if cameraInterval == 0 {
		cameraInterval = defaultCameraInterval
	}
	return &IncidentsCollector{interval: incidentInterval, stateStore: stateStore},
		&CamerasCollector{interval: cameraInterval, stateStore: stateStore},
		&RoadConditionsCollector{interval: incidentInterval, stateStore: stateStore}
}

// ── Incidents ─────────────────────────────────────────────────────────────────

type IncidentsCollector struct {
	interval   time.Duration
	stateStore *store.StateStore
	mu         sync.RWMutex
	state      domain.TrafficIncidents
}

func (c *IncidentsCollector) ID() string                { return "511on.incidents" }
func (c *IncidentsCollector) Domain() domain.DomainType { return domain.DomainTrafficIncidents }

func (c *IncidentsCollector) Start(ctx context.Context) error {
	go func() {
		if err := c.fetch(ctx); err != nil {
			log.Printf("[511on.incidents] initial fetch: %v", err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
		} else {
			c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
		}
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := c.fetch(ctx); err != nil {
					log.Printf("[511on.incidents] fetch: %v", err)
					c.stateStore.PublishSystem(store.SystemEvent{
						CollectorID: c.ID(), Status: "error", Message: err.Error(),
					})
				} else {
					c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
				}
			}
		}
	}()
	return nil
}

func (c *IncidentsCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// on511Event is the raw JSON shape returned by the Events endpoint.
type on511Event struct {
	ID                int     `json:"ID"`
	RoadwayName       string  `json:"RoadwayName"`
	DirectionOfTravel string  `json:"DirectionOfTravel"`
	Description       string  `json:"Description"`
	StartDate         int64   `json:"StartDate"`
	PlannedEndDate    int64   `json:"PlannedEndDate"`
	Latitude          float64 `json:"Latitude"`
	Longitude         float64 `json:"Longitude"`
	EventType         string  `json:"EventType"`
	IsFullClosure     bool    `json:"IsFullClosure"`
	Severity          string  `json:"Severity"`
}

func (c *IncidentsCollector) fetch(ctx context.Context) error {
	body, err := getJSON(ctx, eventsURL)
	if err != nil {
		return err
	}
	var raw []on511Event
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("unmarshal events: %w", err)
	}

	incidents := make([]domain.TrafficIncident, 0, len(raw))
	for _, e := range raw {
		inc := domain.TrafficIncident{
			ID:          fmt.Sprintf("%d", e.ID),
			Type:        mapEventType(e.EventType),
			Severity:    mapSeverity(e.EventType, e.IsFullClosure),
			Lat:         e.Latitude,
			Lon:         e.Longitude,
			Description: e.Description,
			RoadName:    e.RoadwayName,
		}
		if e.StartDate > 0 {
			t := time.Unix(e.StartDate, 0)
			inc.StartsAt = &t
		}
		if e.PlannedEndDate > 0 {
			t := time.Unix(e.PlannedEndDate, 0)
			inc.EndsAt = &t
		}
		incidents = append(incidents, inc)
	}

	state := domain.TrafficIncidents{
		StateBase: domain.StateBase{UpdatedAt: time.Now().UTC()},
		Incidents: incidents,
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[511on.incidents] fetched %d incidents", len(incidents))
	return nil
}

func mapEventType(et string) string {
	switch et {
	case "roadwork":
		return "construction"
	case "closures":
		return "closure"
	case "accidentsAndIncidents":
		return "collision"
	default:
		return "hazard"
	}
}

func mapSeverity(et string, fullClosure bool) string {
	if et == "closures" || fullClosure {
		return "major"
	}
	if et == "roadwork" {
		return "minor"
	}
	return "moderate"
}

// ── Cameras ───────────────────────────────────────────────────────────────────

type CamerasCollector struct {
	interval   time.Duration
	stateStore *store.StateStore
	mu         sync.RWMutex
	state      domain.TrafficCameras
}

func (c *CamerasCollector) ID() string                { return "511on.cameras" }
func (c *CamerasCollector) Domain() domain.DomainType { return domain.DomainTrafficCameras }

func (c *CamerasCollector) Start(ctx context.Context) error {
	go func() {
		if err := c.fetch(ctx); err != nil {
			log.Printf("[511on.cameras] initial fetch: %v", err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
		} else {
			c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
		}
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := c.fetch(ctx); err != nil {
					log.Printf("[511on.cameras] fetch: %v", err)
					c.stateStore.PublishSystem(store.SystemEvent{
						CollectorID: c.ID(), Status: "error", Message: err.Error(),
					})
				} else {
					c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
				}
			}
		}
	}()
	return nil
}

func (c *CamerasCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

type on511CameraView struct {
	ID          int    `json:"Id"`
	URL         string `json:"Url"`
	Status      string `json:"Status"`
	Description string `json:"Description"`
}

type on511Camera struct {
	ID        int               `json:"Id"`
	Roadway   string            `json:"Roadway"`
	Location  string            `json:"Location"`
	Latitude  float64           `json:"Latitude"`
	Longitude float64           `json:"Longitude"`
	Views     []on511CameraView `json:"Views"`
}

func (c *CamerasCollector) fetch(ctx context.Context) error {
	body, err := getJSON(ctx, camerasURL)
	if err != nil {
		return err
	}
	var raw []on511Camera
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("unmarshal cameras: %w", err)
	}

	cameras := make([]domain.TrafficCamera, 0, len(raw))
	for _, cam := range raw {
		snapshotURL := firstEnabledViewURL(cam.Views)
		if snapshotURL == "" {
			continue
		}
		cameras = append(cameras, domain.TrafficCamera{
			ID:          fmt.Sprintf("%d", cam.ID),
			Name:        cam.Location,
			Lat:         cam.Latitude,
			Lon:         cam.Longitude,
			SnapshotURL: snapshotURL,
			UpdatedAt:   time.Now().UTC(),
		})
	}

	state := domain.TrafficCameras{
		StateBase: domain.StateBase{UpdatedAt: time.Now().UTC()},
		Cameras:   cameras,
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[511on.cameras] fetched %d cameras", len(cameras))
	return nil
}

func firstEnabledViewURL(views []on511CameraView) string {
	for _, v := range views {
		if v.Status == "Enabled" && v.URL != "" {
			return v.URL
		}
	}
	return ""
}

// ── Road Conditions ───────────────────────────────────────────────────────────

type RoadConditionsCollector struct {
	interval   time.Duration
	stateStore *store.StateStore
	mu         sync.RWMutex
	state      domain.TrafficRoadConditions
}

func (c *RoadConditionsCollector) ID() string                { return "511on.road_conditions" }
func (c *RoadConditionsCollector) Domain() domain.DomainType { return domain.DomainTrafficRoadConditions }

func (c *RoadConditionsCollector) Start(ctx context.Context) error {
	go func() {
		if err := c.fetch(ctx); err != nil {
			log.Printf("[511on.road_conditions] initial fetch: %v", err)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
		} else {
			c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
		}
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := c.fetch(ctx); err != nil {
					log.Printf("[511on.road_conditions] fetch: %v", err)
					c.stateStore.PublishSystem(store.SystemEvent{
						CollectorID: c.ID(), Status: "error", Message: err.Error(),
					})
				} else {
					c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
				}
			}
		}
	}()
	return nil
}

func (c *RoadConditionsCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

type on511RoadCondition struct {
	LocationDescription string   `json:"LocationDescription"`
	Condition           []string `json:"Condition"`
	Visibility          string   `json:"Visibility"`
	Drifting            string   `json:"Drifting"`
	Region              string   `json:"Region"`
	RoadwayName         string   `json:"RoadwayName"`
	LastUpdated         int64    `json:"LastUpdated"`
}

func (c *RoadConditionsCollector) fetch(ctx context.Context) error {
	body, err := getJSON(ctx, roadCondURL)
	if err != nil {
		return err
	}
	var raw []on511RoadCondition
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("unmarshal road conditions: %w", err)
	}

	conditions := make([]domain.TrafficRoadCondition, 0, len(raw))
	for _, r := range raw {
		cond := domain.TrafficRoadCondition{
			LocationDescription: r.LocationDescription,
			Conditions:          r.Condition,
			Visibility:          r.Visibility,
			Drifting:            r.Drifting,
			Region:              r.Region,
			RoadwayName:         r.RoadwayName,
		}
		if r.LastUpdated > 0 {
			cond.LastUpdated = time.Unix(r.LastUpdated, 0)
		}
		conditions = append(conditions, cond)
	}

	state := domain.TrafficRoadConditions{
		StateBase:  domain.StateBase{UpdatedAt: time.Now().UTC()},
		Conditions: conditions,
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[511on.road_conditions] fetched %d road condition segments", len(conditions))
	return nil
}

// ── Shared HTTP helper ────────────────────────────────────────────────────────

func getJSON(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 512)) //nolint:errcheck
		return nil, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body from %s: %w", url, err)
	}
	return body, nil
}
