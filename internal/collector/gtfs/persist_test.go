package gtfs

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmrobinson/cupola/internal/store"
)

func TestWarmFromCacheHydratesValidSQLiteMetadata(t *testing.T) {
	gtfsDB, cacheDir := loadTestGTFSCache(t)
	s := New()

	ok, err := WarmFromCache(s, "test", cacheDir, gtfsDB, time.Date(2026, time.January, 5, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("WarmFromCache() error = %v", err)
	}
	if !ok {
		t.Fatal("WarmFromCache() = false, want true")
	}
	if got := s.RouteName("R1"); got != "KI" {
		t.Fatalf("RouteName() = %q, want KI", got)
	}
	if got := s.StopName("S1"); got != "Kitchener GO" {
		t.Fatalf("StopName() = %q, want Kitchener GO", got)
	}
	if stops := s.StopsForRoute("R1"); len(stops) != 1 || stops[0].ID != "S1" {
		t.Fatalf("StopsForRoute() = %#v, want S1", stops)
	}
}

func TestWarmFromCacheRejectsStaleCache(t *testing.T) {
	gtfsDB, cacheDir := loadTestGTFSCache(t)
	s := New()

	ok, err := WarmFromCache(s, "test", cacheDir, gtfsDB, time.Date(2027, time.January, 5, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("WarmFromCache() error = %v", err)
	}
	if ok {
		t.Fatal("WarmFromCache() = true, want false for stale cache")
	}
	if s.HasRoutes() {
		t.Fatal("HasRoutes() = true, want stale cache hidden")
	}
}

func TestWarmFromCacheUpgradesOldSQLiteFromZipCache(t *testing.T) {
	dir := t.TempDir()
	gtfsDB, err := store.NewGTFSSQLiteStore(dir)
	if err != nil {
		t.Fatalf("NewGTFSSQLiteStore() error = %v", err)
	}
	zipData := minimalGTFSZip(t)
	oldSchedule := New()
	if _, err := oldSchedule.parseFromBlobsFull("test", [][]byte{zipData}); err != nil {
		t.Fatalf("parseFromBlobsFull() error = %v", err)
	}
	if err := gtfsDB.ReplaceGTFSAgency("test",
		[]store.GTFSStopTime{{RouteID: "R1", TripID: "T1", StopID: "S1", Headsign: "Union Station", ServiceID: "SVC", StopSequence: 1, DepartureSecs: 6 * 3600}},
		[]store.GTFSService{{ServiceID: "SVC", WeekdayMask: store.WeekdayMon, StartDate: "20260101", EndDate: "20261231"}},
		nil,
		nil,
	); err != nil {
		t.Fatalf("ReplaceGTFSAgency() error = %v", err)
	}
	if err := SaveZips(dir, "test", [][]byte{zipData}); err != nil {
		t.Fatalf("SaveZips() error = %v", err)
	}

	s := New()
	ok, err := WarmFromCache(s, "test", dir, gtfsDB, time.Date(2026, time.January, 5, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("WarmFromCache() error = %v", err)
	}
	if !ok {
		t.Fatal("WarmFromCache() = false, want true")
	}
	if got := s.StopName("S1"); got != "Kitchener GO" {
		t.Fatalf("StopName() = %q, want Kitchener GO", got)
	}
	hasMetadata, err := gtfsDB.HasGTFSMetadata("test")
	if err != nil {
		t.Fatalf("HasGTFSMetadata() error = %v", err)
	}
	if !hasMetadata {
		t.Fatal("HasGTFSMetadata() = false after warmup upgrade")
	}
}

func loadTestGTFSCache(t *testing.T) (*store.GTFSSQLiteStore, string) {
	t.Helper()
	dir := t.TempDir()
	gtfsDB, err := store.NewGTFSSQLiteStore(dir)
	if err != nil {
		t.Fatalf("NewGTFSSQLiteStore() error = %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(minimalGTFSZip(t))
	}))
	t.Cleanup(srv.Close)

	if err := LoadAndPersist(New(), "test", []string{srv.URL}, dir, gtfsDB); err != nil {
		t.Fatalf("LoadAndPersist() error = %v", err)
	}
	return gtfsDB, dir
}

func minimalGTFSZip(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"routes.txt": "route_id,agency_id,route_short_name,route_long_name,route_type\n" +
			"R1,test,KI,Kitchener Line,2\n",
		"stops.txt": "stop_id,stop_code,stop_name,stop_lat,stop_lon\n" +
			"S1,KI,Kitchener GO,43.4516,-80.4925\n",
		"trips.txt": "route_id,service_id,trip_id,trip_headsign,shape_id\n" +
			"R1,SVC,T1,Union Station,\n",
		"calendar.txt": "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\n" +
			"SVC,1,0,0,0,0,0,0,20260101,20261231\n",
		"calendar_dates.txt": "service_id,date,exception_type\n",
		"stop_times.txt": "trip_id,arrival_time,departure_time,stop_id,stop_sequence\n" +
			"T1,06:00:00,06:00:00,S1,1\n",
	}
	for name, contents := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s in zip: %v", name, err)
		}
		if _, err := w.Write([]byte(contents)); err != nil {
			t.Fatalf("write %s in zip: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
