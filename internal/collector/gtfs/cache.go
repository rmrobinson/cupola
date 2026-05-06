package gtfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func zipCacheDir(cacheDir, agencyID string) string {
	return filepath.Join(cacheDir, "gtfs", agencyID)
}

func zipCachePath(cacheDir, agencyID string, n int) string {
	return filepath.Join(zipCacheDir(cacheDir, agencyID), fmt.Sprintf("%d.zip", n))
}

// SaveZips atomically writes each blob to <cacheDir>/gtfs/<agencyID>/<n>.zip
// using a temp-file-then-rename strategy.
func SaveZips(cacheDir, agencyID string, blobs [][]byte) error {
	dir := zipCacheDir(cacheDir, agencyID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	for i, data := range blobs {
		dest := zipCachePath(cacheDir, agencyID, i)
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return fmt.Errorf("write temp zip %d: %w", i, err)
		}
		if err := os.Rename(tmp, dest); err != nil {
			os.Remove(tmp) //nolint:errcheck
			return fmt.Errorf("rename zip %d: %w", i, err)
		}
	}
	return nil
}

// LoadZips reads previously cached ZIPs for agencyID from cacheDir.
// Returns nil, nil when no cache exists (0.zip not found).
// Files are loaded in numeric order: 0.zip, 1.zip, ... until a gap is hit.
func LoadZips(cacheDir, agencyID string) ([][]byte, error) {
	var blobs [][]byte
	for i := 0; ; i++ {
		p := zipCachePath(cacheDir, agencyID, i)
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read cached zip %d: %w", i, err)
		}
		blobs = append(blobs, data)
	}
	return blobs, nil
}
