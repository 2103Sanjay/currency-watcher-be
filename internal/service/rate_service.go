// Package service holds the business logic: answering rate queries from an
// in-memory cache backed by a repository.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/2103Sanjay/currency-watcher-be/internal/model"
	"github.com/2103Sanjay/currency-watcher-be/internal/repository"
)

// fetchTimeout bounds a single upstream fetch. The fetch is detached from the
// caller's context (it is shared by every concurrent caller via singleflight),
// so it needs its own deadline.
const fetchTimeout = 15 * time.Second

// RateService answers exchange-rate queries.
type RateService interface {
	// Rates returns the rates from base to each of targets. If targets is
	// empty, every rate available for base is returned. Codes must already
	// be normalised to upper case.
	Rates(ctx context.Context, base string, targets []string) (model.RateResult, error)
	// Currencies returns the list of supported currencies.
	Currencies(ctx context.Context) ([]model.Currency, error)
}

type tableEntry struct {
	table     model.RateTable
	fetchedAt time.Time
}

type currenciesEntry struct {
	list      []model.Currency
	fetchedAt time.Time
}

// CachedRateService is a RateService that answers from an in-memory cache,
// refreshing entries from the repository once they are older than the TTL.
//
// The cache is keyed by base currency and holds the repository's full rate
// table for that base, so any combination of targets for the same base is
// served by one upstream call. Concurrent misses for the same base are
// collapsed into a single upstream request. If a refresh fails, the expired
// entry is served and flagged as stale rather than failing the request.
//
// CachedRateService is safe for concurrent use.
type CachedRateService struct {
	repo   repository.RateRepository
	ttl    time.Duration
	logger *slog.Logger
	now    func() time.Time

	mu         sync.RWMutex
	tables     map[string]tableEntry
	currencies *currenciesEntry

	flight singleflight.Group
}

var _ RateService = (*CachedRateService)(nil)

// Option configures a CachedRateService.
type Option func(*CachedRateService)

// WithClock overrides the time source used for cache expiry. Intended for
// tests.
func WithClock(now func() time.Time) Option {
	return func(s *CachedRateService) { s.now = now }
}

// NewCachedRateService returns a service that caches repository responses
// for ttl.
func NewCachedRateService(repo repository.RateRepository, ttl time.Duration, logger *slog.Logger, opts ...Option) *CachedRateService {
	s := &CachedRateService{
		repo:   repo,
		ttl:    ttl,
		logger: logger,
		now:    time.Now,
		tables: make(map[string]tableEntry),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Rates implements RateService.
func (s *CachedRateService) Rates(ctx context.Context, base string, targets []string) (model.RateResult, error) {
	entry, cached, stale, err := s.table(ctx, base)
	if err != nil {
		return model.RateResult{}, err
	}

	rates, err := pick(entry.table, targets)
	if err != nil {
		return model.RateResult{}, err
	}

	return model.RateResult{
		Base:      entry.table.Base,
		Date:      entry.table.Date,
		Rates:     rates,
		FetchedAt: entry.fetchedAt,
		ExpiresAt: entry.fetchedAt.Add(s.ttl),
		Cached:    cached,
		Stale:     stale,
	}, nil
}

// Currencies implements RateService. The list is cached for the TTL.
func (s *CachedRateService) Currencies(ctx context.Context) ([]model.Currency, error) {
	s.mu.RLock()
	existing := s.currencies
	s.mu.RUnlock()

	if existing != nil && s.fresh(existing.fetchedAt) {
		return existing.list, nil
	}

	v, err, _ := s.flight.Do("currencies", func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()

		list, err := s.repo.Currencies(fetchCtx)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.currencies = &currenciesEntry{list: list, fetchedAt: s.now()}
		s.mu.Unlock()
		return list, nil
	})
	if err != nil {
		if existing != nil {
			s.logger.Warn("serving stale currency list", "error", err)
			return existing.list, nil
		}
		return nil, err
	}
	return v.([]model.Currency), nil
}

// table returns the cached table for base, fetching it if missing or expired.
func (s *CachedRateService) table(ctx context.Context, base string) (entry tableEntry, cached, stale bool, err error) {
	s.mu.RLock()
	existing, ok := s.tables[base]
	s.mu.RUnlock()

	if ok && s.fresh(existing.fetchedAt) {
		return existing, true, false, nil
	}

	v, err, _ := s.flight.Do("rates:"+base, func() (any, error) {
		// Another flight may have refreshed the entry between our read and
		// acquiring this one.
		s.mu.RLock()
		latest, ok := s.tables[base]
		s.mu.RUnlock()
		if ok && s.fresh(latest.fetchedAt) {
			return latest, nil
		}

		// The fetch is shared by every caller waiting on this key, so one
		// caller cancelling must not abort it for the others.
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()

		table, err := s.repo.Latest(fetchCtx, base)
		if err != nil {
			return nil, err
		}
		fetched := tableEntry{table: table, fetchedAt: s.now()}
		s.mu.Lock()
		s.tables[base] = fetched
		s.mu.Unlock()
		return fetched, nil
	})
	if err != nil {
		if ok && !errors.Is(err, model.ErrUnknownCurrency) {
			s.logger.Warn("upstream refresh failed; serving stale rates", "base", base, "error", err)
			return existing, true, true, nil
		}
		return tableEntry{}, false, false, err
	}
	return v.(tableEntry), false, false, nil
}

func (s *CachedRateService) fresh(fetchedAt time.Time) bool {
	return s.now().Before(fetchedAt.Add(s.ttl))
}

// pick copies the requested targets out of table so callers never share the
// cached map.
func pick(table model.RateTable, targets []string) (map[string]float64, error) {
	if len(targets) == 0 {
		out := make(map[string]float64, len(table.Rates))
		for code, rate := range table.Rates {
			out[code] = rate
		}
		return out, nil
	}

	out := make(map[string]float64, len(targets))
	var missing []string
	for _, code := range targets {
		if code == table.Base {
			out[code] = 1
			continue
		}
		rate, ok := table.Rates[code]
		if !ok {
			missing = append(missing, code)
			continue
		}
		out[code] = rate
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", model.ErrUnknownCurrency, strings.Join(missing, ", "))
	}
	return out, nil
}
