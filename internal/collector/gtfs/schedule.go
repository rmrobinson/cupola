// Package gtfs downloads and parses GTFS static feeds, providing thread-safe
// lookups of routes, stops, and trips for use by the GTFS-RT collectors.
package gtfs

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/store"
)

// Route holds the display fields from routes.txt.
type Route struct {
	ID        string
	AgencyID  string
	ShortName string
	LongName  string
	Type      int    // GTFS route_type (0=tram/LRT, 1=metro, 2=rail, 3=bus, etc.)
	Color     string // route_color without leading '#'; empty if unset
}

// ShapePoint is one vertex in a GTFS shape polyline.
type ShapePoint struct {
	Lat      float64
	Lon      float64
	Sequence int
}

// Stop holds the display fields from stops.txt.
type Stop struct {
	ID   string
	Code string
	Name string
	Lat  float64
	Lon  float64
}

// Trip holds the display fields from trips.txt.
type Trip struct {
	ID        string
	RouteID   string
	Headsign  string
	ShapeID   string // shape_id from trips.txt; empty if feed omits it
	ServiceID string // service_id from trips.txt; used for calendar-based filtering
}

// Schedule holds parsed GTFS static data merged from one or more feed ZIPs.
// All methods are safe for concurrent use.
type Schedule struct {
	mu          sync.RWMutex
	routes      map[string]Route
	stops       map[string]Stop
	trips       map[string]Trip
	routeStops  map[string][]string     // route_id → deduplicated stop_ids
	shapes      map[string][]ShapePoint // shape_id → ordered points
	routeShapes map[string][]string     // route_id → ordered unique shape_ids (precomputed index)
}

// ScheduleStats summarizes the static GTFS data currently cached in memory.
type ScheduleStats struct {
	Routes     int `json:"routes"`
	Stops      int `json:"stops"`
	Trips      int `json:"trips"`
	Shapes     int `json:"shapes"`
	RouteStops int `json:"route_stops"`
}

func New() *Schedule {
	return &Schedule{
		routes:      make(map[string]Route),
		stops:       make(map[string]Stop),
		trips:       make(map[string]Trip),
		routeStops:  make(map[string][]string),
		shapes:      make(map[string][]ShapePoint),
		routeShapes: make(map[string][]string),
	}
}

// Stats returns counts for the currently loaded in-memory GTFS schedule.
func (s *Schedule) Stats() ScheduleStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	routeStops := 0
	for _, stops := range s.routeStops {
		routeStops += len(stops)
	}
	return ScheduleStats{
		Routes:     len(s.routes),
		Stops:      len(s.stops),
		Trips:      len(s.trips),
		Shapes:     len(s.shapes),
		RouteStops: routeStops,
	}
}

// Load downloads and merges all provided feed ZIP URLs into the schedule.
// On success the existing data is atomically replaced.
func (s *Schedule) Load(agencyID string, urls []string) error {
	blobs := make([][]byte, 0, len(urls))
	for _, url := range urls {
		data, err := downloadZip(url)
		if err != nil {
			return fmt.Errorf("%s feed %s: %w", agencyID, url, err)
		}
		blobs = append(blobs, data)
	}
	return s.parseFromBlobs(agencyID, blobs)
}

// parseFromBlobs merges raw GTFS ZIP bytes into the schedule, atomically
// replacing the existing in-memory data on success.
func (s *Schedule) parseFromBlobs(agencyID string, blobs [][]byte) error {
	routes := make(map[string]Route)
	stops := make(map[string]Stop)
	trips := make(map[string]Trip)
	routeStops := make(map[string][]string)
	shapes := make(map[string][]ShapePoint)

	for _, data := range blobs {
		if err := mergeZipBytes(data, routes, stops, trips, routeStops, shapes, nil); err != nil {
			return fmt.Errorf("%s: %w", agencyID, err)
		}
	}

	// Sort shape points once after all feeds are merged; GTFS does not guarantee
	// row order within shapes.txt and multiple feed ZIPs may interleave points.
	for id, pts := range shapes {
		sort.Slice(pts, func(i, j int) bool { return pts[i].Sequence < pts[j].Sequence })
		shapes[id] = pts
	}

	// Precompute a route→shape_ids index from the fully-merged trip set so that
	// ShapesForRoute is a constant-time lookup rather than a full trip scan.
	routeShapes := buildRouteShapes(trips)

	s.mu.Lock()
	s.routes = routes
	s.stops = stops
	s.trips = trips
	s.routeStops = routeStops
	s.shapes = shapes
	s.routeShapes = routeShapes
	s.mu.Unlock()

	log.Printf("[gtfs] %s: loaded %d routes, %d stops, %d trips, %d shapes",
		agencyID, len(routes), len(stops), len(trips), len(shapes))
	return nil
}

// AllRoutes returns all routes sorted numerically by short name where possible,
// falling back to lexicographic order for non-numeric or mixed names.
func (s *Schedule) AllRoutes() []Route {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Route, 0, len(s.routes))
	for _, r := range s.routes {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		ni := out[i].ShortName
		if ni == "" {
			ni = out[i].LongName
		}
		nj := out[j].ShortName
		if nj == "" {
			nj = out[j].LongName
		}
		ii, erri := strconv.Atoi(ni)
		ij, errj := strconv.Atoi(nj)
		if erri == nil && errj == nil {
			return ii < ij
		}
		return ni < nj
	})
	return out
}

// HasRoutes reports whether routes.txt has been loaded into memory.
func (s *Schedule) HasRoutes() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.routes) > 0
}

// StopsForRoute returns all stops served by routeID, deduplicated.
func (s *Schedule) StopsForRoute(routeID string) []Stop {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.routeStops[routeID]
	out := make([]Stop, 0, len(ids))
	for _, id := range ids {
		if st, ok := s.stops[id]; ok {
			out = append(out, st)
		}
	}
	return out
}

// RouteName returns the short name for routeID, falling back to the long name
// or the raw ID if the route is not found.
func (s *Schedule) RouteName(routeID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.routes[routeID]; ok {
		if r.ShortName != "" {
			return r.ShortName
		}
		return r.LongName
	}
	return routeID
}

// RouteNameAndType returns the display name and GTFS route_type for routeID
// in a single lock acquisition. Falls back to the raw ID and type 3 (bus)
// when the route is not found.
func (s *Schedule) RouteNameAndType(routeID string) (name string, routeType int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.routes[routeID]; ok {
		n := r.ShortName
		if n == "" {
			n = r.LongName
		}
		if n == "" {
			n = routeID
		}
		return n, r.Type
	}
	return routeID, 3
}

// StopName returns the display name for stopID, falling back to the raw ID.
func (s *Schedule) StopName(stopID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, ok := s.stops[stopID]; ok {
		return st.Name
	}
	return stopID
}

// TripHeadsign returns the headsign for tripID.
func (s *Schedule) TripHeadsign(tripID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trips[tripID].Headsign
}

// HasShapes reports whether shapes.txt was present and non-empty in the feed.
func (s *Schedule) HasShapes() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.shapes) > 0
}

// ShapesForRoute returns the distinct polylines for routeID (one per unique
// shape_id referenced by that route's trips) and the route's branded color
// (hex without '#'; empty string if unset). Each polyline is a slice of
// [lat, lon] pairs ordered by shape_pt_sequence. Returns nil, "" when the
// route is unknown or has no associated shapes.
func (s *Schedule) ShapesForRoute(routeID string) ([][][2]float64, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rt, ok := s.routes[routeID]
	if !ok {
		return nil, ""
	}

	shapeIDs := s.routeShapes[routeID]
	if len(shapeIDs) == 0 {
		return nil, rt.Color
	}

	out := make([][][2]float64, 0, len(shapeIDs))
	for _, shapeID := range shapeIDs {
		pts := s.shapes[shapeID]
		if len(pts) == 0 {
			continue
		}
		line := make([][2]float64, len(pts))
		for i, p := range pts {
			line[i] = [2]float64{p.Lat, p.Lon}
		}
		out = append(out, line)
	}
	return out, rt.Color
}

// buildRouteShapes builds a route_id → []shape_id index from the merged trip
// set. Shape IDs are recorded in first-seen order (a stable proxy for
// direction within most GTFS feeds) with duplicates removed.
func buildRouteShapes(trips map[string]Trip) map[string][]string {
	seen := make(map[string]map[string]struct{})
	out := make(map[string][]string)
	for _, tr := range trips {
		if tr.ShapeID == "" {
			continue
		}
		if seen[tr.RouteID] == nil {
			seen[tr.RouteID] = make(map[string]struct{})
		}
		if _, dup := seen[tr.RouteID][tr.ShapeID]; !dup {
			seen[tr.RouteID][tr.ShapeID] = struct{}{}
			out[tr.RouteID] = append(out[tr.RouteID], tr.ShapeID)
		}
	}
	return out
}

// ── internals ────────────────────────────────────────────────────────────────

// downloadZip fetches a GTFS ZIP archive from url and returns the raw bytes.
func downloadZip(url string) ([]byte, error) {
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

// mergeZipBytes parses a raw GTFS ZIP and merges its contents into the provided
// maps. When td is non-nil, timetable rows (stop_times, services, exceptions)
// are also collected in the same pass for SQLite persistence.
func mergeZipBytes(data []byte, routes map[string]Route, stops map[string]Stop, trips map[string]Trip, routeStops map[string][]string, shapes map[string][]ShapePoint, td *timetableData) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("unzip: %w", err)
	}

	for _, f := range zr.File {
		var parseErr error
		switch f.Name {
		case "routes.txt":
			parseErr = parseCSV(f, func(h map[string]int, row []string) {
				id := col(row, h, "route_id")
				if id == "" {
					return
				}
				routeType, _ := strconv.Atoi(col(row, h, "route_type"))
				routes[id] = Route{
					ID:        id,
					AgencyID:  col(row, h, "agency_id"),
					ShortName: col(row, h, "route_short_name"),
					LongName:  col(row, h, "route_long_name"),
					Type:      routeType,
					Color:     strings.TrimPrefix(col(row, h, "route_color"), "#"),
				}
			})
		case "stops.txt":
			parseErr = parseCSV(f, func(h map[string]int, row []string) {
				id := col(row, h, "stop_id")
				if id == "" {
					return
				}
				lat, _ := strconv.ParseFloat(col(row, h, "stop_lat"), 64)
				lon, _ := strconv.ParseFloat(col(row, h, "stop_lon"), 64)
				stops[id] = Stop{
					ID:   id,
					Code: col(row, h, "stop_code"),
					Name: col(row, h, "stop_name"),
					Lat:  lat,
					Lon:  lon,
				}
			})
		case "trips.txt":
			parseErr = parseCSV(f, func(h map[string]int, row []string) {
				id := col(row, h, "trip_id")
				if id == "" {
					return
				}
				trips[id] = Trip{
					ID:        id,
					RouteID:   col(row, h, "route_id"),
					Headsign:  col(row, h, "trip_headsign"),
					ShapeID:   col(row, h, "shape_id"),
					ServiceID: col(row, h, "service_id"),
				}
			})
		case "shapes.txt":
			parseErr = parseCSV(f, func(h map[string]int, row []string) {
				shapeID := col(row, h, "shape_id")
				if shapeID == "" {
					return
				}
				lat, _ := strconv.ParseFloat(col(row, h, "shape_pt_lat"), 64)
				lon, _ := strconv.ParseFloat(col(row, h, "shape_pt_lon"), 64)
				seq, _ := strconv.Atoi(col(row, h, "shape_pt_sequence"))
				shapes[shapeID] = append(shapes[shapeID], ShapePoint{Lat: lat, Lon: lon, Sequence: seq})
			})
		case "calendar.txt":
			if td != nil {
				parseErr = parseCSV(f, func(h map[string]int, row []string) {
					svcID := col(row, h, "service_id")
					if svcID == "" {
						return
					}
					td.services = append(td.services, store.GTFSService{
						ServiceID:   svcID,
						WeekdayMask: buildWeekdayMask(row, h),
						StartDate:   col(row, h, "start_date"),
						EndDate:     col(row, h, "end_date"),
					})
				})
			}
		case "calendar_dates.txt":
			if td != nil {
				parseErr = parseCSV(f, func(h map[string]int, row []string) {
					svcID := col(row, h, "service_id")
					date := col(row, h, "date")
					excType := col(row, h, "exception_type")
					if svcID == "" || date == "" || excType == "" {
						return
					}
					td.exceptions = append(td.exceptions, store.GTFSServiceException{
						ServiceID: svcID,
						Date:      date,
						Added:     excType == "1",
					})
				})
			}
		}
		if parseErr != nil {
			return fmt.Errorf("%s: %w", f.Name, parseErr)
		}
	}

	// Second pass: build route→stop mapping from stop_times.txt.
	// Requires trips to be loaded first (first pass above).
	// When td is non-nil, timetable rows are also collected in this same pass.
	newStops := make(map[string]map[string]struct{}) // route_id → set of stop_ids
	for _, f := range zr.File {
		if f.Name != "stop_times.txt" {
			continue
		}
		if err := parseCSV(f, func(h map[string]int, row []string) {
			tripID := col(row, h, "trip_id")
			stopID := col(row, h, "stop_id")
			if tripID == "" || stopID == "" {
				return
			}
			t, ok := trips[tripID]
			if !ok || t.RouteID == "" {
				return
			}
			if newStops[t.RouteID] == nil {
				newStops[t.RouteID] = make(map[string]struct{})
			}
			newStops[t.RouteID][stopID] = struct{}{}

			if td != nil {
				depStr := col(row, h, "departure_time")
				if depStr == "" {
					return
				}
				depSecs, ok := parseDepartureSecs(depStr)
				if !ok {
					return
				}
				seq, _ := strconv.Atoi(col(row, h, "stop_sequence"))
				td.stopTimes = append(td.stopTimes, store.GTFSStopTime{
					RouteID:       t.RouteID,
					TripID:        tripID,
					StopID:        stopID,
					Headsign:      t.Headsign,
					ServiceID:     t.ServiceID,
					StopSequence:  seq,
					DepartureSecs: depSecs,
				})
			}
		}); err != nil {
			return fmt.Errorf("stop_times.txt: %w", err)
		}
	}
	// Merge newStops into routeStops, deduplicating against prior ZIPs.
	for routeID, stopSet := range newStops {
		existing := make(map[string]struct{}, len(routeStops[routeID]))
		for _, sid := range routeStops[routeID] {
			existing[sid] = struct{}{}
		}
		for stopID := range stopSet {
			if _, dup := existing[stopID]; !dup {
				routeStops[routeID] = append(routeStops[routeID], stopID)
			}
		}
	}
	return nil
}

func parseCSV(f *zip.File, fn func(headers map[string]int, row []string)) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	r := csv.NewReader(rc)
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	headers, err := r.Read()
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	h := make(map[string]int, len(headers))
	for i, name := range headers {
		// Strip UTF-8 BOM that some GTFS feeds include on the first field.
		h[strings.TrimPrefix(name, "\xef\xbb\xbf")] = i
	}

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fn(h, row)
	}
	return nil
}

func col(row []string, h map[string]int, name string) string {
	idx, ok := h[name]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}
