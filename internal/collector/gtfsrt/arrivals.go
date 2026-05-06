package gtfsrt

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

	pb "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"

	"github.com/rmrobinson/cupola/internal/collector/gtfs"
	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

const maxArrivalsPerStop = 8

// ArrivalsCollector polls GTFS-RT trip updates and publishes transit.arrivals.
// Only (agency:route:stop_id) combinations that have active widget subscriptions
// are populated; all others are omitted to avoid unbounded state growth.
type ArrivalsCollector struct {
	agencies       []*Agency
	subs           *store.SubscriptionManager
	state          *store.StateStore
	rtInterval     time.Duration
	staticInterval time.Duration
	cacheDir       string
	db             *store.SQLiteStore
	loc            *time.Location       // local timezone for calendar queries
	inFallback     map[string]bool      // agency_id → currently serving static schedule
	wake           chan struct{}         // buffered(1): nudges rtLoop to fetch immediately
}

func (c *ArrivalsCollector) ID() string                { return "gtfsrt.arrivals" }
func (c *ArrivalsCollector) Domain() domain.DomainType { return domain.DomainTransitArrivals }
func (c *ArrivalsCollector) State() domain.DomainState { return c.state.Get(c.Domain()) }

// OnSubscription implements collector.SubscriptionNotifiable. A non-blocking
// send into the wake channel causes rtLoop to fetch on the next iteration
// without waiting for the full poll interval.
func (c *ArrivalsCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *ArrivalsCollector) Start(ctx context.Context) error {
	go c.staticLoop(ctx)
	go c.rtLoop(ctx)
	return nil
}

func (c *ArrivalsCollector) staticLoop(ctx context.Context) {
	t := time.NewTicker(c.staticInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, ag := range c.agencies {
				if err := gtfs.LoadAndPersist(ag.Schedule, ag.ID, ag.StaticURLs, c.cacheDir, c.db); err != nil {
					log.Printf("[gtfsrt] %s: static refresh: %v", ag.ID, err)
				}
			}
		}
	}
}

func (c *ArrivalsCollector) rtLoop(ctx context.Context) {
	c.fetch()
	t := time.NewTicker(c.rtInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.fetch()
		case <-c.wake:
			c.fetch()
		}
	}
}

func (c *ArrivalsCollector) fetch() {
	activeSubs := c.subs.ActiveParams(domain.DomainTransitArrivals)

	// Build set of canonical state keys we actually need to fill.
	wanted := make(map[string]bool, len(activeSubs))
	for _, p := range activeSubs {
		agency, _ := p["agency"].(string)
		route, _ := p["route"].(string)
		stop, _ := p["stop_id"].(string)
		if agency != "" && route != "" && stop != "" {
			wanted[agency+":"+route+":"+stop] = true
		}
	}

	if len(wanted) == 0 {
		return
	}

	stops := make(map[string]domain.StopArrivals)
	now := time.Now().In(c.loc)

	for _, ag := range c.agencies {
		if !c.fetchRTForAgency(ag, wanted, stops, now) && c.db != nil {
			c.fetchStaticForAgency(ag, wanted, stops, now)
		}
	}

	for key, sa := range stops {
		sort.Slice(sa.Arrivals, func(i, j int) bool {
			return sa.Arrivals[i].Scheduled.Before(sa.Arrivals[j].Scheduled)
		})
		if len(sa.Arrivals) > maxArrivalsPerStop {
			sa.Arrivals = sa.Arrivals[:maxArrivalsPerStop]
		}
		stops[key] = sa
	}

	c.state.Set(domain.TransitArrivals{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Stops:     stops,
	})
}

// fetchRTForAgency tries each trip-updates URL for ag. Returns true if at least
// one URL succeeded and contributed data. Also clears the per-agency fallback
// flag and logs a recovery message the first time RT is restored after an outage.
func (c *ArrivalsCollector) fetchRTForAgency(ag *Agency, wanted map[string]bool, stops map[string]domain.StopArrivals, now time.Time) bool {
	if len(ag.TripUpdatesURLs) == 0 {
		return false
	}
	anyOK := false
	for _, url := range ag.TripUpdatesURLs {
		feed, err := fetchFeed(url)
		if err != nil {
			log.Printf("[gtfsrt] %s: trip updates error: %v", ag.ID, err)
			c.state.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
			continue
		}
		c.state.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
		collectArrivals(feed, ag, wanted, stops, now)
		anyOK = true
	}
	if anyOK && c.inFallback[ag.ID] {
		log.Printf("[gtfsrt] %s: RT feed restored, resuming real-time arrivals", ag.ID)
		c.inFallback[ag.ID] = false
	}
	return anyOK
}

// fetchStaticForAgency populates stops with schedule-based arrivals from SQLite
// for every subscribed (route, stop) pair belonging to ag. Called when all RT
// feeds for the agency are unavailable. Logs once on transition into fallback.
func (c *ArrivalsCollector) fetchStaticForAgency(ag *Agency, wanted map[string]bool, stops map[string]domain.StopArrivals, now time.Time) {
	if !c.inFallback[ag.ID] {
		log.Printf("[gtfsrt] %s: RT unavailable, switching to static schedule fallback", ag.ID)
		c.inFallback[ag.ID] = true
	}
	for key := range wanted {
		parts := strings.SplitN(key, ":", 3)
		if len(parts) != 3 || parts[0] != ag.ID {
			continue
		}
		routeID, stopID := parts[1], parts[2]

		deps, err := c.db.QueryUpcomingDepartures(ag.ID, routeID, stopID, now, maxArrivalsPerStop)
		if err != nil {
			log.Printf("[gtfsrt] %s: static query %s: %v", ag.ID, key, err)
			continue
		}
		if len(deps) == 0 {
			continue
		}

		sa := stops[key]
		if sa.AgencyID == "" {
			sa = domain.StopArrivals{
				AgencyID:  ag.ID,
				RouteID:   routeID,
				RouteName: ag.Schedule.RouteName(routeID),
				StopID:    stopID,
				StopName:  ag.Schedule.StopName(stopID),
			}
		}
		for _, dep := range deps {
			sa.Arrivals = append(sa.Arrivals, domain.Arrival{
				TripID:    dep.TripID,
				Headsign:  dep.Headsign,
				Scheduled: dep.DepartureTime,
			})
		}
		stops[key] = sa
	}
}

func collectArrivals(feed *pb.FeedMessage, ag *Agency, wanted map[string]bool, stops map[string]domain.StopArrivals, now time.Time) {
	for _, entity := range feed.GetEntity() {
		tu := entity.GetTripUpdate()
		if tu == nil {
			continue
		}
		routeID := tu.GetTrip().GetRouteId()
		tripID := tu.GetTrip().GetTripId()

		for _, stu := range tu.GetStopTimeUpdate() {
			if stu.GetScheduleRelationship() == pb.TripUpdate_StopTimeUpdate_SKIPPED {
				continue
			}
			stopID := stu.GetStopId()
			stateKey := ag.ID + ":" + routeID + ":" + stopID
			if !wanted[stateKey] {
				continue
			}

			evt := stu.GetArrival()
			if evt == nil {
				evt = stu.GetDeparture()
			}
			if evt == nil {
				continue
			}

			t := evt.GetTime()
			if t == 0 {
				continue
			}

			predicted := time.Unix(t, 0)
			var delay *int
			var scheduled time.Time

			if d := evt.GetDelay(); d != 0 {
				di := int(d)
				delay = &di
				scheduled = predicted.Add(-time.Duration(d) * time.Second)
			} else {
				scheduled = predicted
			}

			if scheduled.Before(now) {
				continue
			}

			arr := domain.Arrival{
				TripID:    tripID,
				Headsign:  ag.Schedule.TripHeadsign(tripID),
				Scheduled: scheduled,
				Predicted: &predicted,
				Delay:     delay,
			}
			if vd := tu.GetVehicle(); vd != nil {
				if vid := vd.GetId(); vid != "" {
					arr.VehicleID = &vid
				}
			}

			sa := stops[stateKey]
			if sa.AgencyID == "" {
				sa = domain.StopArrivals{
					AgencyID:  ag.ID,
					RouteID:   routeID,
					RouteName: ag.Schedule.RouteName(routeID),
					StopID:    stopID,
					StopName:  ag.Schedule.StopName(stopID),
				}
			}
			sa.Arrivals = append(sa.Arrivals, arr)
			stops[stateKey] = sa
		}
	}
}
