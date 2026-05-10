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
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Table names are internal constants, not user-supplied, so the concatenation
	// below is safe from injection.
	for _, tbl := range []string{"gtfs_stop_times", "gtfs_services", "gtfs_service_exceptions"} {
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

func (s *GTFSSQLiteStore) DeleteGTFSAgency(agencyID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, tbl := range []string{"gtfs_stop_times", "gtfs_services", "gtfs_service_exceptions"} {
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
