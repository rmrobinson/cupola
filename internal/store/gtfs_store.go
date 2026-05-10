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
		db.Close()
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
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(40, len(stmt))], err)
		}
	}
	return nil
}
