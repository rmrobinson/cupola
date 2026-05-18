package store

import "database/sql"

func (s *GTFSSQLiteStore) loadRoutes(agencyID string) ([]GTFSRoute, error) {
	rows, err := s.db.Query(`
		SELECT route_id, route_agency_id, short_name, long_name, route_type, color
		FROM gtfs_routes
		WHERE agency_id = ?
		ORDER BY route_id
	`, agencyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GTFSRoute
	for rows.Next() {
		var r GTFSRoute
		if err := rows.Scan(&r.ID, &r.AgencyID, &r.ShortName, &r.LongName, &r.Type, &r.Color); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *GTFSSQLiteStore) loadStops(agencyID string) ([]GTFSStop, error) {
	rows, err := s.db.Query(`
		SELECT stop_id, code, name, lat, lon
		FROM gtfs_stops
		WHERE agency_id = ?
		ORDER BY stop_id
	`, agencyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GTFSStop
	for rows.Next() {
		var st GTFSStop
		if err := rows.Scan(&st.ID, &st.Code, &st.Name, &st.Lat, &st.Lon); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *GTFSSQLiteStore) loadTrips(agencyID string) ([]GTFSTrip, error) {
	rows, err := s.db.Query(`
		SELECT trip_id, route_id, headsign, shape_id, service_id
		FROM gtfs_trips
		WHERE agency_id = ?
		ORDER BY trip_id
	`, agencyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GTFSTrip
	for rows.Next() {
		var tr GTFSTrip
		if err := rows.Scan(&tr.ID, &tr.RouteID, &tr.Headsign, &tr.ShapeID, &tr.ServiceID); err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}

func (s *GTFSSQLiteStore) loadRouteStops(agencyID string) ([]GTFSRouteStop, error) {
	rows, err := s.db.Query(`
		SELECT route_id, stop_id, ordinal
		FROM gtfs_route_stops
		WHERE agency_id = ?
		ORDER BY route_id, ordinal
	`, agencyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GTFSRouteStop
	for rows.Next() {
		var rs GTFSRouteStop
		if err := rows.Scan(&rs.RouteID, &rs.StopID, &rs.Ordinal); err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

func (s *GTFSSQLiteStore) loadShapePoints(agencyID string) ([]GTFSShapePoint, error) {
	rows, err := s.db.Query(`
		SELECT shape_id, lat, lon, sequence
		FROM gtfs_shapes
		WHERE agency_id = ?
		ORDER BY shape_id, sequence
	`, agencyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GTFSShapePoint
	for rows.Next() {
		var pt GTFSShapePoint
		if err := rows.Scan(&pt.ShapeID, &pt.Lat, &pt.Lon, &pt.Sequence); err != nil {
			return nil, err
		}
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return out, nil
}
