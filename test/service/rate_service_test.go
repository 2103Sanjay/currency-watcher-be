package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/model"
	"github.com/2103Sanjay/currency-watcher-be/internal/repository"
	"github.com/2103Sanjay/currency-watcher-be/internal/service"
	"github.com/2103Sanjay/currency-watcher-be/test/testutil"
)

func newTestService(repo repository.RateRepository, ttl time.Duration) (*service.CachedRateService, *testutil.FakeClock) {
	clock := testutil.NewFakeClock()
	svc := service.NewCachedRateService(repo, ttl, testutil.DiscardLogger(), service.WithClock(clock.Now))
	return svc, clock
}

func TestRates_ServesFromCacheWithinTTL(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, clock := newTestService(p, time.Hour)
	ctx := context.Background()

	first, err := svc.Rates(ctx, "USD", []string{"EUR"})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if first.Cached {
		t.Error("first call should not be served from cache")
	}

	clock.Advance(59 * time.Minute)

	// A different target for the same base must reuse the cached table.
	second, err := svc.Rates(ctx, "USD", []string{"SGD"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !second.Cached {
		t.Error("second call should be served from cache")
	}
	if got := p.Calls.Load(); got != 1 {
		t.Errorf("repository calls = %d, want 1", got)
	}
	if second.Rates["SGD"] != 1.3 {
		t.Errorf("SGD rate = %v, want 1.3", second.Rates["SGD"])
	}
}

func TestRates_RefetchesAfterTTL(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, clock := newTestService(p, time.Hour)
	ctx := context.Background()

	if _, err := svc.Rates(ctx, "USD", []string{"EUR"}); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Hour)

	res, err := svc.Rates(ctx, "USD", []string{"EUR"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached {
		t.Error("expired entry should have been refreshed")
	}
	if got := p.Calls.Load(); got != 2 {
		t.Errorf("repository calls = %d, want 2", got)
	}
	if want := clock.Now().Add(time.Hour); !res.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", res.ExpiresAt, want)
	}
}

func TestRates_CachesPerBase(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, _ := newTestService(p, time.Hour)
	ctx := context.Background()

	for _, base := range []string{"USD", "SGD", "USD", "SGD"} {
		if _, err := svc.Rates(ctx, base, []string{"EUR"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := p.Calls.Load(); got != 2 {
		t.Errorf("repository calls = %d, want 2 (one per base)", got)
	}
}

func TestRates_ServesStaleWhenUpstreamFails(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, clock := newTestService(p, time.Hour)
	ctx := context.Background()

	if _, err := svc.Rates(ctx, "USD", []string{"EUR"}); err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Hour)
	p.SetErr(errors.New("upstream down"))

	res, err := svc.Rates(ctx, "USD", []string{"EUR"})
	if err != nil {
		t.Fatalf("expected stale result, got error: %v", err)
	}
	if !res.Stale {
		t.Error("result should be flagged stale")
	}
	if res.Rates["EUR"] != 0.9 {
		t.Errorf("EUR = %v, want 0.9", res.Rates["EUR"])
	}
}

func TestRates_ErrorWhenUpstreamFailsWithoutCache(t *testing.T) {
	p := &testutil.FakeRepository{Err: errors.New("upstream down")}
	svc, _ := newTestService(p, time.Hour)

	if _, err := svc.Rates(context.Background(), "USD", []string{"EUR"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRates_UnknownTarget(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, _ := newTestService(p, time.Hour)

	_, err := svc.Rates(context.Background(), "USD", []string{"EUR", "XYZ"})
	if !errors.Is(err, model.ErrUnknownCurrency) {
		t.Fatalf("err = %v, want ErrUnknownCurrency", err)
	}
}

func TestRates_BaseAsTargetIsOne(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, _ := newTestService(p, time.Hour)

	res, err := svc.Rates(context.Background(), "USD", []string{"USD"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rates["USD"] != 1 {
		t.Errorf("USD->USD = %v, want 1", res.Rates["USD"])
	}
}

func TestRates_EmptyTargetsReturnsAll(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, _ := newTestService(p, time.Hour)

	res, err := svc.Rates(context.Background(), "USD", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rates) != 3 {
		t.Errorf("got %d rates, want 3", len(res.Rates))
	}
}

func TestRates_ResultDoesNotAliasCache(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, _ := newTestService(p, time.Hour)
	ctx := context.Background()

	res, err := svc.Rates(ctx, "USD", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Rates["EUR"] = 999

	again, err := svc.Rates(ctx, "USD", []string{"EUR"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Rates["EUR"] != 0.9 {
		t.Errorf("cache was mutated through a returned map: EUR = %v", again.Rates["EUR"])
	}
}

func TestRates_ConcurrentMissesShareOneFetch(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable(), Block: make(chan struct{})}
	svc, _ := newTestService(p, time.Hour)

	const callers = 50
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Rates(context.Background(), "USD", []string{"EUR"})
			errs <- err
		}()
	}

	// Let the goroutines pile up on the in-flight fetch, then release it.
	time.Sleep(50 * time.Millisecond)
	close(p.Block)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if got := p.Calls.Load(); got != 1 {
		t.Errorf("repository calls = %d, want 1", got)
	}
}

func TestCurrencies_Cached(t *testing.T) {
	p := &testutil.FakeRepository{}
	svc, clock := newTestService(p, time.Hour)
	ctx := context.Background()

	for range 3 {
		if _, err := svc.Currencies(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := p.Calls.Load(); got != 1 {
		t.Errorf("repository calls = %d, want 1", got)
	}

	clock.Advance(2 * time.Hour)
	p.SetErr(errors.New("upstream down"))
	list, err := svc.Currencies(ctx)
	if err != nil || len(list) != 1 {
		t.Errorf("expected stale currency list, got %v, %v", list, err)
	}
}

func TestRates_UnknownBaseIsNotServedStale(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, clock := newTestService(p, time.Hour)
	ctx := context.Background()

	if _, err := svc.Rates(ctx, "USD", []string{"EUR"}); err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Hour)
	// The provider now rejects the currency outright (e.g. it was withdrawn),
	// which is a definitive answer rather than an outage.
	p.SetErr(fmt.Errorf("%w: USD", model.ErrUnknownCurrency))

	if _, err := svc.Rates(ctx, "USD", []string{"EUR"}); !errors.Is(err, model.ErrUnknownCurrency) {
		t.Fatalf("err = %v, want ErrUnknownCurrency", err)
	}
}

func TestRates_UpstreamErrorIsNotCached(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable(), Err: errors.New("upstream down")}
	svc, _ := newTestService(p, time.Hour)
	ctx := context.Background()

	if _, err := svc.Rates(ctx, "USD", []string{"EUR"}); err == nil {
		t.Fatal("expected error")
	}
	p.SetErr(nil)

	res, err := svc.Rates(ctx, "USD", []string{"EUR"})
	if err != nil {
		t.Fatalf("expected recovery once upstream is back, got %v", err)
	}
	if res.Cached || res.Stale {
		t.Errorf("result should be freshly fetched: %+v", res)
	}
}

func TestRates_FetchIgnoresCallerCancellation(t *testing.T) {
	p := &testutil.FakeRepository{Table: testutil.SampleTable()}
	svc, _ := newTestService(p, time.Hour)

	// The fetch is shared with other waiters, so a caller that has already
	// gone away must not abort it for everyone else.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.Rates(ctx, "USD", []string{"EUR"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := p.LastCtxErr(); err != nil {
		t.Errorf("repository saw cancelled context: %v", err)
	}
}

func TestCurrencies_ErrorWithoutCache(t *testing.T) {
	p := &testutil.FakeRepository{Err: errors.New("upstream down")}
	svc, _ := newTestService(p, time.Hour)

	if _, err := svc.Currencies(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestCurrencies_RefreshesAfterTTL(t *testing.T) {
	p := &testutil.FakeRepository{}
	svc, clock := newTestService(p, time.Hour)
	ctx := context.Background()

	if _, err := svc.Currencies(ctx); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Hour)
	if _, err := svc.Currencies(ctx); err != nil {
		t.Fatal(err)
	}
	if got := p.Calls.Load(); got != 2 {
		t.Errorf("repository calls = %d, want 2", got)
	}
}
