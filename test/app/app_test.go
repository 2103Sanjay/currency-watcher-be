package app_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/app"
	"github.com/2103Sanjay/currency-watcher-be/test/testutil"
)

// freePort returns a TCP port that is currently unused on localhost.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

// fakeUpstream stands in for the Frankfurter API and counts rate requests.
func fakeUpstream(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rates" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		fmt.Fprintf(w, `[{"date":"2026-09-26","base":%q,"quote":"EUR","rate":0.9},
			{"date":"2026-09-26","base":%q,"quote":"INR","rate":95.87}]`,
			r.URL.Query().Get("base"), r.URL.Query().Get("base"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setEnv(t *testing.T, port, upstream string) {
	t.Helper()
	t.Setenv("PORT", port)
	t.Setenv("RATES_API_URL", upstream)
	t.Setenv("CACHE_TTL", "1h")
	t.Setenv("UPSTREAM_TIMEOUT", "5s")
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:5173")
}

// waitHealthy polls the health endpoint until the server answers.
func waitHealthy(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not become healthy in time")
}

func getRates(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	var data map[string]any
	testutil.DecodeResponse(t, resp.Body, http.StatusOK, &data)
	return data
}

// TestRunEndToEnd starts the real server against a fake upstream, checks the
// endpoints and caching over HTTP, then verifies graceful shutdown.
func TestRunEndToEnd(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := fakeUpstream(t, &upstreamCalls)
	port := freePort(t)
	setEnv(t, port, upstream.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, testutil.DiscardLogger(), "test") }()

	baseURL := "http://127.0.0.1:" + port
	waitHealthy(t, baseURL)

	first := getRates(t, baseURL+"/api/rates?base=USD&targets=INR")
	if rates, _ := first["rates"].(map[string]any); rates["INR"] != 95.87 {
		t.Errorf("unexpected first response: %v", first)
	}
	if first["cached"] != false {
		t.Errorf("first response should come from upstream: %v", first)
	}

	second := getRates(t, baseURL+"/api/rates?base=USD&targets=EUR")
	if second["cached"] != true {
		t.Errorf("second response should come from cache: %v", second)
	}
	if got := upstreamCalls.Load(); got != 1 {
		t.Errorf("upstream calls = %d, want 1", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v after shutdown, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}

	if _, err := http.Get(baseURL + "/api/health"); err == nil {
		t.Error("server still accepting connections after shutdown")
	}
}

func TestRunInvalidConfig(t *testing.T) {
	setEnv(t, freePort(t), "http://127.0.0.1:1")
	t.Setenv("CACHE_TTL", "not-a-duration")

	if err := app.Run(context.Background(), testutil.DiscardLogger(), "test"); err == nil {
		t.Fatal("expected configuration error")
	}
}

func TestRunPortInUse(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	setEnv(t, strconv.Itoa(l.Addr().(*net.TCPAddr).Port), "http://127.0.0.1:1")

	done := make(chan error, 1)
	go func() { done <- app.Run(context.Background(), testutil.DiscardLogger(), "test") }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected listen error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not fail on an occupied port")
	}
}
