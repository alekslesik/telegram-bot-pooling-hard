package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	healthHandler("v1.2.3", "deadbeef")(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json content type")
	}
	var body healthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Version != "v1.2.3" || body.Commit != "deadbeef" {
		t.Fatalf("unexpected version fields: %+v", body)
	}
}

func TestReadyzMemoryAndRedisDisabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	h := readinessHandler(nil, nil, true)
	h(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
