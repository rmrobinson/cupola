package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Weekday bit constants for GTFSService.WeekdayMask.
// Values match 1 << int(time.Weekday) so the query can use (weekday_mask & ?) != 0
// directly with 1 << int(after.Weekday()) as the parameter.
const (
	WeekdaySun = 1 << int(time.Sunday)    // 1
	WeekdayMon = 1 << int(time.Monday)    // 2
	WeekdayTue = 1 << int(time.Tuesday)   // 4
	WeekdayWed = 1 << int(time.Wednesday) // 8
	WeekdayThu = 1 << int(time.Thursday)  // 16
	WeekdayFri = 1 << int(time.Friday)    // 32
	WeekdaySat = 1 << int(time.Saturday)  // 64
)

// GTFSStopTime is one stop_times.txt row, denormalized with route_id, headsign,
// and service_id so that QueryUpcomingDepartures needs no additional joins.
type GTFSStopTime struct {
	RouteID       string
	TripID        string
	StopID        string
	Headsign      string
	ServiceID     string
	StopSequence  int
	DepartureSecs int // seconds from midnight; may exceed 86400 for overnight trips
}

// GTFSService is one calendar.txt row.
type GTFSService struct {
	ServiceID   string
	WeekdayMask int    // bitmask using WeekdaySun..WeekdaySat constants above
	StartDate   string // YYYYMMDD
	EndDate     string // YYYYMMDD
}

// GTFSServiceException is one calendar_dates.txt row.
type GTFSServiceException struct {
	ServiceID string
	Date      string // YYYYMMDD
	Added     bool   // true = exception_type 1 (added); false = exception_type 2 (removed)
}

type GTFSRoute struct {
	ID        string
	AgencyID  string
	ShortName string
	LongName  string
	Type      int
	Color     string
}

type GTFSStop struct {
	ID   string
	Code string
	Name string
	Lat  float64
	Lon  float64
}

type GTFSTrip struct {
	ID        string
	RouteID   string
	Headsign  string
	ShapeID   string
	ServiceID string
}

type GTFSRouteStop struct {
	RouteID string
	StopID  string
	Ordinal int
}

type GTFSShapePoint struct {
	ShapeID  string
	Lat      float64
	Lon      float64
	Sequence int
}

type GTFSMetadata struct {
	Routes     []GTFSRoute
	Stops      []GTFSStop
	Trips      []GTFSTrip
	RouteStops []GTFSRouteStop
	Shapes     []GTFSShapePoint
}

// ScheduledDeparture is returned by QueryUpcomingDepartures.
type ScheduledDeparture struct {
	TripID        string
	Headsign      string
	DepartureTime time.Time
}

// ReplaceGTFSAgency atomically replaces all GTFS timetable data for agencyID.
// Existing rows for the agency are deleted before new rows are inserted.
func (s *GTFSSQLiteStore) ReplaceGTFSAgency(
	agencyID string,
	stopTimes []GTFSStopTime,
	services []GTFSService,
	exceptions []GTFSServiceException,
	metadata *GTFSMetadata,
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Table names are internal constants, not user-supplied, so the concatenation
	// below is safe from injection.
	for _, tbl := range gtfsAgencyTables {
		if _, err := tx.Exec("DELETE FROM "+tbl+" WHERE agency_id = ?", agencyID); err != nil {
			return fmt.Errorf("clear %s: %w", tbl, err)
		}
	}

	if err := batchInsertStopTimes(tx, agencyID, stopTimes); err != nil {
		return fmt.Errorf("insert stop_times: %w", err)
	}
	if err := batchInsertServices(tx, agencyID, services); err != nil {
		return fmt.Errorf("insert services: %w", err)
	}
	if err := batchInsertExceptions(tx, agencyID, exceptions); err != nil {
		return fmt.Errorf("insert exceptions: %w", err)
	}
	if metadata != nil {
		if err := batchInsertRoutes(tx, agencyID, metadata.Routes); err != nil {
			return fmt.Errorf("insert routes: %w", err)
		}
		if err := batchInsertStops(tx, agencyID, metadata.Stops); err != nil {
			return fmt.Errorf("insert stops: %w", err)
		}
		if err := batchInsertTrips(tx, agencyID, metadata.Trips); err != nil {
			return fmt.Errorf("insert trips: %w", err)
		}
		if err := batchInsertRouteStops(tx, agencyID, metadata.RouteStops); err != nil {
			return fmt.Errorf("insert route stops: %w", err)
		}
		if err := batchInsertShapePoints(tx, agencyID, metadata.Shapes); err != nil {
			return fmt.Errorf("insert shapes: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO gtfs_cache_meta (agency_id, loaded_at) VALUES (?, ?)`,
			agencyID, time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("insert cache meta: %w", err)
		}
	}

	return tx.Commit()
}

// QueryUpcomingDepartures returns up to limit scheduled departures for the given
// agency/route/stop combination, ordered by departure time, occurring after the
// after timestamp. The result uses only the static schedule; no real-time data.
//
// Overnight trips with departure_secs > 86400 are not returned when querying
// during normal daytime hours — they will appear correctly when after is past
// midnight on the following day.
func (s *GTFSSQLiteStore) QueryUpcomingDepartures(
	agencyID, routeID, stopID string,
	after time.Time,
	limit int,
) ([]ScheduledDeparture, error) {
	secsAfterMidnight := after.Hour()*3600 + after.Minute()*60 + after.Second()
	weekdayBit := 1 << int(after.Weekday())
	dateStr := after.Format("20060102")

	rows, err := s.db.Query(`
		SELECT st.trip_id, st.headsign, st.departure_secs
		FROM gtfs_stop_times st
		LEFT JOIN gtfs_services sv
		  ON sv.agency_id = st.agency_id AND sv.service_id = st.service_id
		WHERE st.agency_id = ?
		  AND st.route_id  = ?
		  AND st.stop_id   = ?
		  AND st.departure_secs > ?
		  AND (
		    (
		      sv.service_id IS NOT NULL
		      AND (sv.weekday_mask & ?) != 0
		      AND sv.start_date <= ?
		      AND sv.end_date   >= ?
		      AND NOT EXISTS (
		        SELECT 1 FROM gtfs_service_exceptions e
		        WHERE e.agency_id  = st.agency_id
		          AND e.service_id = st.service_id
		          AND e.date       = ?
		          AND e.added      = 0
		      )
		    )
		    OR EXISTS (
		      SELECT 1 FROM gtfs_service_exceptions e
		      WHERE e.agency_id  = st.agency_id
		        AND e.service_id = st.service_id
		        AND e.date       = ?
		        AND e.added      = 1
		    )
		  )
		ORDER BY st.departure_secs
		LIMIT ?
	`,
		agencyID, routeID, stopID, secsAfterMidnight,
		weekdayBit, dateStr, dateStr,
		dateStr,
		dateStr,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	midnight := time.Date(after.Year(), after.Month(), after.Day(), 0, 0, 0, 0, after.Location())

	var out []ScheduledDeparture
	for rows.Next() {
		var d ScheduledDeparture
		var depSecs int
		if err := rows.Scan(&d.TripID, &d.Headsign, &depSecs); err != nil {
			return nil, err
		}
		d.DepartureTime = midnight.Add(time.Duration(depSecs) * time.Second)
		out = append(out, d)
	}
	return out, rows.Err()
}

// HasGTFSData reports whether any stop_times rows exist for agencyID.
// Used at startup to decide whether SQLite can serve the static fallback
// without repopulating from the ZIP cache.
func (s *GTFSSQLiteStore) HasGTFSData(agencyID string) (bool, error) {
	var exists int
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM gtfs_stop_times WHERE agency_id = ? LIMIT 1)`, agencyID,
	).Scan(&exists)
	return exists == 1, err
}

func (s *GTFSSQLiteStore) HasGTFSMetadata(agencyID string) (bool, error) {
	var exists int
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM gtfs_routes WHERE agency_id = ? LIMIT 1)`, agencyID,
	).Scan(&exists)
	return exists == 1, err
}

func (s *GTFSSQLiteStore) HasActiveService(agencyID string, when time.Time) (bool, error) {
	dateStr := when.Format("20060102")
	weekdayBit := 1 << int(when.Weekday())
	var exists int
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM gtfs_services sv
			WHERE sv.agency_id = ?
			  AND (
			    (
			      (sv.weekday_mask & ?) != 0
			      AND sv.start_date <= ?
			      AND sv.end_date >= ?
			      AND NOT EXISTS (
			        SELECT 1 FROM gtfs_service_exceptions e
			        WHERE e.agency_id = sv.agency_id
			          AND e.service_id = sv.service_id
			          AND e.date = ?
			          AND e.added = 0
			      )
			    )
			    OR EXISTS (
			      SELECT 1 FROM gtfs_service_exceptions e
			      WHERE e.agency_id = sv.agency_id
			        AND e.service_id = sv.service_id
			        AND e.date = ?
			        AND e.added = 1
			    )
			  )
			LIMIT 1
		)
		OR EXISTS(
			SELECT 1
			FROM gtfs_service_exceptions e
			WHERE e.agency_id = ?
			  AND e.date = ?
			  AND e.added = 1
			LIMIT 1
		)
	`, agencyID, weekdayBit, dateStr, dateStr, dateStr, dateStr, agencyID, dateStr).Scan(&exists)
	return exists == 1, err
}

func (s *GTFSSQLiteStore) LoadGTFSMetadata(agencyID string) (*GTFSMetadata, error) {
	routes, err := s.loadRoutes(agencyID)
	if err != nil {
		return nil, err
	}
	stops, err := s.loadStops(agencyID)
	if err != nil {
		return nil, err
	}
	trips, err := s.loadTrips(agencyID)
	if err != nil {
		return nil, err
	}
	routeStops, err := s.loadRouteStops(agencyID)
	if err != nil {
		return nil, err
	}
	shapes, err := s.loadShapePoints(agencyID)
	if err != nil {
		return nil, err
	}
	return &GTFSMetadata{
		Routes:     routes,
		Stops:      stops,
		Trips:      trips,
		RouteStops: routeStops,
		Shapes:     shapes,
	}, nil
}

func (s *GTFSSQLiteStore) DeleteGTFSAgency(agencyID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, tbl := range gtfsAgencyTables {
		if _, err := tx.Exec("DELETE FROM "+tbl+" WHERE agency_id = ?", agencyID); err != nil {
			return fmt.Errorf("clear %s: %w", tbl, err)
		}
	}
	return tx.Commit()
}

// gtfsBatchSize is the number of rows per multi-row INSERT. Each table's column
// count × gtfsBatchSize must stay under SQLite's default variable limit (999).
// stop_times has 8 columns → 8×100 = 800 params, safely under the limit.
const gtfsBatchSize = 100

var gtfsAgencyTables = []string{
	"gtfs_stop_times",
	"gtfs_services",
	"gtfs_service_exceptions",
	"gtfs_routes",
	"gtfs_stops",
	"gtfs_trips",
	"gtfs_route_stops",
	"gtfs_shapes",
	"gtfs_cache_meta",
}

func batchInsertStopTimes(tx *sql.Tx, agencyID string, rows []GTFSStopTime) error {
	for i := 0; i < len(rows); i += gtfsBatchSize {
		batch := rows[i:min(i+gtfsBatchSize, len(rows))]
		var sb strings.Builder
		sb.WriteString("INSERT OR IGNORE INTO gtfs_stop_times (agency_id,route_id,trip_id,stop_id,headsign,service_id,stop_sequence,departure_secs) VALUES ")
		args := make([]any, 0, len(batch)*8)
		for j, st := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?,?,?,?)")
			args = append(args, agencyID, st.RouteID, st.TripID, st.StopID, st.Headsign, st.ServiceID, st.StopSequence, st.DepartureSecs)
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func batchInsertServices(tx *sql.Tx, agencyID string, rows []GTFSService) error {
	for i := 0; i < len(rows); i += gtfsBatchSize {
		batch := rows[i:min(i+gtfsBatchSize, len(rows))]
		var sb strings.Builder
		sb.WriteString("INSERT OR IGNORE INTO gtfs_services (agency_id,service_id,weekday_mask,start_date,end_date) VALUES ")
		args := make([]any, 0, len(batch)*5)
		for j, svc := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?)")
			args = append(args, agencyID, svc.ServiceID, svc.WeekdayMask, svc.StartDate, svc.EndDate)
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func batchInsertExceptions(tx *sql.Tx, agencyID string, rows []GTFSServiceException) error {
	for i := 0; i < len(rows); i += gtfsBatchSize {
		batch := rows[i:min(i+gtfsBatchSize, len(rows))]
		var sb strings.Builder
		sb.WriteString("INSERT OR IGNORE INTO gtfs_service_exceptions (agency_id,service_id,date,added) VALUES ")
		args := make([]any, 0, len(batch)*4)
		for j, exc := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?)")
			args = append(args, agencyID, exc.ServiceID, exc.Date, boolInt(exc.Added))
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func batchInsertRoutes(tx *sql.Tx, agencyID string, rows []GTFSRoute) error {
	for i := 0; i < len(rows); i += gtfsBatchSize {
		batch := rows[i:min(i+gtfsBatchSize, len(rows))]
		var sb strings.Builder
		sb.WriteString("INSERT OR IGNORE INTO gtfs_routes (agency_id,route_id,route_agency_id,short_name,long_name,route_type,color) VALUES ")
		args := make([]any, 0, len(batch)*7)
		for j, r := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?,?,?)")
			args = append(args, agencyID, r.ID, r.AgencyID, r.ShortName, r.LongName, r.Type, r.Color)
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func batchInsertStops(tx *sql.Tx, agencyID string, rows []GTFSStop) error {
	for i := 0; i < len(rows); i += gtfsBatchSize {
		batch := rows[i:min(i+gtfsBatchSize, len(rows))]
		var sb strings.Builder
		sb.WriteString("INSERT OR IGNORE INTO gtfs_stops (agency_id,stop_id,code,name,lat,lon) VALUES ")
		args := make([]any, 0, len(batch)*6)
		for j, st := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?,?)")
			args = append(args, agencyID, st.ID, st.Code, st.Name, st.Lat, st.Lon)
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func batchInsertTrips(tx *sql.Tx, agencyID string, rows []GTFSTrip) error {
	for i := 0; i < len(rows); i += gtfsBatchSize {
		batch := rows[i:min(i+gtfsBatchSize, len(rows))]
		var sb strings.Builder
		sb.WriteString("INSERT OR IGNORE INTO gtfs_trips (agency_id,trip_id,route_id,headsign,shape_id,service_id) VALUES ")
		args := make([]any, 0, len(batch)*6)
		for j, tr := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?,?)")
			args = append(args, agencyID, tr.ID, tr.RouteID, tr.Headsign, tr.ShapeID, tr.ServiceID)
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func batchInsertRouteStops(tx *sql.Tx, agencyID string, rows []GTFSRouteStop) error {
	for i := 0; i < len(rows); i += gtfsBatchSize {
		batch := rows[i:min(i+gtfsBatchSize, len(rows))]
		var sb strings.Builder
		sb.WriteString("INSERT OR IGNORE INTO gtfs_route_stops (agency_id,route_id,stop_id,ordinal) VALUES ")
		args := make([]any, 0, len(batch)*4)
		for j, rs := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?)")
			args = append(args, agencyID, rs.RouteID, rs.StopID, rs.Ordinal)
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

func batchInsertShapePoints(tx *sql.Tx, agencyID string, rows []GTFSShapePoint) error {
	for i := 0; i < len(rows); i += gtfsBatchSize {
		batch := rows[i:min(i+gtfsBatchSize, len(rows))]
		var sb strings.Builder
		sb.WriteString("INSERT OR IGNORE INTO gtfs_shapes (agency_id,shape_id,lat,lon,sequence) VALUES ")
		args := make([]any, 0, len(batch)*5)
		for j, pt := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?,?)")
			args = append(args, agencyID, pt.ShapeID, pt.Lat, pt.Lon, pt.Sequence)
		}
		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}
