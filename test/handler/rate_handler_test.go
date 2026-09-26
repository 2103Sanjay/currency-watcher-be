package handler_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/2103Sanjay/currency-watcher-be/internal/handler"
	"github.com/2103Sanjay/currency-watcher-be/internal/model"
	"github.com/2103Sanjay/currency-watcher-be/test/testutil"
)

// ratesBody mirrors the data of GET /api/rates.
type ratesBody struct {
	Base   string             `json:"base"`
	Rates  map[string]float64 `json:"rates"`
	Cached bool               `json:"cached"`
}

func newHandler(svc *testutil.FakeService) *handler.RateHandler {
	return handler.NewRateHandler(svc, testutil.DiscardLogger(), "test")
}

// manyTargets returns n distinct, well-formed currency codes joined by commas.
func manyTargets(n int) string {
	codes := make([]string, n)
	for i := range codes {
		codes[i] = string([]byte{'A' + byte(i/26%26), 'A' + byte(i%26), 'X'})
	}
	return strings.Join(codes, ",")
}

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandler(&testutil.FakeService{}).Health(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var data map[string]string
	testutil.DecodeResponse(t, rec.Body, http.StatusOK, &data)
	if data["version"] != "test" {
		t.Errorf("unexpected data: %v", data)
	}
}

func TestRates(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		svcErr      error
		wantStatus  int
		wantBase    string
		wantTargets []string
	}{
		{
			name:        "normalises and de-duplicates codes",
			query:       "base=usd&targets=eur, SGD,EUR",
			wantStatus:  http.StatusOK,
			wantBase:    "USD",
			wantTargets: []string{"EUR", "SGD"},
		},
		{name: "targets optional", query: "base=USD", wantStatus: http.StatusOK, wantBase: "USD"},
		{name: "missing base", query: "targets=EUR", wantStatus: http.StatusBadRequest},
		{name: "invalid base", query: "base=US&targets=EUR", wantStatus: http.StatusBadRequest},
		{name: "invalid target", query: "base=USD&targets=EUR,E1R", wantStatus: http.StatusBadRequest},
		{
			name:        "skips empty entries in targets",
			query:       "base=USD&targets=,EUR,,SGD,",
			wantStatus:  http.StatusOK,
			wantBase:    "USD",
			wantTargets: []string{"EUR", "SGD"},
		},
		{name: "too many targets", query: "base=USD&targets=" + manyTargets(handler.MaxTargets+1), wantStatus: http.StatusBadRequest},
		{
			name:       "unknown currency",
			query:      "base=USD&targets=XYZ",
			svcErr:     fmt.Errorf("%w: XYZ", model.ErrUnknownCurrency),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "upstream failure",
			query:      "base=USD&targets=EUR",
			svcErr:     errors.New("connection refused"),
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &testutil.FakeService{Err: tc.svcErr}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/rates?"+strings.ReplaceAll(tc.query, " ", "%20"), nil)
			newHandler(svc).Rates(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
			if tc.wantStatus != http.StatusOK {
				testutil.DecodeResponse(t, rec.Body, tc.wantStatus, nil)
				return
			}
			if svc.GotBase != tc.wantBase || !reflect.DeepEqual(svc.GotTargets, tc.wantTargets) {
				t.Errorf("service got base=%q targets=%v, want %q %v", svc.GotBase, svc.GotTargets, tc.wantBase, tc.wantTargets)
			}
			var body ratesBody
			testutil.DecodeResponse(t, rec.Body, http.StatusOK, &body)
			if body.Base != tc.wantBase || !body.Cached {
				t.Errorf("unexpected body: %+v", body)
			}
		})
	}
}

func TestCurrencies(t *testing.T) {
	tests := []struct {
		name       string
		svcErr     error
		wantStatus int
	}{
		{"success", nil, http.StatusOK},
		{"upstream failure", errors.New("connection refused"), http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newHandler(&testutil.FakeService{Err: tc.svcErr}).Currencies(rec, httptest.NewRequest(http.MethodGet, "/api/currencies", nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK {
				testutil.DecodeResponse(t, rec.Body, tc.wantStatus, nil)
				return
			}
			var list []model.Currency
			testutil.DecodeResponse(t, rec.Body, http.StatusOK, &list)
			if len(list) != 1 || list[0].Code != "EUR" {
				t.Errorf("unexpected currencies: %+v", list)
			}
		})
	}
}
