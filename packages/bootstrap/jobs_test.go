package bootstrap_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
)

// broken is the expense repository failing to claim one series.
type broken struct {
	*repositories.Expenses
	id uuid.UUID
}

func (b broken) Claim(ctx context.Context, id uuid.UUID, day time.Time) (*expense.Expense, error) {
	if id == b.id {
		return nil, errors.New("database.claim.failed")
	}
	return b.Expenses.Claim(ctx, id, day)
}

func TestJobsRunPastFailures(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	bad := series(t, env, ada, ben, &g.ID, "Gym", "2026-03-01", expense.RecurMonthly)
	series(t, env, ada, ben, &g.ID, "Rent", "2026-03-10", expense.RecurMonthly)
	// Ada's session from flat expires after 90 days.
	env.Clock.Add(100 * 24 * time.Hour)

	var logs bytes.Buffer
	services := *env.Services
	services.Expenses = wired(env, broken{repositories.NewExpenses(env.DB), bad.ID})
	rt := &bootstrap.Runtime{Config: env.Config, Log: slog.New(slog.NewTextHandler(&logs, nil)), Services: &services}
	err := rt.RunJobsOnce(ctx)
	var skipped *expense.Skipped
	if !errors.As(err, &skipped) || skipped.ID != bad.ID {
		t.Fatalf("the run must report the series it skipped, got %v", err)
	}
	if !strings.Contains(logs.String(), "jobs.recurring.skipped") {
		t.Fatalf("the skipped series must be logged: %s", logs.String())
	}

	var gym, rent int
	for _, e := range groupExpenses(t, env, ada, g.ID) {
		switch e.Description {
		case "Gym":
			gym++
		case "Rent":
			rent++
		}
	}
	if gym != 1 || rent != 4 {
		t.Fatalf("want the failing series alone and Rent booked through June, got %d Gym and %d Rent", gym, rent)
	}
	var sessions int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("expired sessions must be removed although a series failed, %d left", sessions)
	}
}

// TestJobsPruneQuotas checks that the jobs remove budgets whose window has
// ended, which a new use would start afresh anyway, and keep running ones.
func TestJobsPruneQuotas(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	quotas := repositories.NewQuotas(env.DB)
	take := func(key string) {
		t.Helper()
		if ok, err := quotas.Take(ctx, key, 3, 24*time.Hour, env.Clock.Now()); !ok || err != nil {
			t.Fatalf("take %s: %v, %v", key, ok, err)
		}
	}
	take("ended")
	env.Clock.Add(25 * time.Hour)
	take("running")

	rt := &bootstrap.Runtime{Config: env.Config, Log: slog.New(slog.DiscardHandler), Services: env.Services}
	if err := rt.RunJobsOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var keys []string
	rows, err := env.DB.Pool.Query(ctx, `SELECT key FROM quotas ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "running" {
		t.Fatalf("only the running budget must stay, got %v", keys)
	}
}
