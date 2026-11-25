package bootstrap_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/rates"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/exchange"
)

// TestRatesCache stores a fetched table in PostgreSQL, serves it without
// refetching while fresh, and keeps serving it when the provider fails.
func TestRatesCache(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	env := bootstraptest.Open(t, bootstraptest.Options{Now: now})
	ctx := context.Background()

	var calls, failing atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if failing.Load() == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"result":"success","provider":"https://provider.test","time_last_update_unix":1795996801,"base_code":"USD","rates":{"USD":1,"EUR":0.912,"JPY":159.07}}`))
	}))
	defer srv.Close()

	svc := rates.NewService(repositories.NewRates(env.DB), exchange.New(srv.URL), env.Clock, slog.New(slog.DiscardHandler))

	if err := svc.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Latest(ctx)
	if err != nil || got == nil {
		t.Fatalf("Latest = %v, %v", got, err)
	}
	if calls.Load() != 1 || got.Base != "USD" || got.Source != "https://provider.test" || !got.Fetched.Equal(now) {
		t.Fatalf("calls=%d table=%+v", calls.Load(), got)
	}
	if v, ok := got.Convert(rates.Amount{Currency: "EUR", Minor: 5000}, "USD"); !ok || v != 5482 {
		t.Fatalf("convert = %d %v", v, ok)
	}

	env.Clock.Add(rates.Fresh + time.Hour)
	failing.Store(1)
	stale, err := svc.Latest(ctx)
	if err != nil || stale == nil || calls.Load() != 2 || !stale.Fetched.Equal(now) {
		t.Fatalf("stale fallback: calls=%d table=%+v err=%v", calls.Load(), stale, err)
	}

	failing.Store(0)
	fresh, err := svc.Latest(ctx)
	if err != nil || fresh == nil || !fresh.Fetched.Equal(env.Clock.Now()) {
		t.Fatalf("refetch: %+v %v", fresh, err)
	}
	var rows int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM exchange_rates`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows = %d, %v", rows, err)
	}
}

// TestRatesRoundTrip saves rates with more decimals than a float holds and
// reads back exactly the same values.
func TestRatesRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	env := bootstraptest.Open(t, bootstraptest.Options{Now: now})
	ctx := context.Background()

	long := "0." + strings.Repeat("123456789", 5)
	table, ok := rates.New("USD", map[string]string{"EUR": long, "JPY": "1.5e-7"})
	if !ok {
		t.Fatal("table rejected")
	}
	table.Fetched, table.Published = now, now
	repo := repositories.NewRates(env.DB)
	if err := repo.Save(ctx, table); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Latest(ctx)
	if err != nil || got == nil {
		t.Fatalf("Latest = %v, %v", got, err)
	}
	for _, code := range []string{"EUR", "JPY"} {
		want, _ := table.Rate(code)
		if r, ok := got.Rate(code); !ok || r.Cmp(want) != 0 {
			t.Errorf("%s = %v, want %v", code, r, want)
		}
	}
}
