package gtfsrt

import (
	"context"
	"log"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	"github.com/rmrobinson/cupola/internal/store"
)

// VehiclesCollector polls GTFS-RT vehicle position feeds and publishes
// transit.vehicles. All vehicles from all configured agencies are included.
type VehiclesCollector struct {
	agencies AgencySource
	state    *store.StateStore
	interval time.Duration
	wake     chan struct{}
	netCheck func() bool
}

func (c *VehiclesCollector) SetNetCheck(fn func() bool) { c.netCheck = fn }

func (c *VehiclesCollector) ID() string                { return "gtfsrt.vehicles" }
func (c *VehiclesCollector) Domain() domain.DomainType { return domain.DomainTransitVehicles }
func (c *VehiclesCollector) State() domain.DomainState { return c.state.Get(c.Domain()) }

func (c *VehiclesCollector) OnSubscription() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *VehiclesCollector) Start(ctx context.Context) error {
	go func() {
		prevUp := true
		if c.netCheck != nil && !c.netCheck() {
			prevUp = false
		} else {
			c.fetch()
		}
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				isUp := c.netCheck == nil || c.netCheck()
				if !isUp {
					if prevUp {
						c.publishEmpty()
					}
					prevUp = false
					continue
				}
				prevUp = true
				c.fetch()
			case <-c.wake:
				if c.netCheck != nil && !c.netCheck() {
					continue
				}
				c.fetch()
			}
		}
	}()
	return nil
}

func (c *VehiclesCollector) publishEmpty() {
	c.state.Set(domain.TransitVehicles{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Vehicles:  []domain.TransitVehicle{},
	})
}

func (c *VehiclesCollector) fetch() {
	var vehicles []domain.TransitVehicle

	for _, ag := range c.agencies.List() {
		for _, url := range ag.VehiclePositionsURLs {
			feed, err := fetchFeed(url)
			if err != nil {
				log.Printf("[gtfsrt] vehicle positions %s: %v", url, err)
				continue
			}

			for _, entity := range feed.GetEntity() {
				vp := entity.GetVehicle()
				if vp == nil {
					continue
				}
				pos := vp.GetPosition()
				if pos == nil {
					continue
				}

				lat := float64(pos.GetLatitude())
				lon := float64(pos.GetLongitude())
				if lat == 0 && lon == 0 {
					continue
				}

				vid := entity.GetId()
				if vd := vp.GetVehicle(); vd != nil && vd.GetId() != "" {
					vid = vd.GetId()
				}

				routeID := vp.GetTrip().GetRouteId()
				routeName, routeTypeInt := ag.Schedule.RouteNameAndType(routeID)
				v := domain.TransitVehicle{
					AgencyID:    ag.ID,
					VehicleID:   vid,
					RouteID:     routeID,
					RouteName:   routeName,
					VehicleType: gtfsRouteTypeToVehicleType(routeTypeInt),
					Lat:         lat,
					Lon:         lon,
					UpdatedAt:   time.Unix(int64(vp.GetTimestamp()), 0),
				}

				if b := pos.GetBearing(); b != 0 {
					bf := float64(b)
					v.Bearing = &bf
				}
				if s := pos.GetSpeed(); s != 0 {
					sf := float64(s) * 3.6 // m/s → km/h
					v.Speed = &sf
				}

				vehicles = append(vehicles, v)
			}
		}
	}

	c.state.Set(domain.TransitVehicles{
		StateBase: domain.StateBase{UpdatedAt: time.Now()},
		Vehicles:  vehicles,
	})
}

func gtfsRouteTypeToVehicleType(t int) string {
	switch t {
	case 0:
		return "lrt" // tram / light rail / streetcar
	case 1:
		return "metro" // subway / metro
	case 2:
		return "train" // intercity / commuter rail
	case 3:
		return "bus" // bus
	case 11:
		return "bus" // trolleybus
	default:
		return "bus"
	}
}
