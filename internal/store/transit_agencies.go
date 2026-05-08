package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type TransitAgencyConfig struct {
	ID                         string    `json:"id"`
	Enabled                    bool      `json:"enabled"`
	GTFSStaticURLs             []string  `json:"gtfs_static_urls"`
	GTFSRTTripUpdatesURLs      []string  `json:"gtfs_rt_trip_updates_urls"`
	GTFSRTVehiclePositionsURLs []string  `json:"gtfs_rt_vehicle_positions_urls"`
	GTFSRTAlertsURL            string    `json:"gtfs_rt_alerts_url"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

func (s *SQLiteStore) ListTransitAgencies() ([]TransitAgencyConfig, error) {
	rows, err := s.db.Query(`
		SELECT id, enabled, gtfs_static_urls, gtfs_rt_trip_updates_urls,
		       gtfs_rt_vehicle_positions_urls, gtfs_rt_alerts_url, created_at, updated_at
		FROM transit_agencies
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TransitAgencyConfig
	for rows.Next() {
		cfg, err := scanTransitAgency(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *cfg)
	}
	if out == nil {
		out = []TransitAgencyConfig{}
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetTransitAgency(id string) (*TransitAgencyConfig, error) {
	row := s.db.QueryRow(`
		SELECT id, enabled, gtfs_static_urls, gtfs_rt_trip_updates_urls,
		       gtfs_rt_vehicle_positions_urls, gtfs_rt_alerts_url, created_at, updated_at
		FROM transit_agencies
		WHERE id = ?
	`, id)
	cfg, err := scanTransitAgency(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return cfg, err
}

func (s *SQLiteStore) CreateTransitAgency(cfg TransitAgencyConfig) error {
	now := time.Now().UTC()
	staticURLs, tripURLs, vehicleURLs, err := marshalTransitAgencyURLs(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO transit_agencies
			(id, enabled, gtfs_static_urls, gtfs_rt_trip_updates_urls,
			 gtfs_rt_vehicle_positions_urls, gtfs_rt_alerts_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, cfg.ID, boolInt(cfg.Enabled), staticURLs, tripURLs, vehicleURLs, cfg.GTFSRTAlertsURL, now.Format(time.RFC3339), now.Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) UpdateTransitAgency(cfg TransitAgencyConfig) error {
	staticURLs, tripURLs, vehicleURLs, err := marshalTransitAgencyURLs(cfg)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`
		UPDATE transit_agencies
		SET enabled = ?,
		    gtfs_static_urls = ?,
		    gtfs_rt_trip_updates_urls = ?,
		    gtfs_rt_vehicle_positions_urls = ?,
		    gtfs_rt_alerts_url = ?,
		    updated_at = ?
		WHERE id = ?
	`, boolInt(cfg.Enabled), staticURLs, tripURLs, vehicleURLs, cfg.GTFSRTAlertsURL, time.Now().UTC().Format(time.RFC3339), cfg.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLiteStore) DeleteTransitAgency(id string) error {
	_, err := s.db.Exec(`DELETE FROM transit_agencies WHERE id = ?`, id)
	return err
}

func marshalTransitAgencyURLs(cfg TransitAgencyConfig) (string, string, string, error) {
	staticURLs, err := json.Marshal(nonNilStrings(cfg.GTFSStaticURLs))
	if err != nil {
		return "", "", "", fmt.Errorf("marshal static urls: %w", err)
	}
	tripURLs, err := json.Marshal(nonNilStrings(cfg.GTFSRTTripUpdatesURLs))
	if err != nil {
		return "", "", "", fmt.Errorf("marshal trip updates urls: %w", err)
	}
	vehicleURLs, err := json.Marshal(nonNilStrings(cfg.GTFSRTVehiclePositionsURLs))
	if err != nil {
		return "", "", "", fmt.Errorf("marshal vehicle positions urls: %w", err)
	}
	return string(staticURLs), string(tripURLs), string(vehicleURLs), nil
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

type transitAgencyScanner interface {
	Scan(dest ...any) error
}

func scanTransitAgency(row transitAgencyScanner) (*TransitAgencyConfig, error) {
	var cfg TransitAgencyConfig
	var enabled int
	var staticURLs, tripURLs, vehicleURLs string
	var createdAt, updatedAt string
	if err := row.Scan(
		&cfg.ID, &enabled, &staticURLs, &tripURLs, &vehicleURLs,
		&cfg.GTFSRTAlertsURL, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	cfg.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(staticURLs), &cfg.GTFSStaticURLs); err != nil {
		return nil, fmt.Errorf("decode static urls for %s: %w", cfg.ID, err)
	}
	if err := json.Unmarshal([]byte(tripURLs), &cfg.GTFSRTTripUpdatesURLs); err != nil {
		return nil, fmt.Errorf("decode trip updates urls for %s: %w", cfg.ID, err)
	}
	if err := json.Unmarshal([]byte(vehicleURLs), &cfg.GTFSRTVehiclePositionsURLs); err != nil {
		return nil, fmt.Errorf("decode vehicle positions urls for %s: %w", cfg.ID, err)
	}
	var err error
	cfg.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at for %s: %w", cfg.ID, err)
	}
	cfg.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at for %s: %w", cfg.ID, err)
	}
	return &cfg, nil
}
