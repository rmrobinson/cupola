// Package gtfsrt implements GTFS-RT collectors for transit arrivals, vehicle
// positions, and service alerts.
package gtfsrt

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	pb "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"

	"github.com/rmrobinson/cupola/internal/collector/gtfs"
	"github.com/rmrobinson/cupola/internal/store"
)

// Agency holds the RT feed URLs and static schedule for one transit agency.
type Agency struct {
	ID                   string
	StaticURLs           []string
	TripUpdatesURLs      []string
	VehiclePositionsURLs []string
	AlertsURL            string
	Schedule             *gtfs.Schedule
}

// NewCollectors creates the three transit domain collectors sharing the given
// agencies. Static GTFS data is loaded (from network or disk cache) synchronously
// before returning so all three collectors have route/stop names from the first poll.
// loc is used to convert wall-clock time to the agency's local time for calendar
// queries; pass time.UTC when the timezone is unknown.
func NewCollectors(
	agencies []*Agency,
	subs *store.SubscriptionManager,
	state *store.StateStore,
	rtInterval time.Duration,
	staticInterval time.Duration,
	cacheDir string,
	db *store.SQLiteStore,
	loc *time.Location,
) (*ArrivalsCollector, *VehiclesCollector, *AlertsCollector) {
	if rtInterval == 0 {
		rtInterval = 30 * time.Second
	}
	if staticInterval == 0 {
		staticInterval = 24 * time.Hour
	}
	if loc == nil {
		loc = time.UTC
	}

	for _, ag := range agencies {
		if err := gtfs.LoadAndPersist(ag.Schedule, ag.ID, ag.StaticURLs, cacheDir, db); err != nil {
			// Non-fatal: collectors will still work, just without display names.
			log.Printf("[gtfsrt] %s: initial static load: %v", ag.ID, err)
		}
	}

	arr := &ArrivalsCollector{
		agencies:       agencies,
		subs:           subs,
		state:          state,
		rtInterval:     rtInterval,
		staticInterval: staticInterval,
		cacheDir:       cacheDir,
		db:             db,
		loc:            loc,
		inFallback:     make(map[string]bool),
		wake:           make(chan struct{}, 1),
	}
	veh := &VehiclesCollector{agencies: agencies, state: state, interval: rtInterval}
	alt := &AlertsCollector{agencies: agencies, state: state, interval: rtInterval}
	return arr, veh, alt
}

// fetchFeed downloads a GTFS-RT protobuf endpoint and returns the parsed message.
func fetchFeed(url string) (*pb.FeedMessage, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var msg pb.FeedMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &msg, nil
}
