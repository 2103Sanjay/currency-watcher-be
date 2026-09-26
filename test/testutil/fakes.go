// Package testutil provides fakes and helpers shared by the tests under test/.
package testutil

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/model"
	"github.com/2103Sanjay/currency-watcher-be/internal/repository"
	"github.com/2103Sanjay/currency-watcher-be/internal/service"
)

// DiscardLogger returns a logger that drops everything.
func DiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// SampleTable is a small rate table for tests.
func SampleTable() model.RateTable {
	return model.RateTable{Date: "2026-01-01", Rates: map[string]float64{"EUR": 0.9, "SGD": 1.3, "JPY": 150}}
}

// FakeRepository is an in-memory repository.RateRepository.
type FakeRepository struct {
	// Calls counts calls to Latest and Currencies.
	Calls atomic.Int32
	// Block, if non-nil, is waited on before Latest returns.
	Block chan struct{}
	// Table is returned by Latest, with Base set to the requested base.
	Table model.RateTable
	// Err, if non-nil, is returned by every call. Use SetErr once the
	// repository is shared with other goroutines.
	Err error

	mu     sync.Mutex
	ctxErr error
}

var _ repository.RateRepository = (*FakeRepository)(nil)

// Latest implements repository.RateRepository.
func (f *FakeRepository) Latest(ctx context.Context, base string) (model.RateTable, error) {
	f.Calls.Add(1)
	f.mu.Lock()
	f.ctxErr = ctx.Err()
	f.mu.Unlock()
	if f.Block != nil {
		<-f.Block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return model.RateTable{}, f.Err
	}
	t := f.Table
	t.Base = base
	return t, nil
}

// Currencies implements repository.RateRepository.
func (f *FakeRepository) Currencies(context.Context) ([]model.Currency, error) {
	f.Calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	return []model.Currency{{Code: "EUR", Name: "Euro"}}, nil
}

// SetErr changes the error returned by subsequent calls.
func (f *FakeRepository) SetErr(err error) {
	f.mu.Lock()
	f.Err = err
	f.mu.Unlock()
}

// LastCtxErr returns ctx.Err() as observed by the most recent Latest call.
func (f *FakeRepository) LastCtxErr() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxErr
}

// FakeService is a service.RateService that records its inputs and returns
// a rate of 1.5 for every requested target.
type FakeService struct {
	GotBase    string
	GotTargets []string
	Err        error
	// PanicMsg, if set, makes Rates panic to exercise recovery middleware.
	PanicMsg string
}

var _ service.RateService = (*FakeService)(nil)

// Rates implements service.RateService.
func (f *FakeService) Rates(_ context.Context, base string, targets []string) (model.RateResult, error) {
	if f.PanicMsg != "" {
		panic(f.PanicMsg)
	}
	f.GotBase, f.GotTargets = base, targets
	if f.Err != nil {
		return model.RateResult{}, f.Err
	}
	out := make(map[string]float64, len(targets))
	for _, t := range targets {
		out[t] = 1.5
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return model.RateResult{Base: base, Date: "2026-01-01", Rates: out, FetchedAt: now, ExpiresAt: now.Add(time.Hour), Cached: true}, nil
}

// Currencies implements service.RateService.
func (f *FakeService) Currencies(context.Context) ([]model.Currency, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return []model.Currency{{Code: "EUR", Name: "Euro"}}, nil
}

// FakeClock is a manually advanced clock, safe for concurrent use.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a clock set to a fixed instant.
func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

// Now returns the clock's current time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}
