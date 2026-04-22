package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/parsedong/stock/api-server/internal/config"
	"github.com/parsedong/stock/api-server/internal/handler"
	"github.com/parsedong/stock/api-server/internal/svc"
)

// newTestServer creates a test HTTP handler (no real DB/gRPC needed).
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	ctx := &svc.ServiceContext{Config: config.Config{}}
	return handler.NewHTTPHandler(ctx)
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("health: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ok") {
		t.Errorf("health: body %q does not contain 'ok'", rr.Body.String())
	}
}

func TestSymbolsEndpointReturnsEmptyWhenNoData(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/symbols?freq=1m", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("symbols: got %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "symbols") {
		t.Errorf("symbols: body %q does not contain 'symbols'", rr.Body.String())
	}
}
