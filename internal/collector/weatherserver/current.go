package weatherserver

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/rmrobinson/cupola/internal/domain"
	weatherv1 "github.com/rmrobinson/cupola/internal/proto/weather/v1"
	"github.com/rmrobinson/cupola/internal/store"
)

// CurrentCollector connects to a weather-server gRPC endpoint and streams
// real-time readings into the weather.current domain. No polling ticker is
// used; updates are pushed by the server.
type CurrentCollector struct {
	address    string
	useTLS     bool
	caCert     string
	stateStore *store.StateStore
	mu         sync.RWMutex
	state      domain.WeatherCurrent
}

func NewCurrentCollector(address string, useTLS bool, caCert string, stateStore *store.StateStore) *CurrentCollector {
	return &CurrentCollector{
		address:    address,
		useTLS:     useTLS,
		caCert:     caCert,
		stateStore: stateStore,
	}
}

func (c *CurrentCollector) ID() string                { return "weatherserver.current" }
func (c *CurrentCollector) Domain() domain.DomainType { return domain.DomainWeatherCurrent }

func (c *CurrentCollector) State() domain.DomainState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *CurrentCollector) Start(ctx context.Context) error {
	go c.streamLoop(ctx)
	return nil
}

func (c *CurrentCollector) streamLoop(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		wasConnected, err := c.runStream(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("[weatherserver.current] stream: %v; reconnect in %s", err, backoff)
			c.stateStore.PublishSystem(store.SystemEvent{
				CollectorID: c.ID(), Status: "error", Message: err.Error(),
			})
		}
		if wasConnected {
			backoff = time.Second
		} else if backoff < maxBackoff {
			backoff *= 2
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// runStream opens one streaming RPC call and processes messages until an error
// or context cancellation. Returns (true, nil) after a clean ctx shutdown,
// (true/false, err) on a stream failure, where the bool indicates whether at
// least one message was received before the failure.
func (c *CurrentCollector) runStream(ctx context.Context) (wasConnected bool, _ error) {
	opts, err := dialOpts(c.useTLS, c.caCert)
	if err != nil {
		return false, err
	}
	conn, err := grpc.NewClient(c.address, opts...)
	if err != nil {
		return false, fmt.Errorf("dial %s: %w", c.address, err)
	}
	defer conn.Close()

	client := weatherv1.NewWeatherServiceClient(conn)
	stream, err := client.StreamReadings(ctx, &weatherv1.StreamRequest{})
	if err != nil {
		if ctx.Err() != nil {
			return false, nil
		}
		return false, fmt.Errorf("open stream: %w", err)
	}

	for {
		r, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return wasConnected, nil
			}
			return wasConnected, fmt.Errorf("recv: %w", err)
		}
		if !wasConnected {
			wasConnected = true
			c.stateStore.PublishSystem(store.SystemEvent{CollectorID: c.ID(), Status: "ok"})
		}
		c.applyReading(r)
	}
}

func (c *CurrentCollector) applyReading(r *weatherv1.WeatherReading) {
	state := domain.WeatherCurrent{
		StateBase:     domain.StateBase{UpdatedAt: r.Timestamp.AsTime()},
		Temperature:   r.TempC,
		Humidity:      r.HumidityPct,
		Pressure:      r.PressureHpa,
		WindSpeed:     r.WindSpeedMs * 3.6,
		WindGust:      r.WindGustMs * 3.6,
		WindDirection: int(r.WindDirDeg),
		Precipitation: r.RainMmHr,
		UV:            r.UvIndex,
		RainEvent:     r.RainEventMm,
		RainDaily:     r.RainDailyMm,
		RainWeekly:    r.RainWeeklyMm,
		RainMonthly:   r.RainMonthlyMm,
		RainYearly:    r.RainYearlyMm,
		Condition: conditionString(r.Condition),
		FeelsLike: r.FeelsLikeC,
	}

	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	c.stateStore.Set(state)
	log.Printf("[weatherserver.current] %.1f°C (feels %.1f°C) hum=%.0f%% wind=%.1fkm/h dir=%d° UV=%.0f pres=%.1fhPa cond=%s",
		state.Temperature, state.FeelsLike, state.Humidity,
		state.WindSpeed, state.WindDirection, state.UV, state.Pressure, state.Condition)
}

func conditionString(c weatherv1.WeatherCondition) string {
	switch c {
	case weatherv1.WeatherCondition_WEATHER_CONDITION_SUNNY:
		return "sunny"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_MOSTLY_SUNNY:
		return "mostly sunny"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_PARTLY_CLOUDY:
		return "partly cloudy"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_MOSTLY_CLOUDY:
		return "mostly cloudy"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_OVERCAST:
		return "overcast"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_LIGHT_RAIN:
		return "light rain"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_RAIN:
		return "rain"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_HEAVY_RAIN:
		return "heavy rain"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_FREEZING_RAIN:
		return "freezing rain"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_SNOW:
		return "snow"
	case weatherv1.WeatherCondition_WEATHER_CONDITION_NIGHT:
		return "night"
	default:
		return ""
	}
}
