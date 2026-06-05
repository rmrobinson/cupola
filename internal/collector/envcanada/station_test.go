package envcanada

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProvinceStationsCacheDoesNotRefetch(t *testing.T) {
	resetStationTestState()
	defer resetStationTestState()

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = fmt.Fprint(w, `<html><script>[{"display_name":"Kitchener Waterloo","code":"s0000585","lat":43.45,"lon":-80.49,"province":"ON"}]</script></html>`)
	}))
	defer srv.Close()
	stationPageBaseURL = srv.URL

	first, err := provinceStations("ON")
	if err != nil {
		t.Fatalf("provinceStations first: %v", err)
	}
	second, err := provinceStations("ON")
	if err != nil {
		t.Fatalf("provinceStations second: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("station lengths = %d, %d; want 1, 1", len(first), len(second))
	}
	if first[0].Name != "Kitchener Waterloo" || second[0].Name != "Kitchener Waterloo" {
		t.Fatalf("unexpected station data: %+v %+v", first, second)
	}
}

func resetStationTestState() {
	stationPageBaseURL = "https://weather.gc.ca"
	provinceStationCache.mu.Lock()
	provinceStationCache.stations = nil
	provinceStationCache.mu.Unlock()
	stationCache.mu.Lock()
	stationCache.ready = false
	stationCache.lat = 0
	stationCache.lon = 0
	stationCache.name = ""
	stationCache.mu.Unlock()
}
