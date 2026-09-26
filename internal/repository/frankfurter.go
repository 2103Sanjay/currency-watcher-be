package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/model"
)

// maxResponseBytes caps how much of an upstream response body we read.
const maxResponseBytes = 1 << 20 // 1 MiB

// activeWithin is how recently a currency must have been quoted for
// Currencies to report it as supported; older ones are discontinued.
const activeWithin = 30 * 24 * time.Hour

// FrankfurterRepository is a RateRepository backed by the Frankfurter v2 API
// (https://frankfurter.dev).
type FrankfurterRepository struct {
	baseURL string
	client  *http.Client
	now     func() time.Time
}

var _ RateRepository = (*FrankfurterRepository)(nil)

// Option configures a FrankfurterRepository.
type Option func(*FrankfurterRepository)

// WithClock overrides the time source used to decide which currencies are
// still active. Intended for tests.
func WithClock(now func() time.Time) Option {
	return func(f *FrankfurterRepository) { f.now = now }
}

// NewFrankfurterRepository returns a client for the Frankfurter API rooted at
// baseURL, e.g. "https://api.frankfurter.dev/v2".
func NewFrankfurterRepository(baseURL string, client *http.Client, opts ...Option) *FrankfurterRepository {
	f := &FrankfurterRepository{baseURL: baseURL, client: client, now: time.Now}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

type frankfurterQuote struct {
	Date  string  `json:"date"`
	Base  string  `json:"base"`
	Quote string  `json:"quote"`
	Rate  float64 `json:"rate"`
}

type frankfurterCurrency struct {
	ISOCode string `json:"iso_code"`
	Name    string `json:"name"`
	EndDate string `json:"end_date"`
}

// Latest implements RateRepository.
func (f *FrankfurterRepository) Latest(ctx context.Context, base string) (model.RateTable, error) {
	endpoint := f.baseURL + "/rates?base=" + url.QueryEscape(base)

	var quotes []frankfurterQuote
	if err := f.getJSON(ctx, endpoint, &quotes); err != nil {
		if errors.Is(err, model.ErrUnknownCurrency) {
			return model.RateTable{}, fmt.Errorf("%w: %s", model.ErrUnknownCurrency, base)
		}
		return model.RateTable{}, err
	}

	table := model.RateTable{Base: base, Rates: make(map[string]float64, len(quotes))}
	for _, q := range quotes {
		table.Rates[q.Quote] = q.Rate
		// ISO dates compare correctly as strings.
		if q.Date > table.Date {
			table.Date = q.Date
		}
	}
	return table, nil
}

// Currencies implements RateRepository.
func (f *FrankfurterRepository) Currencies(ctx context.Context) ([]model.Currency, error) {
	var raw []frankfurterCurrency
	if err := f.getJSON(ctx, f.baseURL+"/currencies", &raw); err != nil {
		return nil, err
	}

	cutoff := f.now().Add(-activeWithin).Format(time.DateOnly)
	out := make([]model.Currency, 0, len(raw))
	for _, c := range raw {
		if c.EndDate != "" && c.EndDate < cutoff {
			continue
		}
		out = append(out, model.Currency{Code: c.ISOCode, Name: c.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (f *FrankfurterRepository) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("frankfurter: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("frankfurter: request failed: %w", err)
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, maxResponseBytes)

	switch {
	case resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusNotFound:
		// Frankfurter answers 422 for an invalid currency code.
		return model.ErrUnknownCurrency
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("frankfurter: unexpected status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(body).Decode(dst); err != nil {
		return fmt.Errorf("frankfurter: decode response: %w", err)
	}
	return nil
}
