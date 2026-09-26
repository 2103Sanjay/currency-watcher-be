package route_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/2103Sanjay/currency-watcher-be/internal/handler"
	"github.com/2103Sanjay/currency-watcher-be/internal/route"
	"github.com/2103Sanjay/currency-watcher-be/test/testutil"
)

func newRouter(svc *testutil.FakeService, allowedOrigins ...string) http.Handler {
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:5173"}
	}
	logger := testutil.DiscardLogger()
	return route.New(handler.NewRateHandler(svc, logger, "test"), logger, allowedOrigins)
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		method, path string
		wantStatus   int
	}{
		{http.MethodGet, "/api/health", http.StatusOK},
		{http.MethodGet, "/api/rates?base=USD&targets=EUR", http.StatusOK},
		{http.MethodGet, "/api/currencies", http.StatusOK},
		{http.MethodHead, "/api/health", http.StatusOK},
		{http.MethodPost, "/api/rates?base=USD", http.StatusMethodNotAllowed},
		{http.MethodDelete, "/api/currencies", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/nope", http.StatusNotFound},
		{http.MethodGet, "/", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newRouter(&testutil.FakeService{}).ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.method == http.MethodHead {
				return // no body
			}
			testutil.DecodeResponse(t, rec.Body, tc.wantStatus, nil)
			if tc.wantStatus == http.StatusMethodNotAllowed && rec.Header().Get("Allow") == "" {
				t.Error("405 response is missing the Allow header")
			}
		})
	}
}

func TestCORS_Preflight(t *testing.T) {
	tests := []struct {
		name       string
		origin     string
		wantHeader string
	}{
		{"allowed origin", "http://localhost:5173", "http://localhost:5173"},
		{"disallowed origin", "https://evil.example", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/api/rates", nil)
			req.Header.Set("Origin", tc.origin)
			rec := httptest.NewRecorder()
			newRouter(&testutil.FakeService{}).ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Errorf("preflight status = %d, want 204", rec.Code)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.wantHeader {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantHeader)
			}
		})
	}
}

func TestCORS_SimpleRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	newRouter(&testutil.FakeService{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestCORS_Wildcard(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Origin", "https://anywhere.example")
	rec := httptest.NewRecorder()
	newRouter(&testutil.FakeService{}, "*").ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://anywhere.example" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestRecoverPanics(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(&testutil.FakeService{PanicMsg: "boom"}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rates?base=USD", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	env := testutil.DecodeResponse(t, rec.Body, http.StatusInternalServerError, nil)
	if strings.Contains(env.Message, "boom") {
		t.Errorf("panic value leaked to client: %q", env.Message)
	}
}
