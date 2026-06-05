package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
)

const (
	regionWaterlooClosuresURL      = "https://gis.regionofwaterloo.ca/wamap/rest/services/RegionalClosures/MapServer"
	regionWaterlooCurrentLayerID   = "0"
	regionWaterlooAlertSourceID    = "region.waterloo.roadclosures"
	regionWaterlooIncidentIDPrefix = "region-waterloo-roadclosures:"
)

type RegionWaterlooClosuresSource struct {
	baseURL string
	alerts  []domain.MunicipalAlert
}

func NewRegionWaterlooClosuresSource() *RegionWaterlooClosuresSource {
	return &RegionWaterlooClosuresSource{baseURL: regionWaterlooClosuresURL}
}

func NewRegionWaterlooClosuresSourceWithURL(rawURL string) *RegionWaterlooClosuresSource {
	baseURL := strings.TrimSpace(rawURL)
	if baseURL == "" {
		baseURL = regionWaterlooClosuresURL
	}
	return &RegionWaterlooClosuresSource{baseURL: baseURL}
}

func (s *RegionWaterlooClosuresSource) ID() string { return regionWaterlooAlertSourceID }

func (s *RegionWaterlooClosuresSource) Fetch(ctx context.Context) ([]domain.TrafficIncident, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, regionWaterlooQueryURL(s.baseURL, regionWaterlooCurrentLayerID), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get regional closures: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 512)) //nolint:errcheck
		return nil, fmt.Errorf("get regional closures: status %d", resp.StatusCode)
	}
	incidents, alerts, err := parseRegionWaterlooClosures(resp.Body, s.baseURL)
	if err != nil {
		return nil, err
	}
	s.alerts = alerts
	return incidents, nil
}

func (s *RegionWaterlooClosuresSource) PromotedMunicipalAlerts() []domain.MunicipalAlert {
	return append([]domain.MunicipalAlert(nil), s.alerts...)
}

func regionWaterlooQueryURL(baseURL, layerID string) string {
	u := strings.TrimRight(baseURL, "/") + "/" + layerID + "/query"
	v := url.Values{}
	v.Set("where", "1=1")
	v.Set("outFields", "*")
	v.Set("returnGeometry", "true")
	v.Set("f", "geojson")
	return u + "?" + v.Encode()
}

type regionWaterlooFeatureCollection struct {
	Features []regionWaterlooFeature `json:"features"`
}

type regionWaterlooFeature struct {
	ID         any                         `json:"id"`
	Geometry   regionWaterlooGeometry      `json:"geometry"`
	Properties regionWaterlooClosureFields `json:"properties"`
}

type regionWaterlooGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type regionWaterlooClosureFields struct {
	ObjectID         int     `json:"OBJECTID"`
	GlobalID         string  `json:"GlobalID"`
	ClosureType      string  `json:"CLOSURE_TYPE"`
	StreetName       string  `json:"STREET_NAME"`
	StreetFrom       string  `json:"STREET_FROM"`
	StreetTo         string  `json:"STREET_TO"`
	Municipality     string  `json:"MUNICIPALITY"`
	DateFrom         *int64  `json:"DATE_FROM"`
	DateTo           *int64  `json:"DATE_TO"`
	Reason           string  `json:"REASON"`
	Status           string  `json:"STATUS"`
	ClosureScheduled string  `json:"CLOSURE_SCHEDULED"`
	Details          string  `json:"DETAILS"`
	Detour           string  `json:"DETOUR"`
	Organization     string  `json:"ORGANIZATION"`
	Contact          string  `json:"CONTACT"`
	OpenStatus       string  `json:"OPEN_STATUS"`
	Description      string  `json:"Description"`
	EmergencyReason  *string `json:"EMERGENCY_REASON"`
	CreateDate       *int64  `json:"Create_Date"`
}

func parseRegionWaterlooClosures(r io.Reader, sourceURL string) ([]domain.TrafficIncident, []domain.MunicipalAlert, error) {
	var fc regionWaterlooFeatureCollection
	if err := json.NewDecoder(r).Decode(&fc); err != nil {
		return nil, nil, fmt.Errorf("decode regional closures: %w", err)
	}

	incidents := make([]domain.TrafficIncident, 0, len(fc.Features))
	var alerts []domain.MunicipalAlert
	for _, feature := range fc.Features {
		props := feature.Properties
		if skipRegionWaterlooClosure(props) {
			continue
		}
		lines, err := parseRegionWaterlooLines(feature.Geometry)
		if err != nil {
			return nil, nil, fmt.Errorf("parse regional closure %s geometry: %w", props.stableID(), err)
		}
		if len(lines) == 0 {
			continue
		}
		startsAt := msTime(props.DateFrom)
		endsAt := msTime(props.DateTo)
		lat, lon := firstLinePoint(lines)
		id := regionWaterlooIncidentIDPrefix + props.stableID()
		roadName := props.roadName()
		description := props.description()
		severity := props.severity()

		incidents = append(incidents, domain.TrafficIncident{
			ID:          id,
			Type:        "closure",
			Severity:    severity,
			Lat:         lat,
			Lon:         lon,
			Description: description,
			RoadName:    roadName,
			Lines:       lines,
			StartsAt:    startsAt,
			EndsAt:      endsAt,
			SourceURL:   sourceURL,
		})

		if props.isEmergency() {
			area := roadName
			alerts = append(alerts, domain.MunicipalAlert{
				ID:          regionWaterlooAlertSourceID + ":" + props.stableID(),
				SourceID:    regionWaterlooAlertSourceID,
				Title:       roadName + " road closure",
				Description: description,
				AlertType:   "road-closure",
				Severity:    domain.SeverityWarning,
				Area:        &area,
				StartsAt:    startsAt,
				EndsAt:      endsAt,
				URL:         &sourceURL,
				PublishedAt: publishedAtFromRegionDates(props.CreateDate, props.DateFrom),
			})
		}
	}
	incidents, _ = dedupeTrafficIncidents(incidents)
	alerts, _ = dedupeRegionWaterlooAlertsByID(alerts)
	return incidents, alerts, nil
}

func dedupeRegionWaterlooAlertsByID(alerts []domain.MunicipalAlert) ([]domain.MunicipalAlert, int) {
	if len(alerts) < 2 {
		return alerts, 0
	}
	seen := make(map[string]struct{}, len(alerts))
	deduped := make([]domain.MunicipalAlert, 0, len(alerts))
	var duplicates int
	for _, alert := range alerts {
		if alert.ID == "" {
			deduped = append(deduped, alert)
			continue
		}
		if _, ok := seen[alert.ID]; ok {
			duplicates++
			continue
		}
		seen[alert.ID] = struct{}{}
		deduped = append(deduped, alert)
	}
	if duplicates == 0 {
		return alerts, 0
	}
	return deduped, duplicates
}

func skipRegionWaterlooClosure(p regionWaterlooClosureFields) bool {
	if strings.EqualFold(strings.TrimSpace(p.Status), "No Closure") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(p.OpenStatus)) {
	case "road reopened", "postponed", "cancelled":
		return true
	default:
		return false
	}
}

func parseRegionWaterlooLines(g regionWaterlooGeometry) ([][][]float64, error) {
	switch g.Type {
	case "LineString":
		var line [][]float64
		if err := json.Unmarshal(g.Coordinates, &line); err != nil {
			return nil, err
		}
		return normalizeRegionWaterlooLines([][][]float64{line}), nil
	case "MultiLineString":
		var lines [][][]float64
		if err := json.Unmarshal(g.Coordinates, &lines); err != nil {
			return nil, err
		}
		return normalizeRegionWaterlooLines(lines), nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported geometry type %q", g.Type)
	}
}

func normalizeRegionWaterlooLines(lines [][][]float64) [][][]float64 {
	out := make([][][]float64, 0, len(lines))
	for _, line := range lines {
		if len(line) < 1 {
			continue
		}
		clean := make([][]float64, 0, len(line))
		for _, pt := range line {
			if len(pt) < 2 {
				continue
			}
			clean = append(clean, []float64{pt[0], pt[1]})
		}
		if len(clean) > 0 {
			out = append(out, clean)
		}
	}
	return out
}

func firstLinePoint(lines [][][]float64) (lat, lon float64) {
	if len(lines) == 0 || len(lines[0]) == 0 || len(lines[0][0]) < 2 {
		return 0, 0
	}
	return lines[0][0][1], lines[0][0][0]
}

func msTime(ms *int64) *time.Time {
	if ms == nil || *ms <= 0 {
		return nil
	}
	t := time.UnixMilli(*ms).UTC()
	return &t
}

func publishedAtFromRegionDates(createMS, dateFromMS *int64) time.Time {
	if t := msTime(createMS); t != nil {
		return *t
	}
	if t := msTime(dateFromMS); t != nil {
		return *t
	}
	return time.Time{}
}

func (p regionWaterlooClosureFields) stableID() string {
	id := strings.TrimSpace(strings.Trim(p.GlobalID, "{}"))
	if id != "" {
		return strings.ToLower(id)
	}
	if p.ObjectID != 0 {
		return fmt.Sprintf("%d", p.ObjectID)
	}
	return regionWaterlooStableID(p.StreetName, p.StreetFrom, p.StreetTo, p.Reason, p.Status)
}

func (p regionWaterlooClosureFields) roadName() string {
	if v := normalizeRegionText(p.Description); v != "" {
		return v
	}
	street := normalizeRegionText(p.StreetName)
	from := normalizeRegionText(p.StreetFrom)
	to := normalizeRegionText(p.StreetTo)
	switch {
	case street != "" && from != "" && to != "":
		return street + " from " + from + " to " + to
	case street != "":
		return street
	default:
		return "Regional road closure"
	}
}

func (p regionWaterlooClosureFields) description() string {
	parts := []string{
		joinLabelValue("Status", p.Status),
		joinLabelValue("Reason", p.reasonText()),
		normalizeRegionText(p.Details),
		joinLabelValue("Detour", p.Detour),
		joinLabelValue("Municipality", p.Municipality),
		joinLabelValue("Organization", p.Organization),
		joinLabelValue("Contact", p.Contact),
	}
	return strings.Join(nonEmpty(parts), ". ")
}

func (p regionWaterlooClosureFields) reasonText() string {
	reason := normalizeRegionText(p.Reason)
	if p.EmergencyReason != nil && normalizeRegionText(*p.EmergencyReason) != "" {
		if reason != "" {
			return reason + ": " + normalizeRegionText(*p.EmergencyReason)
		}
		return normalizeRegionText(*p.EmergencyReason)
	}
	return reason
}

func (p regionWaterlooClosureFields) severity() string {
	if p.isEmergency() || strings.EqualFold(strings.TrimSpace(p.Status), "Closed") {
		return "major"
	}
	switch strings.ToLower(strings.TrimSpace(p.Status)) {
	case "partially closed", "lane reduced", "local access only", "no through traffic":
		return "moderate"
	default:
		return "minor"
	}
}

func (p regionWaterlooClosureFields) isEmergency() bool {
	if strings.EqualFold(strings.TrimSpace(p.Reason), "Emergency") ||
		strings.EqualFold(strings.TrimSpace(p.ClosureScheduled), "Emergency") {
		return true
	}
	return p.EmergencyReason != nil && normalizeRegionText(*p.EmergencyReason) != ""
}

func joinLabelValue(label, value string) string {
	value = normalizeRegionText(value)
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func normalizeRegionText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

func regionWaterlooStableID(parts ...string) string {
	h := fnv.New64a()
	for _, part := range parts {
		io.WriteString(h, strings.ToLower(strings.TrimSpace(part))) //nolint:errcheck
		io.WriteString(h, "\x00")                                   //nolint:errcheck
	}
	return fmt.Sprintf("%x", h.Sum64())
}
