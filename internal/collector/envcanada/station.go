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
	"sync"
	"time"
)

// All Canadian province and territory IDs used by weather.gc.ca.
var allProvinces = []string{
	"AB", "BC", "MB", "NB", "NL", "NS", "NT", "NU", "ON", "PE", "QC", "SK", "YT",
}

// ecStation is one entry from the embedded station JSON on each province page.
type ecStation struct {
	Name     string  `json:"display_name"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Province string  `json:"province"`
}

// stationCache holds the single discovered station for this server's lifetime.
var stationCache struct {
	once sync.Once
	lat  float64
	lon  float64
	name string
	err  error
}

// getNearestStation returns the lat/lon of the EC reporting station closest to
// (userLat, userLon).  The result is computed once and cached for the process
// lifetime — all 13 province pages are fetched concurrently on the first call.
func getNearestStation(userLat, userLon float64) (lat, lon float64, err error) {
	stationCache.once.Do(func() {
		stationCache.lat, stationCache.lon, stationCache.name, stationCache.err =
			discoverNearestStation(userLat, userLon)
	})
	return stationCache.lat, stationCache.lon, stationCache.err
}

// discoverNearestStation fetches all province pages concurrently, aggregates
// the embedded station JSON, and returns the closest station to the user.
func discoverNearestStation(userLat, userLon float64) (lat, lon float64, name string, err error) {
	type result struct {
		stations []ecStation
		err      error
	}
	ch := make(chan result, len(allProvinces))
	client := &http.Client{Timeout: 20 * time.Second}

	for _, prov := range allProvinces {
		prov := prov
		go func() {
			s, e := fetchProvinceStations(client, prov)
			ch <- result{s, e}
		}()
	}

	var all []ecStation
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

// fetchProvinceStations downloads one province page and extracts its embedded
// station JSON array.
func fetchProvinceStations(client *http.Client, province string) ([]ecStation, error) {
	url := "https://weather.gc.ca/forecast/canada/index_e.html?id=" + province
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
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

	var stations []ecStation
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

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
