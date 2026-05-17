// Package enovapower parses the Enova Power OMS outage API
// (POST https://oms.enovapower.com/Outages/Home/UpdatePushpin) into
// municipal.alerts.
// Register via import side-effect: _ "github.com/rmrobinson/cupola/internal/collector/municipal/enovapower"
// Config URL should be https://oms.enovapower.com/Outages/ — the parser appends /Home/UpdatePushpin.
package enovapower

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rmrobinson/cupola/internal/collector/municipal"
	"github.com/rmrobinson/cupola/internal/domain"
)

type geocodeCacheEntry struct {
	result   string
	cachedAt time.Time
}

const geocodeTTL = 24 * time.Hour

var (
	geocodeMu    sync.RWMutex
	geocodeCache = make(map[string]geocodeCacheEntry)
)

func init() {
	municipal.RegisterAlertsParser("enova.power", func() municipal.AlertsParser {
		return &Parser{}
	})
}

// Parser implements municipal.AlertsParser for the Enova Power OMS.
type Parser struct{}

func (p *Parser) Parse(rawURL string) ([]domain.MunicipalAlert, error) {
	apiURL := strings.TrimRight(rawURL, "/") + "/Home/UpdatePushpin"

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Post(apiURL, "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("enova.power: post %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enova.power: post %s: status %d", apiURL, resp.StatusCode)
	}

	var dataset omsDataset
	if err := xml.NewDecoder(resp.Body).Decode(&dataset); err != nil {
		return nil, fmt.Errorf("enova.power: decode XML: %w", err)
	}

	return casesToAlerts(client, dataset.Cases), nil
}

// omsDataset is the root XML element returned by the Enova OMS API.
type omsDataset struct {
	Cases []omsCase `xml:"OMSCASES"`
}

type omsCase struct {
	Serial      string `xml:"SERIAL"`
	Planned     string `xml:"PLANNED"`
	OutTime     string `xml:"OUTTIME"`
	InitCust    string `xml:"INITCUST"`
	CurCust     string `xml:"CURCUST"`
	RestoreTime string `xml:"RESTORETIM"`
	RestRange   string `xml:"RESTRANGE"`
	DescCause   string `xml:"DESC_CAUSE"`
	PublicMsg   string `xml:"PUBLICMSG"`
	WorkStat    string `xml:"WORKSTAT"`
	CaseStat    string `xml:"CASESTAT"`
	JobStat     string `xml:"JOBSTAT"`
	AvgLat      string `xml:"AVGLAT"`
	AvgLong     string `xml:"AVGLONG"`
	CoordList   string `xml:"COORDLIST"`
}

func caseToAlert(client *http.Client, c omsCase) *domain.MunicipalAlert {
	if c.Serial == "" {
		return nil
	}

	planned := strings.EqualFold(strings.TrimSpace(c.Planned), "true") ||
		strings.TrimSpace(c.Planned) == "1"
	title := buildTitle(c, planned)
	desc := buildDescription(c)

	startsAt := parseOmsTime(c.OutTime)
	endsAt := parseOmsTime(c.RestoreTime)

	severity := domain.SeverityWarning
	if planned {
		severity = domain.SeverityInfo
	}

	var startsPtr, endsPtr *time.Time
	if !startsAt.IsZero() {
		startsPtr = &startsAt
	}
	if !endsAt.IsZero() {
		endsPtr = &endsAt
	}

	pub := startsAt
	if pub.IsZero() {
		pub = time.Now().UTC()
	}

	var areaPtr *string
	if area := reverseGeocode(client, c.AvgLat, c.AvgLong); area != "" {
		areaPtr = &area
	}

	return &domain.MunicipalAlert{
		ID:          "enova.power:" + c.Serial,
		Title:       title,
		Description: desc,
		AlertType:   "power-outage",
		Severity:    severity,
		Area:        areaPtr,
		Polygon:     parseCoordList(c.CoordList),
		StartsAt:    startsPtr,
		EndsAt:      endsPtr,
		PublishedAt: pub,
	}
}

func casesToAlerts(client *http.Client, cases []omsCase) []domain.MunicipalAlert {
	var alerts []domain.MunicipalAlert
	bySerial := make(map[string]int, len(cases))
	for _, c := range cases {
		a := caseToAlert(client, c)
		if a == nil {
			continue
		}
		if idx, ok := bySerial[c.Serial]; ok {
			alerts[idx] = *a
			continue
		}
		bySerial[c.Serial] = len(alerts)
		alerts = append(alerts, *a)
	}
	return alerts
}

func buildTitle(c omsCase, planned bool) string {
	kind := "Unplanned"
	if planned {
		kind = "Planned"
	}
	cause := strings.TrimSpace(c.DescCause)
	lower := strings.ToLower(cause)
	if cause == "" || lower == "unknown" || lower == "under investigation" {
		cause = "Outage"
	}
	return fmt.Sprintf("Enova Power %s %s", kind, cause)
}

func buildDescription(c omsCase) string {
	var parts []string

	if msg := strings.TrimSpace(c.PublicMsg); msg != "" {
		parts = append(parts, msg)
	} else if cause := strings.TrimSpace(c.DescCause); cause != "" &&
		!strings.EqualFold(cause, "unknown") && !strings.EqualFold(cause, "under investigation") {
		parts = append(parts, cause)
	}

	cur := parseCustomerCount(c.CurCust)
	init_ := parseCustomerCount(c.InitCust)
	if cur > 0 {
		parts = append(parts, fmt.Sprintf("%d customers affected", cur))
	} else if init_ > 0 {
		parts = append(parts, fmt.Sprintf("%d customers initially affected", init_))
	}

	if r := strings.TrimSpace(c.RestRange); r != "" {
		parts = append(parts, "Estimated restore: "+r)
	}

	return strings.Join(parts, ". ")
}

// parseCoordList converts the OMS flat "lat,lon,lat,lon,..." string into
// a slice of [lon, lat] pairs (GeoJSON order). The ring is already closed
// by the OMS (first == last), so no closing step is needed.
func parseCoordList(s string) [][]float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	if len(parts)%2 != 0 {
		return nil
	}
	ring := make([][]float64, 0, len(parts)/2)
	for i := 0; i < len(parts); i += 2 {
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[i+1]), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		ring = append(ring, []float64{lon, lat})
	}
	return ring
}

func parseCustomerCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// nominatimResponse is the subset of fields we use from the Nominatim reverse geocode API.
type nominatimResponse struct {
	Address struct {
		Suburb        string `json:"suburb"`
		Neighbourhood string `json:"neighbourhood"`
		City          string `json:"city"`
		Town          string `json:"town"`
		Village       string `json:"village"`
	} `json:"address"`
}

// reverseGeocode resolves lat/lon strings to a human-readable area name via Nominatim.
// Results are cached for geocodeTTL (24 h) to comply with Nominatim's usage policy.
// Returns empty string if either coordinate is unparseable or the lookup fails.
func reverseGeocode(client *http.Client, latStr, lonStr string) string {
	latF, err1 := strconv.ParseFloat(strings.TrimSpace(latStr), 64)
	lonF, err2 := strconv.ParseFloat(strings.TrimSpace(lonStr), 64)
	if err1 != nil || err2 != nil {
		return ""
	}

	key := fmt.Sprintf("%.8f,%.8f", latF, lonF)

	geocodeMu.RLock()
	entry, cached := geocodeCache[key]
	geocodeMu.RUnlock()
	if cached && time.Since(entry.cachedAt) < geocodeTTL {
		return entry.result
	}

	apiURL := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/reverse?format=json&lat=%.8f&lon=%.8f&zoom=16&addressdetails=1",
		latF, lonF,
	)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "cupola/1.0")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[enova.power] reverse geocode %.6f,%.6f: %v", latF, lonF, err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var nr nominatimResponse
	if err := json.NewDecoder(resp.Body).Decode(&nr); err != nil {
		return ""
	}

	suburb := nr.Address.Suburb
	if suburb == "" {
		suburb = nr.Address.Neighbourhood
	}
	place := nr.Address.City
	if place == "" {
		place = nr.Address.Town
	}
	if place == "" {
		place = nr.Address.Village
	}

	var result string
	if suburb != "" && place != "" {
		result = suburb + ", " + place
	} else if place != "" {
		result = place
	} else {
		result = suburb
	}

	geocodeMu.Lock()
	geocodeCache[key] = geocodeCacheEntry{result: result, cachedAt: time.Now()}
	geocodeMu.Unlock()

	return result
}

var omsTimeFormats = []string{
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"1/2/2006 3:04:05 PM",
	"1/2/2006 15:04:05",
}

func parseOmsTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, f := range omsTimeFormats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t.UTC()
		}
	}
	log.Printf("[enova.power] unparseable time %q", s)
	return time.Time{}
}
