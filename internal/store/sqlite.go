package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
	_ "modernc.org/sqlite"
)

// SQLiteStore persists profiles, notes, and cached GTFS timetable data. It is
// not used for sensor or alert time-series data.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) the SQLite database inside dataDir and
// runs schema migrations.
func NewSQLiteStore(dataDir string) (*SQLiteStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dbPath, err := filepath.Abs(filepath.Join(dataDir, "cupola.db"))
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	dbURL := url.URL{Scheme: "file", Path: dbPath}
	q := dbURL.Query()
	q.Add("_pragma", "busy_timeout(30000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	dbURL.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", dbURL.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL allows concurrent readers while a writer holds the write lock.
	// No connection cap so profile/notes reads proceed during large GTFS transactions.
	db.SetMaxOpenConns(0)

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	stmts := []string{
		`PRAGMA busy_timeout=30000`,
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS profiles (
			id          TEXT PRIMARY KEY NOT NULL,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			layout      TEXT NOT NULL DEFAULT 'landscape',
			data        TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS notes (
			id         TEXT PRIMARY KEY NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			body       TEXT NOT NULL DEFAULT '',
			author     TEXT NOT NULL DEFAULT '',
			pinned     INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS transit_agencies (
			id                             TEXT PRIMARY KEY NOT NULL,
			enabled                        INTEGER NOT NULL DEFAULT 1,
			gtfs_static_urls               TEXT NOT NULL,
			gtfs_rt_trip_updates_urls      TEXT NOT NULL,
			gtfs_rt_vehicle_positions_urls TEXT NOT NULL DEFAULT '[]',
			gtfs_rt_alerts_url             TEXT NOT NULL DEFAULT '',
			created_at                     TEXT NOT NULL,
			updated_at                     TEXT NOT NULL
		)`,

	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(40, len(stmt))], err)
		}
	}
	return nil
}

// ── Profiles ─────────────────────────────────────────────────────────────────

func (s *SQLiteStore) ListProfiles() ([]ProfileMeta, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, layout FROM profiles ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProfileMeta
	for rows.Next() {
		var p ProfileMeta
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Layout); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []ProfileMeta{}
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetProfile(id string) (*Profile, error) {
	var data string
	err := s.db.QueryRow(
		`SELECT data FROM profiles WHERE id = ?`, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SQLiteStore) UpsertProfile(p *Profile) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO profiles (id, name, description, layout, data)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name        = excluded.name,
			description = excluded.description,
			layout      = excluded.layout,
			data        = excluded.data
	`, p.ID, p.Name, p.Description, p.Layout, string(data))
	return err
}

func (s *SQLiteStore) DeleteProfile(id string) error {
	_, err := s.db.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	return err
}

// ── Notes ─────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) ListNotes() ([]domain.Note, error) {
	rows, err := s.db.Query(`
		SELECT id, title, body, author, pinned, created_at, updated_at
		FROM notes
		ORDER BY pinned DESC, updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if out == nil {
		out = []domain.Note{}
	}
	return out, rows.Err()
}

func (s *SQLiteStore) CreateNote(n domain.Note) error {
	_, err := s.db.Exec(`
		INSERT INTO notes (id, title, body, author, pinned, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, n.ID, n.Title, n.Body, n.Author, boolInt(n.Pinned),
		n.CreatedAt.UTC().Format(time.RFC3339),
		n.UpdatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (s *SQLiteStore) UpdateNote(id string, u NoteUpdate) (*domain.Note, error) {
	row := s.db.QueryRow(`
		SELECT id, title, body, author, pinned, created_at, updated_at
		FROM notes WHERE id = ?
	`, id)
	n, err := scanNote(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if u.Title != nil {
		n.Title = *u.Title
	}
	if u.Body != nil {
		n.Body = *u.Body
	}
	if u.Author != nil {
		n.Author = *u.Author
	}
	if u.Pinned != nil {
		n.Pinned = *u.Pinned
	}
	n.UpdatedAt = time.Now().UTC()

	_, err = s.db.Exec(`
		UPDATE notes
		SET title=?, body=?, author=?, pinned=?, updated_at=?
		WHERE id=?
	`, n.Title, n.Body, n.Author, boolInt(n.Pinned),
		n.UpdatedAt.Format(time.RFC3339), id,
	)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *SQLiteStore) DeleteNote(id string) error {
	_, err := s.db.Exec(`DELETE FROM notes WHERE id = ?`, id)
	return err
}

// ── helpers ───────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanNote(s scanner) (domain.Note, error) {
	var n domain.Note
	var pinned int
	var createdAt, updatedAt string

	if err := s.Scan(
		&n.ID, &n.Title, &n.Body, &n.Author,
		&pinned, &createdAt, &updatedAt,
	); err != nil {
		return domain.Note{}, err
	}

	n.Pinned = pinned != 0

	var err error
	if n.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
		return domain.Note{}, fmt.Errorf("parse created_at: %w", err)
	}
	if n.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt); err != nil {
		return domain.Note{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return n, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
