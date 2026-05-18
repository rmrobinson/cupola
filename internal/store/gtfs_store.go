package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// GTFSSQLiteStore persists GTFS timetable data in its own database file
// (gtfs.db), isolated from the main cupola.db. This ensures long-running GTFS
// imports never hold the SQLite write lock long enough to block profile saves.
type GTFSSQLiteStore struct {
	db *sql.DB
}

// NewGTFSSQLiteStore opens (or creates) gtfs.db inside dataDir.
func NewGTFSSQLiteStore(dataDir string) (*GTFSSQLiteStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dbPath, err := filepath.Abs(filepath.Join(dataDir, "gtfs.db"))
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	dbURL := url.URL{Scheme: "file", Path: dbPath}
	q := dbURL.Query()
	q.Add("_pragma", "busy_timeout(30000)")
	q.Add("_pragma", "journal_mode(WAL)")
	dbURL.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", dbURL.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(0)

	s := &GTFSSQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *GTFSSQLiteStore) migrate() error {
	stmts := []string{
		`PRAGMA busy_timeout=30000`,
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE IF NOT EXISTS gtfs_stop_times (
			agency_id      TEXT    NOT NULL,
			route_id       TEXT    NOT NULL,
			trip_id        TEXT    NOT NULL,
			stop_id        TEXT    NOT NULL,
			headsign       TEXT    NOT NULL DEFAULT '',
			service_id     TEXT    NOT NULL,
			stop_sequence  INTEGER NOT NULL,
			departure_secs INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gst_route_stop
			ON gtfs_stop_times (agency_id, route_id, stop_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_gst_unique
			ON gtfs_stop_times (agency_id, trip_id, stop_sequence)`,
		`CREATE TABLE IF NOT EXISTS gtfs_services (
			agency_id    TEXT    NOT NULL,
			service_id   TEXT    NOT NULL,
			weekday_mask INTEGER NOT NULL,
			start_date   TEXT    NOT NULL,
			end_date     TEXT    NOT NULL,
			PRIMARY KEY (agency_id, service_id)
		)`,
		`CREATE TABLE IF NOT EXISTS gtfs_service_exceptions (
			agency_id  TEXT    NOT NULL,
			service_id TEXT    NOT NULL,
			date       TEXT    NOT NULL,
			added      INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_gse_unique
			ON gtfs_service_exceptions (agency_id, service_id, date)`,
		`CREATE INDEX IF NOT EXISTS idx_gse
			ON gtfs_service_exceptions (agency_id, service_id, date)`,
		`CREATE TABLE IF NOT EXISTS gtfs_routes (
			agency_id        TEXT    NOT NULL,
			route_id         TEXT    NOT NULL,
			route_agency_id  TEXT    NOT NULL DEFAULT '',
			short_name       TEXT    NOT NULL DEFAULT '',
			long_name        TEXT    NOT NULL DEFAULT '',
			route_type       INTEGER NOT NULL DEFAULT 0,
			color            TEXT    NOT NULL DEFAULT '',
			PRIMARY KEY (agency_id, route_id)
		)`,
		`CREATE TABLE IF NOT EXISTS gtfs_stops (
			agency_id TEXT NOT NULL,
			stop_id   TEXT NOT NULL,
			code      TEXT NOT NULL DEFAULT '',
			name      TEXT NOT NULL DEFAULT '',
			lat       REAL NOT NULL DEFAULT 0,
			lon       REAL NOT NULL DEFAULT 0,
			PRIMARY KEY (agency_id, stop_id)
		)`,
		`CREATE TABLE IF NOT EXISTS gtfs_trips (
			agency_id TEXT NOT NULL,
			trip_id   TEXT NOT NULL,
			route_id  TEXT NOT NULL DEFAULT '',
			headsign  TEXT NOT NULL DEFAULT '',
			shape_id  TEXT NOT NULL DEFAULT '',
			service_id TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (agency_id, trip_id)
		)`,
		`CREATE TABLE IF NOT EXISTS gtfs_route_stops (
			agency_id TEXT    NOT NULL,
			route_id  TEXT    NOT NULL,
			stop_id   TEXT    NOT NULL,
			ordinal   INTEGER NOT NULL,
			PRIMARY KEY (agency_id, route_id, stop_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_grs_route
			ON gtfs_route_stops (agency_id, route_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS gtfs_shapes (
			agency_id TEXT    NOT NULL,
			shape_id  TEXT    NOT NULL,
			lat       REAL    NOT NULL,
			lon       REAL    NOT NULL,
			sequence  INTEGER NOT NULL,
			PRIMARY KEY (agency_id, shape_id, sequence)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gsh_shape
			ON gtfs_shapes (agency_id, shape_id, sequence)`,
		`CREATE TABLE IF NOT EXISTS gtfs_cache_meta (
			agency_id TEXT PRIMARY KEY NOT NULL,
			loaded_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(40, len(stmt))], err)
		}
	}
	return nil
}
