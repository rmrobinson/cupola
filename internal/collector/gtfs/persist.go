package gtfs

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/rmrobinson/cupola/internal/store"
)

// timetableData collects rows extracted from GTFS ZIP(s) for SQLite insertion.
type timetableData struct {
	stopTimes  []store.GTFSStopTime
	services   []store.GTFSService
	exceptions []store.GTFSServiceException
}

// LoadAndPersist is the primary entry point used at startup and during periodic
// static refreshes. It:
//  1. Attempts to download fresh ZIPs from urls.
//  2. On success: parses in-memory metadata and timetable rows in one pass,
//     saves ZIPs to disk cache, and replaces the SQLite timetable tables.
//  3. On any download failure: falls back to the disk cache. If SQLite is
//     already populated for this agency the cached ZIPs are only used to
//     restore in-memory metadata; if SQLite is empty they also repopulate it.
func LoadAndPersist(s *Schedule, agencyID string, urls []string, cacheDir string, db *store.GTFSSQLiteStore) error {
	blobs, downloadErr := tryDownload(urls)

	if downloadErr != nil {
		log.Printf("[gtfs] %s: download failed (%v), trying disk cache", agencyID, downloadErr)

		cached, err := LoadZips(cacheDir, agencyID)
		if err != nil {
			return fmt.Errorf("load cache: %w", err)
		}
		if len(cached) == 0 {
			return fmt.Errorf("download failed and no disk cache available: %w", downloadErr)
		}

		hasData, err := db.HasGTFSData(agencyID)
		if err != nil {
			return fmt.Errorf("check gtfs data: %w", err)
		}
		if !hasData {
			log.Printf("[gtfs] %s: SQLite timetable empty, populating from disk cache", agencyID)
			td, err := s.parseFromBlobsFull(agencyID, cached)
			if err != nil {
				return fmt.Errorf("parse cached zips: %w", err)
			}
			log.Printf("[gtfs] %s: persisting %d stop_times, %d services, %d exceptions",
				agencyID, len(td.stopTimes), len(td.services), len(td.exceptions))
			if err := db.ReplaceGTFSAgency(agencyID, td.stopTimes, td.services, td.exceptions); err != nil {
				return fmt.Errorf("populate sqlite from cache: %w", err)
			}
		} else {
			if err := s.parseFromBlobs(agencyID, cached); err != nil {
				return fmt.Errorf("parse cached zips: %w", err)
			}
		}
		return nil
	}

	td, err := s.parseFromBlobsFull(agencyID, blobs)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if err := SaveZips(cacheDir, agencyID, blobs); err != nil {
		log.Printf("[gtfs] %s: save zip cache: %v (non-fatal)", agencyID, err)
	}

	log.Printf("[gtfs] %s: persisting %d stop_times, %d services, %d exceptions",
		agencyID, len(td.stopTimes), len(td.services), len(td.exceptions))
	if err := db.ReplaceGTFSAgency(agencyID, td.stopTimes, td.services, td.exceptions); err != nil {
		log.Printf("[gtfs] %s: persist timetable: %v (non-fatal)", agencyID, err)
	}

	return nil
}

// tryDownload fetches all URLs and returns the raw bytes. Returns the first
// error encountered; any already-downloaded bytes are discarded on failure.
func tryDownload(urls []string) ([][]byte, error) {
	blobs := make([][]byte, 0, len(urls))
	for _, url := range urls {
		data, err := downloadZip(url)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", url, err)
		}
		blobs = append(blobs, data)
	}
	return blobs, nil
}

// parseFromBlobsFull is like parseFromBlobs but collects timetable rows
// (stop_times, services, exceptions) in the same pass through the ZIP files,
// eliminating the need for a second decompression of stop_times.txt.
func (s *Schedule) parseFromBlobsFull(agencyID string, blobs [][]byte) (*timetableData, error) {
	routes := make(map[string]Route)
	stops := make(map[string]Stop)
	trips := make(map[string]Trip)
	routeStops := make(map[string][]string)
	shapes := make(map[string][]ShapePoint)
	td := &timetableData{}

	for _, data := range blobs {
		if err := mergeZipBytes(data, routes, stops, trips, routeStops, shapes, td); err != nil {
			return nil, fmt.Errorf("%s: %w", agencyID, err)
		}
	}

	for id, pts := range shapes {
		sort.Slice(pts, func(i, j int) bool { return pts[i].Sequence < pts[j].Sequence })
		shapes[id] = pts
	}
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
	return td, nil
}

// parseDepartureSecs converts a GTFS departure_time string ("HH:MM:SS", where
// HH may be ≥ 24 for overnight trips) to seconds since midnight.
func parseDepartureSecs(s string) (int, bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return h*3600 + m*60 + sec, true
}

// buildWeekdayMask converts calendar.txt day columns to a bitmask aligned with
// the store.WeekdaySun..WeekdaySat constants (1 << int(time.Weekday)).
func buildWeekdayMask(row []string, h map[string]int) int {
	mask := 0
	if col(row, h, "sunday") == "1" {
		mask |= store.WeekdaySun
	}
	if col(row, h, "monday") == "1" {
		mask |= store.WeekdayMon
	}
	if col(row, h, "tuesday") == "1" {
		mask |= store.WeekdayTue
	}
	if col(row, h, "wednesday") == "1" {
		mask |= store.WeekdayWed
	}
	if col(row, h, "thursday") == "1" {
		mask |= store.WeekdayThu
	}
	if col(row, h, "friday") == "1" {
		mask |= store.WeekdayFri
	}
	if col(row, h, "saturday") == "1" {
		mask |= store.WeekdaySat
	}
	return mask
}
