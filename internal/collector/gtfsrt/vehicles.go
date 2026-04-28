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
	agencies []*Agency
	state    *store.StateStore
	interval time.Duration
}

func (c *VehiclesCollector) ID() string                { return "gtfsrt.vehicles" }
func (c *VehiclesCollector) Domain() domain.DomainType { return domain.DomainTransitVehicles }
func (c *VehiclesCollector) State() domain.DomainState { return c.state.Get(c.Domain()) }

func (c *VehiclesCollector) Start(ctx context.Context) error {
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

func (c *VehiclesCollector) fetch() {
	var vehicles []domain.TransitVehicle

	for _, ag := range c.agencies {
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
				v := domain.TransitVehicle{
					AgencyID:  ag.ID,
					VehicleID: vid,
					RouteID:   routeID,
					RouteName: ag.Schedule.RouteName(routeID),
					Lat:       lat,
					Lon:       lon,
					UpdatedAt: time.Unix(int64(vp.GetTimestamp()), 0),
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
