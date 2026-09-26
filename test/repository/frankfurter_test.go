package repository_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/model"
	"github.com/2103Sanjay/currency-watcher-be/internal/repository"
)

func TestFrankfurterLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rates" || r.URL.Query().Get("base") != "USD" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"date":"2026-09-25","base":"USD","quote":"ALL","rate":80.5},
			{"date":"2026-09-26","base":"USD","quote":"EUR","rate":0.87735},
			{"date":"2026-09-26","base":"USD","quote":"SGD","rate":1.2791}
		]`))
	}))
	defer srv.Close()

	table, err := repository.NewFrankfurterRepository(srv.URL, srv.Client()).Latest(context.Background(), "USD")
	if err != nil {
		t.Fatal(err)
	}
	if table.Base != "USD" || table.Date != "2026-09-26" {
		t.Errorf("got base=%s date=%s", table.Base, table.Date)
	}
	if len(table.Rates) != 3 || table.Rates["EUR"] != 0.87735 {
		t.Errorf("unexpected rates: %v", table.Rates)
	}
}

func TestFrankfurterLatest_Errors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantUnknown bool
	}{
		{"invalid currency", http.StatusUnprocessableEntity, `{"status":422,"message":"invalid currency: XXX"}`, true},
		{"server error", http.StatusInternalServerError, `oops`, false},
		{"malformed body", http.StatusOK, `{not json`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := repository.NewFrankfurterRepository(srv.URL, srv.Client()).Latest(context.Background(), "XXX")
			if err == nil {
				t.Fatal("expected error")
			}
			if got := errors.Is(err, model.ErrUnknownCurrency); got != tc.wantUnknown {
				t.Errorf("errors.Is(err, model.ErrUnknownCurrency) = %v, want %v (err: %v)", got, tc.wantUnknown, err)
			}
		})
	}
}

func TestFrankfurterCurrencies_FiltersDiscontinued(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"iso_code":"USD","name":"US Dollar","end_date":"2026-09-26"},
			{"iso_code":"DEM","name":"German Mark","end_date":"2001-12-31"},
			{"iso_code":"EUR","name":"Euro","end_date":"2026-09-25"}
		]`))
	}))
	defer srv.Close()

	now := func() time.Time { return time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC) }
	f := repository.NewFrankfurterRepository(srv.URL, srv.Client(), repository.WithClock(now))

	list, err := f.Currencies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Code != "EUR" || list[1].Code != "USD" {
		t.Errorf("unexpected currencies: %+v", list)
	}
}

func TestFrankfurterLatest_SendsHeadersAndEscapesBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := r.URL.Query().Get("base"); got != "US D&x=1" {
			t.Errorf("base = %q, want it passed through as a single escaped value", got)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if _, err := repository.NewFrankfurterRepository(srv.URL, srv.Client()).Latest(context.Background(), "US D&x=1"); err != nil {
		t.Fatal(err)
	}
}

func TestFrankfurterLatest_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing is listening any more

	_, err := repository.NewFrankfurterRepository(url, http.DefaultClient).Latest(context.Background(), "USD")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, model.ErrUnknownCurrency) {
		t.Errorf("connection failure must not look like an unknown currency: %v", err)
	}
}

func TestFrankfurterLatest_Timeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	client := &http.Client{Timeout: 50 * time.Millisecond}
	start := time.Now()
	if _, err := repository.NewFrankfurterRepository(srv.URL, client).Latest(context.Background(), "USD"); err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("request took %s; the client timeout was not honoured", elapsed)
	}
}

func TestFrankfurterCurrencies_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := repository.NewFrankfurterRepository(srv.URL, srv.Client()).Currencies(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
