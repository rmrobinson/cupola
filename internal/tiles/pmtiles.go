package tiles

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/protomaps/go-pmtiles/pmtiles"
)

// Handler serves map tiles from a local .pmtiles file.
// If the file is absent on first call to New, it is extracted from
// build.protomaps.com using the configured bounding box.
type Handler struct {
	server    *pmtiles.Server
	tileName  string // pmtiles filename without extension, e.g. "local"
	cachePath string // absolute path to the .pmtiles file, for direct serving
}

// New creates a tile Handler. If cachePath does not exist, it synchronously
// extracts a region from build.protomaps.com before returning — expect this
// to take a few minutes on first run. sourceKey is the remote filename
// (e.g. "20251215.pmtiles"); if empty, the latest available build is
// discovered automatically.
func New(ctx context.Context, cachePath string, lat, lon, radiusKM float64, sourceKey string) (*Handler, error) {
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		if sourceKey == "" {
			log.Printf("[tiles] discovering latest Protomaps build…")
			var err error
			sourceKey, err = findLatestSourceKey(ctx)
			if err != nil {
				return nil, fmt.Errorf("tiles discover: %w", err)
			}
		}
		log.Printf("[tiles] cache absent — extracting %s (this may take several minutes)...", sourceKey)
		if err := extract(ctx, cachePath, lat, lon, radiusKM, sourceKey); err != nil {
			return nil, fmt.Errorf("tiles extract: %w", err)
		}
		log.Printf("[tiles] extraction complete: %s", cachePath)
	} else {
		log.Printf("[tiles] using cached file: %s", cachePath)
	}

	absCachePath, err := filepath.Abs(cachePath)
	if err != nil {
		return nil, fmt.Errorf("tiles cache path: %w", err)
	}
	absDir := filepath.Dir(absCachePath)

	tileName := strings.TrimSuffix(filepath.Base(cachePath), ".pmtiles")
	logger := log.New(os.Stderr, "[tiles] ", 0)
	bucket := pmtiles.NewFileBucket(absDir)
	server, err := pmtiles.NewServerWithBucket(bucket, "", logger, 64, "")
	if err != nil {
		return nil, fmt.Errorf("tiles server: %w", err)
	}
	server.Start()

	return &Handler{server: server, tileName: tileName, cachePath: absCachePath}, nil
}

// ServeFile serves the raw .pmtiles file with HTTP range-request support,
// as required by the protomaps-leaflet browser client.
func (h *Handler) ServeFile(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, h.cachePath)
}

// ServeHTTP translates /tiles/{z}/{x}/{y} into /{tileName}/{z}/{x}/{y}.mvt
// and forwards to the pmtiles server.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Strip the /tiles prefix; path becomes /{z}/{x}/{y}
	suffix := strings.TrimPrefix(r.URL.Path, "/tiles")
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + h.tileName + suffix + ".mvt"
	h.server.ServeHTTP(w, r2)
}

var probeClient = &http.Client{Timeout: 10 * time.Second}

// findLatestSourceKey probes build.protomaps.com for the most recent monthly
// build. Protomaps publishes files named YYYYMMDD.pmtiles roughly on the 1st
// and 15th of each month; we walk backwards up to 6 months.
func findLatestSourceKey(ctx context.Context) (string, error) {
	now := time.Now()
	for i := 0; i < 6; i++ {
		t := now.AddDate(0, -i, 0)
		for _, day := range []int{15, 1} {
			key := fmt.Sprintf("%04d%02d%02d.pmtiles", t.Year(), int(t.Month()), day)
			req, err := http.NewRequestWithContext(ctx, http.MethodHead,
				"https://build.protomaps.com/"+key, nil)
			if err != nil {
				continue
			}
			resp, err := probeClient.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return key, nil
			}
		}
	}
	return "", fmt.Errorf("no Protomaps build found in the last 6 months; set tiles.source_key in config to override")
}

// extract downloads a pmtiles region from build.protomaps.com.
func extract(ctx context.Context, outputPath string, lat, lon, radiusKM float64, sourceKey string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create tile dir: %w", err)
	}
	bbox := boundingBox(lat, lon, radiusKM)
	logger := log.New(os.Stderr, "[tiles] ", 0)
	log.Printf("[tiles] bounding box: %s", bbox)

	return pmtiles.Extract(
		ctx,
		logger,
		"https://build.protomaps.com", // remote bucket
		sourceKey,                     // e.g. "20251215.pmtiles"
		0,                             // minzoom
		14,                            // maxzoom
		"",                            // region file (unused)
		bbox,                          // "minlon,minlat,maxlon,maxlat"
		outputPath,                    // local output
		4,                             // download threads
		0.1,                           // overfetch fraction
		false,                         // dry run
	)
}

// boundingBox returns a "minlon,minlat,maxlon,maxlat" string centred at
// (lat, lon) with the given radius in km.
func boundingBox(lat, lon, radiusKM float64) string {
	dLat := radiusKM / 111.0
	dLon := radiusKM / (111.0 * math.Cos(lat*math.Pi/180))
	return fmt.Sprintf("%.4f,%.4f,%.4f,%.4f", lon-dLon, lat-dLat, lon+dLon, lat+dLat)
}
