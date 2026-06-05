package envcanada

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// All Canadian province and territory IDs used by weather.gc.ca.
var allProvinces = []string{
	"AB", "BC", "MB", "NB", "NL", "NS", "NT", "NU", "ON", "PE", "QC", "SK", "YT",
}

var stationPageBaseURL = "https://weather.gc.ca"

// ECStation is one entry from the embedded station JSON on each province page.
type ECStation struct {
	Name        string  `json:"display_name"`
	Code        string  `json:"code"`
	SiteCode    string  `json:"site_code"`
	StationCode string  `json:"station_code"`
	ID          string  `json:"id"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Province    string  `json:"province"`
}

// StationOverride selects a specific Environment Canada station from a
// province/territory station list instead of auto-discovering the nearest one.
type StationOverride struct {
	Code     string
	Province string
}

// stationCache holds the single discovered station for this server's lifetime.
var stationCache struct {
	mu    sync.Mutex
	ready bool
	lat   float64
	lon   float64
	name  string
}

var provinceStationCache struct {
	mu       sync.Mutex
	stations map[string][]ECStation
}

// getNearestStation returns the lat/lon of the EC reporting station closest to
// (userLat, userLon).  The result is computed once and cached for the process
// lifetime after a successful discovery. Failed discovery is intentionally not
// cached so collectors can recover when internet connectivity returns.
func getNearestStation(userLat, userLon float64) (lat, lon float64, err error) {
	stationCache.mu.Lock()
	defer stationCache.mu.Unlock()
	if stationCache.ready {
		return stationCache.lat, stationCache.lon, nil
	}
	lat, lon, name, err := discoverNearestStation(userLat, userLon)
	if err != nil {
		return 0, 0, err
	}
	stationCache.lat, stationCache.lon, stationCache.name = lat, lon, name
	stationCache.ready = true
	return lat, lon, nil
}

func resolveStation(userLat, userLon float64, override StationOverride) (lat, lon float64, err error) {
	if strings.TrimSpace(override.Code) == "" {
		return getNearestStation(userLat, userLon)
	}
	return discoverStationByCode(override)
}

func discoverStationByCode(override StationOverride) (lat, lon float64, err error) {
	code := strings.TrimSpace(override.Code)
	province := strings.ToUpper(strings.TrimSpace(override.Province))
	if province == "" {
		return 0, 0, fmt.Errorf("province is required when station_code is set")
	}
	stations, err := provinceStations(province)
	if err != nil {
		return 0, 0, err
	}
	for _, s := range stations {
		if s.matchesCode(code) {
			log.Printf("[envcanada] configured station: %s (%.3f, %.3f)", s.Name, s.Lat, s.Lon)
			return s.Lat, s.Lon, nil
		}
	}
	return 0, 0, fmt.Errorf("station_code %q not found in province %s", code, province)
}

// discoverNearestStation fetches all province pages concurrently, aggregates
// the embedded station JSON, and returns the closest station to the user.
func discoverNearestStation(userLat, userLon float64) (lat, lon float64, name string, err error) {
	type result struct {
		stations []ECStation
		err      error
	}
	ch := make(chan result, len(allProvinces))

	for _, prov := range allProvinces {
		prov := prov
		go func() {
			s, e := provinceStations(prov)
			ch <- result{s, e}
		}()
	}

	var all []ECStation
	for range allProvinces {
		r := <-ch
		if r.err != nil {
			log.Printf("[envcanada] station fetch error: %v", r.err)
			continue
		}
		all = append(all, r.stations...)
	}

	if len(all) == 0 {
		return 0, 0, "", fmt.Errorf("no stations found across all provinces")
	}

	// Haversine nearest-neighbour search.
	nearest := all[0]
	best := haversineKm(userLat, userLon, nearest.Lat, nearest.Lon)
	for _, s := range all[1:] {
		if d := haversineKm(userLat, userLon, s.Lat, s.Lon); d < best {
			best = d
			nearest = s
		}
	}

	log.Printf("[envcanada] nearest station: %s (%.3f, %.3f) — %.1f km away",
		nearest.Name, nearest.Lat, nearest.Lon, best)
	return nearest.Lat, nearest.Lon, nearest.Name, nil
}

func (s ECStation) matchesCode(code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, candidate := range []string{s.Code, s.SiteCode, s.StationCode, s.ID} {
		if strings.ToLower(strings.TrimSpace(candidate)) == code {
			return true
		}
	}
	return false
}

func provinceStations(province string) ([]ECStation, error) {
	province = strings.ToUpper(strings.TrimSpace(province))
	if province == "" {
		return nil, fmt.Errorf("province is required")
	}
	provinceStationCache.mu.Lock()
	if provinceStationCache.stations != nil {
		if stations, ok := provinceStationCache.stations[province]; ok {
			out := append([]ECStation(nil), stations...)
			provinceStationCache.mu.Unlock()
			return out, nil
		}
	}
	provinceStationCache.mu.Unlock()

	stations, err := fetchProvinceStations(&http.Client{Timeout: 20 * time.Second}, province)
	if err != nil {
		return nil, err
	}

	provinceStationCache.mu.Lock()
	if provinceStationCache.stations == nil {
		provinceStationCache.stations = make(map[string][]ECStation)
	}
	provinceStationCache.stations[province] = append([]ECStation(nil), stations...)
	provinceStationCache.mu.Unlock()
	return stations, nil
}

func allStations() ([]ECStation, error) {
	type result struct {
		stations []ECStation
		err      error
	}
	ch := make(chan result, len(allProvinces))
	for _, prov := range allProvinces {
		prov := prov
		go func() {
			stations, err := provinceStations(prov)
			ch <- result{stations: stations, err: err}
		}()
	}
	var all []ECStation
	var errs []string
	for range allProvinces {
		r := <-ch
		if r.err != nil {
			errs = append(errs, r.err.Error())
			continue
		}
		all = append(all, r.stations...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no stations found: %s", strings.Join(errs, "; "))
	}
	return all, nil
}

// fetchProvinceStations downloads one province page and extracts its embedded
// station JSON array.
func fetchProvinceStations(client *http.Client, province string) ([]ECStation, error) {
	url := stationPageBaseURL + "/forecast/canada/index_e.html?id=" + province
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}

	// The page embeds the full station list as a JSON array in a script block:
	//   [{"display_name":"...","lat":...,"lon":...,"province":"..."},...]
	marker := []byte(`[{"display_name":`)
	start := bytes.Index(body, marker)
	if start < 0 {
		return nil, fmt.Errorf("no station JSON found on %s page", province)
	}

	var stations []ECStation
	// json.Decoder.Decode reads one complete JSON value and ignores trailing content.
	if err := json.NewDecoder(bytes.NewReader(body[start:])).Decode(&stations); err != nil {
		return nil, fmt.Errorf("decode stations for %s: %w", province, err)
	}
	return stations, nil
}

// stationRSSURL builds the EC RSS URL for a weather station's exact coordinates.
func stationRSSURL(feedType string, lat, lon float64) string {
	// Use strconv to avoid trailing zeros while preserving full precision.
	sLat := strconv.FormatFloat(lat, 'f', -1, 64)
	sLon := strconv.FormatFloat(lon, 'f', -1, 64)
	return fmt.Sprintf("https://weather.gc.ca/rss/%s/%s_%s_e.xml", feedType, sLat, sLon)
}

func stationHourlyForecastURL(lat, lon float64) string {
	sLat := strconv.FormatFloat(lat, 'f', -1, 64)
	sLon := strconv.FormatFloat(lon, 'f', -1, 64)
	return fmt.Sprintf("https://weather.gc.ca/en/forecast/hourly/index.html?coords=%s,%s", sLat, sLon)
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
