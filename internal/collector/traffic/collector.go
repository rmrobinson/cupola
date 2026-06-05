package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

const (
	on511EventsURL   = "https://511on.ca/api/v2/get/event?format=json&lang=en"
	on511CamerasURL  = "https://511on.ca/api/v2/get/Cameras?format=json&lang=en"
	on511RoadCondURL = "https://511on.ca/api/v3/get/RoadConditions?format=json&lang=en"
	on511PublicURL   = "https://511on.ca"

	defaultIncidentInterval = 15 * time.Minute
	defaultCameraInterval   = 24 * time.Hour
)

var client = &http.Client{Timeout: 30 * time.Second}

type SourceSpec struct {
	ID                string
	Type              string
	Province          string
	URL               string
	IncidentsURL      string
	CamerasURL        string
	RoadConditionsURL string
	PublicURL         string
}

type Sources struct {
	Incidents      []IncidentSource
	Cameras        []CameraSource
	RoadConditions []RoadConditionSource
}

func NewSources(specs []SourceSpec) (Sources, []string) {
	var sources Sources
	var skipped []string
	for _, spec := range specs {
		sourceType := strings.ToLower(strings.TrimSpace(spec.Type))
		switch sourceType {
		case "511", "on511", "511on":
			api511, warnings := newAPI511Source(spec)
			skipped = append(skipped, warnings...)
			if api511 == nil {
				continue
			}
			if api511.eventsURL != "" {
				sources.Incidents = append(sources.Incidents, api511)
			}
			if api511.camerasURL != "" {
				sources.Cameras = append(sources.Cameras, api511)
			}
			if api511.roadConditionsURL != "" {
				sources.RoadConditions = append(sources.RoadConditions, api511)
			}
		case "region-waterloo-roadclosures", "region.waterloo.roadclosures":
			if spec.URL != "" {
				sources.Incidents = append(sources.Incidents, NewRegionWaterlooClosuresSourceWithURL(spec.URL))
			} else {
				sources.Incidents = append(sources.Incidents, NewRegionWaterlooClosuresSource())
			}
		default:
			skipped = append(skipped, fmt.Sprintf("%s: unknown traffic source type %q", sourceID(spec), spec.Type))
		}
	}
	return sources, skipped
}

func newAPI511Source(spec SourceSpec) (*API511Source, []string) {
	id := strings.TrimSpace(spec.ID)
	if id == "" {
		id = "511"
	}
	province := strings.ToUpper(strings.TrimSpace(spec.Province))
	eventsURL := strings.TrimSpace(spec.IncidentsURL)
	camerasURL := strings.TrimSpace(spec.CamerasURL)
	roadConditionsURL := strings.TrimSpace(spec.RoadConditionsURL)
	publicURL := strings.TrimSpace(spec.PublicURL)
	if province == "ON" {
		if eventsURL == "" {
			eventsURL = on511EventsURL
		}
		if camerasURL == "" {
			camerasURL = on511CamerasURL
		}
		if roadConditionsURL == "" {
			roadConditionsURL = on511RoadCondURL
		}
		if publicURL == "" {
			publicURL = on511PublicURL
		}
	}
	var warnings []string
	if eventsURL == "" && camerasURL == "" && roadConditionsURL == "" {
		warnings = append(warnings, fmt.Sprintf("%s: 511 source has no configured API URLs", id))
		return nil, warnings
	}
	return &API511Source{
		id:                id,
		eventsURL:         eventsURL,
		camerasURL:        camerasURL,
		roadConditionsURL: roadConditionsURL,
		publicURL:         publicURL,
	}, warnings
}

func sourceID(spec SourceSpec) string {
	if strings.TrimSpace(spec.ID) != "" {
		return spec.ID
	}
	if strings.TrimSpace(spec.Type) != "" {
		return spec.Type
	}
	return "traffic source"
}

func NewCamerasCollector(interval time.Duration, stateStore *store.StateStore, sources ...CameraSource) *CamerasCollector {
	if interval == 0 {
		interval = defaultCameraInterval
	}
	return &CamerasCollector{interval: interval, stateStore: stateStore, sources: sources, wake: make(chan struct{}, 1)}
}

func NewRoadConditionsCollector(interval time.Duration, stateStore *store.StateStore, sources ...RoadConditionSource) *RoadConditionsCollector {
	if interval == 0 {
		interval = defaultIncidentInterval
	}
	return &RoadConditionsCollector{interval: interval, stateStore: stateStore, sources: sources, wake: make(chan struct{}, 1)}
}

// ── Incidents ─────────────────────────────────────────────────────────────────

type IncidentSource interface {
	ID() string
	Fetch(ctx context.Context) ([]domain.TrafficIncident, error)
}

type CameraSource interface {
	ID() string
	FetchCameras(ctx context.Context) ([]domain.TrafficCamera, error)
}

type RoadConditionSource interface {
	ID() string
	FetchRoadConditions(ctx context.Context) ([]domain.TrafficRoadCondition, error)
}

type promotedAlertSource interface {
	PromotedMunicipalAlerts() []domain.MunicipalAlert
}

type MunicipalAlertSink interface {
	SetSourceAlerts(sourceID string, alerts []domain.MunicipalAlert)
}

type IncidentsCollector struct {
	interval   time.Duration
	stateStore *store.StateStore
	sources    []IncidentSource
	alertSink  MunicipalAlertSink
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.TrafficIncidents
	wake       chan struct{}
}

func NewIncidentsCollector(interval time.Duration, stateStore *store.StateStore, sources ...IncidentSource) *IncidentsCollector {
	if interval == 0 {
		interval = defaultIncidentInterval
	}
	return &IncidentsCollector{interval: interval, stateStore: stateStore, sources: sources, wake: make(chan struct{}, 1)}
}

func (c *IncidentsCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *IncidentsCollector) SetMunicipalAlertSink(sink MunicipalAlertSink) {
	c.alertSink = sink
}

func (c *IncidentsCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *IncidentsCollector) ID() string                { return "traffic.incidents" }
func (c *IncidentsCollector) Domain() domain.DomainType { return domain.DomainTrafficIncidents }

func (c *IncidentsCollector) Start(ctx context.Context) error {
	go func() {
		if c.netCheck == nil || c.netCheck() {
			if err := c.fetch(ctx); err != nil {
				log.Printf("[traffic.incidents] initial fetch: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
			}
		}
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.fetchIfReady(ctx)
			case <-c.wake:
				c.fetchIfReady(ctx)
			}
		}
	}()
	return nil
}

func (c *IncidentsCollector) fetchIfReady(ctx context.Context) {
	if c.netCheck != nil && !c.netCheck() {
		return
	}
	if err := c.fetch(ctx); err != nil {
		log.Printf("[traffic.incidents] fetch: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: c.ID(), Status: "error", Message: err.Error(),
		})
	} else {
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	}
}

func (c *IncidentsCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *IncidentsCollector) fetch(ctx context.Context) error {
	var incidents []domain.TrafficIncident
	var failed []string
	for _, source := range c.sources {
		items, err := source.Fetch(ctx)
		if err != nil {
			log.Printf("[%s] fetch: %v", source.ID(), err)
			failed = append(failed, fmt.Sprintf("%s: %v", source.ID(), err))
			if c.alertSink != nil {
				if _, ok := source.(promotedAlertSource); ok {
					c.alertSink.SetSourceAlerts(source.ID(), nil)
				}
			}
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: source.ID(), Status: "error", Message: err.Error(),
			})
			continue
		}
		incidents = append(incidents, items...)
		if c.alertSink != nil {
			if alertSource, ok := source.(promotedAlertSource); ok {
				c.alertSink.SetSourceAlerts(source.ID(), alertSource.PromotedMunicipalAlerts())
			}
		}
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: source.ID(), Status: "ok"})
		log.Printf("[%s] fetched %d incidents", source.ID(), len(items))
	}

	var duplicateCount int
	incidents, duplicateCount = dedupeTrafficIncidents(incidents)
	if duplicateCount > 0 {
		log.Printf("[traffic.incidents] dropped %d duplicate incident(s)", duplicateCount)
	}

	state := domain.TrafficIncidents{
		StateBase: domain.StateBase{UpdatedAt: time.Now().UTC()},
		Incidents: incidents,
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[traffic.incidents] published %d incidents from %d source(s)", len(incidents), len(c.sources))

	if len(failed) == len(c.sources) && len(c.sources) > 0 {
		return fmt.Errorf("all traffic incident sources failed: %s", strings.Join(failed, "; "))
	}
	return nil
}

func dedupeTrafficIncidents(incidents []domain.TrafficIncident) ([]domain.TrafficIncident, int) {
	if len(incidents) < 2 {
		return incidents, 0
	}
	seen := make(map[string]struct{}, len(incidents))
	deduped := make([]domain.TrafficIncident, 0, len(incidents))
	var duplicates int
	for _, inc := range incidents {
		if inc.ID == "" {
			deduped = append(deduped, inc)
			continue
		}
		if _, ok := seen[inc.ID]; ok {
			duplicates++
			continue
		}
		seen[inc.ID] = struct{}{}
		deduped = append(deduped, inc)
	}
	if duplicates == 0 {
		return incidents, 0
	}
	return deduped, duplicates
}

type API511Source struct {
	id                string
	eventsURL         string
	camerasURL        string
	roadConditionsURL string
	publicURL         string
}

func (s *API511Source) ID() string { return s.id }

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

func (s *API511Source) Fetch(ctx context.Context) ([]domain.TrafficIncident, error) {
	if s.eventsURL == "" {
		return nil, fmt.Errorf("%s incidents URL is not configured", s.ID())
	}
	body, err := getJSON(ctx, s.eventsURL)
	if err != nil {
		return nil, err
	}
	var raw []on511Event
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal events: %w", err)
	}

	incidents := make([]domain.TrafficIncident, 0, len(raw))
	for _, e := range raw {
		inc := domain.TrafficIncident{
			ID:          fmt.Sprintf("%s:%d", s.ID(), e.ID),
			Type:        mapEventType(e.EventType),
			Severity:    mapSeverity(e.EventType, e.IsFullClosure),
			Lat:         e.Latitude,
			Lon:         e.Longitude,
			Description: e.Description,
			RoadName:    e.RoadwayName,
			SourceURL:   s.publicURL,
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
	return incidents, nil
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
	sources    []CameraSource
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.TrafficCameras
	wake       chan struct{}
}

func (c *CamerasCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *CamerasCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *CamerasCollector) ID() string                { return "traffic.cameras" }
func (c *CamerasCollector) Domain() domain.DomainType { return domain.DomainTrafficCameras }

func (c *CamerasCollector) Start(ctx context.Context) error {
	go func() {
		if c.netCheck == nil || c.netCheck() {
			if err := c.fetch(ctx); err != nil {
				log.Printf("[traffic.cameras] initial fetch: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
			}
		}
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.fetchIfReady(ctx)
			case <-c.wake:
				c.fetchIfReady(ctx)
			}
		}
	}()
	return nil
}

func (c *CamerasCollector) fetchIfReady(ctx context.Context) {
	if c.netCheck != nil && !c.netCheck() {
		return
	}
	if err := c.fetch(ctx); err != nil {
		log.Printf("[traffic.cameras] fetch: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: c.ID(), Status: "error", Message: err.Error(),
		})
	} else {
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	}
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

func (s *API511Source) FetchCameras(ctx context.Context) ([]domain.TrafficCamera, error) {
	if s.camerasURL == "" {
		return nil, fmt.Errorf("%s cameras URL is not configured", s.ID())
	}
	body, err := getJSON(ctx, s.camerasURL)
	if err != nil {
		return nil, err
	}
	var raw []on511Camera
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal cameras: %w", err)
	}

	cameras := make([]domain.TrafficCamera, 0, len(raw))
	for _, cam := range raw {
		snapshotURL := firstEnabledViewURL(cam.Views)
		if snapshotURL == "" {
			continue
		}
		cameras = append(cameras, domain.TrafficCamera{
			ID:          fmt.Sprintf("%s:%d", s.ID(), cam.ID),
			Name:        cam.Location,
			Lat:         cam.Latitude,
			Lon:         cam.Longitude,
			SnapshotURL: snapshotURL,
			UpdatedAt:   time.Now().UTC(),
		})
	}
	return cameras, nil
}

func (c *CamerasCollector) fetch(ctx context.Context) error {
	var cameras []domain.TrafficCamera
	var failed []string
	for _, source := range c.sources {
		items, err := source.FetchCameras(ctx)
		if err != nil {
			log.Printf("[%s.cameras] fetch: %v", source.ID(), err)
			failed = append(failed, fmt.Sprintf("%s: %v", source.ID(), err))
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: source.ID() + ".cameras", Status: "error", Message: err.Error(),
			})
			continue
		}
		cameras = append(cameras, items...)
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: source.ID() + ".cameras", Status: "ok"})
		log.Printf("[%s.cameras] fetched %d cameras", source.ID(), len(items))
	}

	state := domain.TrafficCameras{
		StateBase: domain.StateBase{UpdatedAt: time.Now().UTC()},
		Cameras:   cameras,
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[traffic.cameras] published %d cameras from %d source(s)", len(cameras), len(c.sources))
	if len(failed) == len(c.sources) && len(c.sources) > 0 {
		return fmt.Errorf("all traffic camera sources failed: %s", strings.Join(failed, "; "))
	}
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
	sources    []RoadConditionSource
	netCheck   func() bool
	mu         sync.RWMutex
	state      domain.TrafficRoadConditions
	wake       chan struct{}
}

func (c *RoadConditionsCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *RoadConditionsCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *RoadConditionsCollector) ID() string { return "traffic.road_conditions" }
func (c *RoadConditionsCollector) Domain() domain.DomainType {
	return domain.DomainTrafficRoadConditions
}

func (c *RoadConditionsCollector) Start(ctx context.Context) error {
	go func() {
		if c.netCheck == nil || c.netCheck() {
			if err := c.fetch(ctx); err != nil {
				log.Printf("[traffic.road_conditions] initial fetch: %v", err)
				c.stateStore.PublishSystem(store.SystemEvent{
					CollectorID: c.ID(), Status: "error", Message: err.Error(),
				})
			} else {
				c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
			}
		}
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.fetchIfReady(ctx)
			case <-c.wake:
				c.fetchIfReady(ctx)
			}
		}
	}()
	return nil
}

func (c *RoadConditionsCollector) fetchIfReady(ctx context.Context) {
	if c.netCheck != nil && !c.netCheck() {
		return
	}
	if err := c.fetch(ctx); err != nil {
		log.Printf("[traffic.road_conditions] fetch: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{
			CollectorID: c.ID(), Status: "error", Message: err.Error(),
		})
	} else {
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	}
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

func (s *API511Source) FetchRoadConditions(ctx context.Context) ([]domain.TrafficRoadCondition, error) {
	if s.roadConditionsURL == "" {
		return nil, fmt.Errorf("%s road conditions URL is not configured", s.ID())
	}
	body, err := getJSON(ctx, s.roadConditionsURL)
	if err != nil {
		return nil, err
	}
	var raw []on511RoadCondition
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal road conditions: %w", err)
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
	return conditions, nil
}

func (c *RoadConditionsCollector) fetch(ctx context.Context) error {
	var conditions []domain.TrafficRoadCondition
	var failed []string
	for _, source := range c.sources {
		items, err := source.FetchRoadConditions(ctx)
		if err != nil {
			log.Printf("[%s.road_conditions] fetch: %v", source.ID(), err)
			failed = append(failed, fmt.Sprintf("%s: %v", source.ID(), err))
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: source.ID() + ".road_conditions", Status: "error", Message: err.Error(),
			})
			continue
		}
		conditions = append(conditions, items...)
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: source.ID() + ".road_conditions", Status: "ok"})
		log.Printf("[%s.road_conditions] fetched %d road condition segments", source.ID(), len(items))
	}

	state := domain.TrafficRoadConditions{
		StateBase:  domain.StateBase{UpdatedAt: time.Now().UTC()},
		Conditions: conditions,
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[traffic.road_conditions] published %d road condition segments from %d source(s)", len(conditions), len(c.sources))
	if len(failed) == len(c.sources) && len(c.sources) > 0 {
		return fmt.Errorf("all traffic road condition sources failed: %s", strings.Join(failed, "; "))
	}
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
	defer func() { _ = resp.Body.Close() }()
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
