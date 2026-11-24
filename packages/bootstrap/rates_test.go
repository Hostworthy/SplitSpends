package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/rates"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/exchange"
)

// TestRatesCache stores a fetched table in PostgreSQL, serves it without
// refetching while fresh, and keeps serving it when the provider fails.
func TestRatesCache(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, url, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

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

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	c := &clock.Fixed{At: now}
	svc := rates.NewService(repositories.NewRates(&database.DB{Pool: pool}), exchange.New(srv.URL), c)

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

	c.At = now.Add(rates.Fresh + time.Hour)
	failing.Store(1)
	stale, err := svc.Latest(ctx)
	if err != nil || stale == nil || calls.Load() != 2 || !stale.Fetched.Equal(now) {
		t.Fatalf("stale fallback: calls=%d table=%+v err=%v", calls.Load(), stale, err)
	}

	failing.Store(0)
	fresh, err := svc.Latest(ctx)
	if err != nil || fresh == nil || !fresh.Fetched.Equal(c.At) {
		t.Fatalf("refetch: %+v %v", fresh, err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM exchange_rates`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows = %d, %v", rows, err)
	}
}
