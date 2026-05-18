package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddlewareAllowsConfiguredOrigin(t *testing.T) {
	h := &Handler{corsOrigins: normalizeCORSOrigins([]string{"http://dashboard.local:8181"})}
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/config", nil)
	req.Header.Set("Origin", "http://dashboard.local:8181")
	rr := httptest.NewRecorder()

	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://dashboard.local:8181" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestCORSMiddlewareRejectsUnknownPreflightOrigin(t *testing.T) {
	h := &Handler{corsOrigins: normalizeCORSOrigins([]string{"http://dashboard.local:8181"})}
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/config", nil)
	req.Header.Set("Origin", "http://evil.example")
	rr := httptest.NewRecorder()

	h.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}
