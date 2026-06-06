package googlepollen

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
	"google.golang.org/api/option"
	pollen "google.golang.org/api/pollen/v1"
)

const (
	sourceName              = "google.pollen"
	maxGoogleDays           = 5
	defaultDays             = 5
	defaultPoll             = 12 * time.Hour
	freeTierMonthlyRequests = 5000
)

var aggregateCodeOrder = map[string]int{
	"GRASS":            10,
	"TREE":             20,
	"WEED":             30,
	"GRAMINALES":       100,
	"RAGWEED":          110,
	"MUGWORT":          120,
	"ALDER":            200,
	"ASH":              210,
	"BIRCH":            220,
	"COTTONWOOD":       230,
	"ELM":              240,
	"MAPLE":            250,
	"OLIVE":            260,
	"JUNIPER":          270,
	"OAK":              280,
	"PINE":             290,
	"CYPRESS_PINE":     300,
	"HAZEL":            310,
	"JAPANESE_CEDAR":   320,
	"JAPANESE_CYPRESS": 330,
}

type Request struct {
	Latitude     float64
	Longitude    float64
	Days         int
	LanguageCode string
}

type forecastClient interface {
	Lookup(context.Context, Request) (*pollen.LookupForecastResponse, error)
}

type sdkClient struct {
	service *pollen.Service
}

func NewSDKClient(ctx context.Context, apiKey string) (forecastClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("google pollen api key is required")
	}
	svc, err := pollen.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	return sdkClient{service: svc}, nil
}

func (c sdkClient) Lookup(ctx context.Context, req Request) (*pollen.LookupForecastResponse, error) {
	call := c.service.Forecast.Lookup().
		LocationLatitude(req.Latitude).
		LocationLongitude(req.Longitude).
		Days(int64(req.Days)).
		PlantsDescription(false).
		Context(ctx)
	if req.LanguageCode != "" {
		call.LanguageCode(req.LanguageCode)
	}
	return call.Do()
}

type Options struct {
	Latitude     float64
	Longitude    float64
	Timezone     string
	Interval     time.Duration
	Days         int
	LanguageCode string
}

type Collector struct {
	client     forecastClient
	opts       Options
	stateStore *store.StateStore
	netCheck   func() bool
	now        func() time.Time
	wake       chan struct{}

	mu    sync.RWMutex
	state domain.WeatherPollen

	fetchMu     sync.Mutex
	fetching    bool
	lastSuccess time.Time
}

func New(ctx context.Context, apiKey string, opts Options, stateStore *store.StateStore) (*Collector, error) {
	client, err := NewSDKClient(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return NewWithClient(client, opts, stateStore), nil
}

func NewWithClient(client forecastClient, opts Options, stateStore *store.StateStore) *Collector {
	if opts.Interval <= 0 {
		opts.Interval = defaultPoll
	}
	if opts.Days == 0 {
		opts.Days = defaultDays
	}
	if opts.Days > maxGoogleDays {
		opts.Days = maxGoogleDays
	}
	if opts.Days < 1 {
		opts.Days = 1
	}
	if strings.TrimSpace(opts.Timezone) == "" {
		opts.Timezone = "UTC"
	}
	return &Collector{
		client:     client,
		opts:       opts,
		stateStore: stateStore,
		now:        time.Now,
		wake:       make(chan struct{}, 1),
	}
}

func (c *Collector) ID() string                 { return sourceName }
func (c *Collector) Domain() domain.DomainType  { return domain.DomainWeatherPollen }
func (c *Collector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *Collector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Collector) Start(ctx context.Context) error {
	if c.client == nil {
		return errors.New("google pollen client is required")
	}
	go func() {
		c.fetchIfReady(ctx)
		t := time.NewTicker(c.opts.Interval)
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

func (c *Collector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Collector) fetchIfReady(ctx context.Context) {
	if c.netCheck != nil && !c.netCheck() {
		log.Printf("[%s] skipping fetch: internet connectivity checker is down", c.ID())
		return
	}
	if !c.reserveFetch() {
		return
	}
	if err := c.Fetch(ctx); err != nil {
		c.finishFetch(false)
		log.Printf("[%s] fetch: %v", c.ID(), err)
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "error", Message: err.Error()})
		return
	}
	c.finishFetch(true)
	c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
}

func (c *Collector) reserveFetch() bool {
	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()
	if c.fetching {
		return false
	}
	now := c.now().UTC()
	if !c.lastSuccess.IsZero() && now.Sub(c.lastSuccess) < c.opts.Interval {
		return false
	}
	c.fetching = true
	return true
}

func (c *Collector) finishFetch(success bool) {
	c.fetchMu.Lock()
	if success {
		c.lastSuccess = c.now().UTC()
	}
	c.fetching = false
	c.fetchMu.Unlock()
}

func (c *Collector) Fetch(ctx context.Context) error {
	if c.client == nil {
		return errors.New("google pollen client is required")
	}
	resp, err := c.client.Lookup(ctx, Request{
		Latitude:     c.opts.Latitude,
		Longitude:    c.opts.Longitude,
		Days:         c.opts.Days,
		LanguageCode: strings.TrimSpace(c.opts.LanguageCode),
	})
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(c.opts.Timezone)
	if err != nil {
		loc = time.UTC
	}
	state := MapResponse(resp, c.now().UTC(), loc)
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[%s] fetched pollen forecast: region=%s days=%d current=%t", c.ID(), state.RegionCode, len(state.Days), state.Current != nil)
	return nil
}

func MapResponse(resp *pollen.LookupForecastResponse, now time.Time, loc *time.Location) domain.WeatherPollen {
	if loc == nil {
		loc = time.UTC
	}
	state := domain.WeatherPollen{
		StateBase: domain.StateBase{UpdatedAt: now.UTC()},
		Source:    sourceName,
		Days:      []domain.PollenDay{},
	}
	if resp == nil {
		return state
	}
	state.RegionCode = resp.RegionCode
	today := now.In(loc).Format("2006-01-02")
	for _, info := range resp.DailyInfo {
		day, ok := mapDay(info)
		if !ok {
			continue
		}
		state.Days = append(state.Days, day)
		if day.Date == today && state.Current == nil {
			current := day
			state.Current = &current
		}
	}
	return state
}

func mapDay(info *pollen.DayInfo) (domain.PollenDay, bool) {
	if info == nil || info.Date == nil || info.Date.Year == 0 || info.Date.Month == 0 || info.Date.Day == 0 {
		return domain.PollenDay{}, false
	}
	day := domain.PollenDay{
		Date:   fmt.Sprintf("%04d-%02d-%02d", info.Date.Year, info.Date.Month, info.Date.Day),
		Types:  []domain.PollenRow{},
		Plants: []domain.PollenRow{},
	}
	for _, t := range info.PollenTypeInfo {
		if t == nil {
			continue
		}
		row := domain.PollenRow{
			Code:        t.Code,
			DisplayName: firstNonEmpty(t.DisplayName, titleCode(t.Code)),
			InSeason:    t.InSeason,
			UPI:         mapIndex(t.IndexInfo),
		}
		day.Types = append(day.Types, row)
		day.HealthRecommendations = append(day.HealthRecommendations, t.HealthRecommendations...)
	}
	for _, p := range info.PlantInfo {
		if p == nil {
			continue
		}
		day.Plants = append(day.Plants, domain.PollenRow{
			Code:        p.Code,
			DisplayName: firstNonEmpty(p.DisplayName, titleCode(p.Code)),
			InSeason:    p.InSeason,
			UPI:         mapIndex(p.IndexInfo),
		})
	}
	day.HealthRecommendations = uniqueStrings(day.HealthRecommendations)
	day.Aggregate = aggregate(day.Types, day.Plants)
	return day, true
}

func mapIndex(info *pollen.IndexInfo) *domain.PollenIndex {
	if info == nil {
		return nil
	}
	return &domain.PollenIndex{
		Value:       int(info.Value),
		Category:    info.Category,
		Description: info.IndexDescription,
		Color:       cssColor(info.Color),
	}
}

type aggregateCandidate struct {
	row    domain.PollenRow
	isType bool
	order  int
}

func aggregate(types, plants []domain.PollenRow) *domain.PollenAggregate {
	candidates := make([]aggregateCandidate, 0, len(types)+len(plants))
	for i, row := range types {
		if row.UPI != nil {
			candidates = append(candidates, aggregateCandidate{row: row, isType: true, order: i})
		}
	}
	for i, row := range plants {
		if row.UPI != nil {
			candidates = append(candidates, aggregateCandidate{row: row, order: len(types) + i})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.row.UPI.Value != b.row.UPI.Value {
			return a.row.UPI.Value > b.row.UPI.Value
		}
		if a.row.InSeason != b.row.InSeason {
			return a.row.InSeason
		}
		if a.isType != b.isType {
			return a.isType
		}
		if aggregateOrder(a.row.Code) != aggregateOrder(b.row.Code) {
			return aggregateOrder(a.row.Code) < aggregateOrder(b.row.Code)
		}
		return strings.TrimSpace(a.row.Code) < strings.TrimSpace(b.row.Code)
	})
	best := candidates[0].row
	return &domain.PollenAggregate{
		Value:       best.UPI.Value,
		Label:       best.DisplayName,
		Code:        best.Code,
		Category:    best.UPI.Category,
		Description: best.UPI.Description,
		Color:       best.UPI.Color,
	}
}

func aggregateOrder(code string) int {
	if order, ok := aggregateCodeOrder[strings.ToUpper(strings.TrimSpace(code))]; ok {
		return order
	}
	return 10000
}

func cssColor(c *pollen.Color) string {
	if c == nil {
		return ""
	}
	alpha := c.Alpha
	if alpha == 0 {
		alpha = 1
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%.3g)", clampColor(c.Red), clampColor(c.Green), clampColor(c.Blue), math.Max(0, math.Min(1, alpha)))
}

func clampColor(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 255
	}
	return int(math.Round(v * 255))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func titleCode(code string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(code)), "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func ResolveOptions(lat, lon float64, timezone string, interval time.Duration, days int, languageCode string) Options {
	if interval <= 0 {
		interval = defaultPoll
	}
	if days == 0 {
		days = defaultDays
	}
	if days > maxGoogleDays {
		days = maxGoogleDays
	}
	if days < 1 {
		days = 1
	}
	return Options{
		Latitude:     lat,
		Longitude:    lon,
		Timezone:     timezone,
		Interval:     interval,
		Days:         days,
		LanguageCode: strings.TrimSpace(languageCode),
	}
}

func EstimatedMonthlyRequests(interval time.Duration) int {
	if interval <= 0 {
		interval = defaultPoll
	}
	return int(math.Ceil((30 * 24 * time.Hour).Seconds() / interval.Seconds()))
}

func ExceedsFreeTier(interval time.Duration) bool {
	return EstimatedMonthlyRequests(interval) > freeTierMonthlyRequests
}
