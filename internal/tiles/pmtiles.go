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

	"github.com/protomaps/go-pmtiles/pmtiles"
)

// Handler serves map tiles from a local .pmtiles file.
// If the file is absent on first call to New, it is extracted from
// build.protomaps.com using the configured bounding box.
type Handler struct {
	server   *pmtiles.Server
	tileName string // pmtiles filename without extension, e.g. "local"
}

// New creates a tile Handler. If cachePath does not exist, it synchronously
// extracts a region from build.protomaps.com before returning — expect this
// to take a few minutes on first run.
func New(ctx context.Context, cachePath string, lat, lon, radiusKM float64) (*Handler, error) {
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		log.Printf("[tiles] cache absent — fetching from build.protomaps.com (this may take several minutes)...")
		if err := extract(ctx, cachePath, lat, lon, radiusKM); err != nil {
			return nil, fmt.Errorf("tiles extract: %w", err)
		}
		log.Printf("[tiles] extraction complete: %s", cachePath)
	} else {
		log.Printf("[tiles] using cached file: %s", cachePath)
	}

	absDir, err := filepath.Abs(filepath.Dir(cachePath))
	if err != nil {
		return nil, fmt.Errorf("tiles dir: %w", err)
	}

	tileName := strings.TrimSuffix(filepath.Base(cachePath), ".pmtiles")
	logger := log.New(os.Stderr, "[tiles] ", 0)
	bucket := pmtiles.NewFileBucket(absDir)
	server, err := pmtiles.NewServerWithBucket(bucket, "", logger, 64, "")
	if err != nil {
		return nil, fmt.Errorf("tiles server: %w", err)
	}
	server.Start()

	return &Handler{server: server, tileName: tileName}, nil
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

// extract downloads a pmtiles region from build.protomaps.com.
func extract(ctx context.Context, outputPath string, lat, lon, radiusKM float64) error {
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
		"planet.pmtiles",              // source key
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
