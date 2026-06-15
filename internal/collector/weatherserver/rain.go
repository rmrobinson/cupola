package weatherserver

import (
	"context"
	"log"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rmrobinson/cupola/internal/domain"
	weatherv1 "github.com/rmrobinson/weather-server/proto/weather/v1"
	"github.com/rmrobinson/cupola/internal/store"
)

// RainCollector queries QueryRainAccumulation for every active subscription
// and publishes results as the weather.rain_accumulation domain.
//
// Widget subscription params: {"since": "thursday"} (lowercase day name).
// The collector computes the most recent occurrence of that weekday at midnight
// in the configured timezone and queries from there to now.
type RainCollector struct {
	address    string
	useTLS     bool
	caCert     string
	subs       *store.SubscriptionManager
	stateStore *store.StateStore
	loc        *time.Location
	wake       chan struct{}
}

func NewRainCollector(address string, useTLS bool, caCert string, subs *store.SubscriptionManager, stateStore *store.StateStore, loc *time.Location) *RainCollector {
	return &RainCollector{
		address:    address,
		useTLS:     useTLS,
		caCert:     caCert,
		subs:       subs,
		stateStore: stateStore,
		loc:        loc,
		wake:       make(chan struct{}, 1),
	}
}

func (c *RainCollector) ID() string                { return "weatherserver.rain" }
func (c *RainCollector) Domain() domain.DomainType { return domain.DomainWeatherRainAccumulation }

func (c *RainCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *RainCollector) State() domain.DomainState {
	return c.stateStore.Get(c.Domain())
}

func (c *RainCollector) Start(ctx context.Context) error {
	go c.loop(ctx)
	return nil
}

func (c *RainCollector) loop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	c.refresh(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refresh(ctx)
		case <-c.wake:
			c.refresh(ctx)
		}
	}
}

func (c *RainCollector) refresh(ctx context.Context) {
	allParams := c.subs.ActiveParams(domain.DomainWeatherRainAccumulation)
	if len(allParams) == 0 {
		return
	}

	opts, err := dialOpts(c.useTLS, c.caCert)
	if err != nil {
		log.Printf("[weatherserver.rain] dial creds: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "error", Message: err.Error()})
		return
	}
	conn, err := grpc.NewClient(c.address, opts...)
	if err != nil {
		log.Printf("[weatherserver.rain] dial: %v", err)
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "error", Message: err.Error()})
		return
	}
	defer conn.Close()

	client := weatherv1.NewWeatherServiceClient(conn)

	// Seed from previous state so a failed individual query keeps its last good value.
	entries := make(map[string]domain.RainAccumulationEntry, len(allParams))
	if prev, ok := c.stateStore.Get(c.Domain()).(domain.WeatherRainAccumulation); ok {
		for k, v := range prev.Entries {
			entries[k] = v
		}
	}

	anySuccess := false
	for _, params := range allParams {
		since, _ := params["since"].(string)
		if since == "" {
			continue
		}
		wd, ok := parseWeekday(since)
		if !ok {
			log.Printf("[weatherserver.rain] unknown weekday %q in subscription params", since)
			continue
		}
		start := mostRecentWeekday(wd, c.loc)
		resp, err := client.QueryRainAccumulation(ctx, &weatherv1.RainAccumulationRequest{
			Start: timestamppb.New(start),
		})
		if err != nil {
			log.Printf("[weatherserver.rain] QueryRainAccumulation(%s): %v", since, err)
			c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "error", Message: err.Error()})
			continue
		}
		entries[since] = domain.RainAccumulationEntry{
			DayOfWeek:   since,
			RainMM:      resp.RainMm,
			PeriodStart: resp.ActualStart.AsTime(),
			PeriodEnd:   resp.ActualEnd.AsTime(),
		}
		anySuccess = true
	}

	if !anySuccess {
		return
	}

	c.stateStore.Set(domain.WeatherRainAccumulation{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Entries:   entries,
	})
	// Only publish "ok" on transition from a non-ok state.
	if snap, _ := c.stateStore.GetSystem(c.ID()); snap.Status != "ok" {
		c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
	}
}

func parseWeekday(s string) (time.Weekday, bool) {
	switch strings.ToLower(s) {
	case "sunday":
		return time.Sunday, true
	case "monday":
		return time.Monday, true
	case "tuesday":
		return time.Tuesday, true
	case "wednesday":
		return time.Wednesday, true
	case "thursday":
		return time.Thursday, true
	case "friday":
		return time.Friday, true
	case "saturday":
		return time.Saturday, true
	default:
		return 0, false
	}
}

// mostRecentWeekday returns midnight at the start of the most recent occurrence
// of wd in the given timezone. If today is wd, it returns today's midnight.
func mostRecentWeekday(wd time.Weekday, loc *time.Location) time.Time {
	now := time.Now().In(loc)
	daysBack := int(now.Weekday()) - int(wd)
	if daysBack < 0 {
		daysBack += 7
	}
	d := now.AddDate(0, 0, -daysBack)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
}
