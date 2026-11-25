package bootstrap_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
)

// held is the expense repository with a hook that runs once, right after
// the first read of an expense for any purpose.
type held struct {
	*repositories.Expenses
	once  *sync.Once
	after func()
}

func (h held) Get(ctx context.Context, id uuid.UUID) (*expense.Expense, error) {
	e, err := h.Expenses.Get(ctx, id)
	h.once.Do(h.after)
	return e, err
}

func (h held) Lock(ctx context.Context, id uuid.UUID) (*expense.Expense, error) {
	e, err := h.Expenses.Lock(ctx, id)
	h.once.Do(h.after)
	return e, err
}

// blocked reports whether a session on the test database waits for a row
// lock.
func blocked(t *testing.T, env *bootstraptest.Env) bool {
	t.Helper()
	var ok bool
	err := env.DB.Pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
		WHERE datname = current_database() AND wait_event_type = 'Lock' AND wait_event IN ('transactionid', 'tuple'))`).Scan(&ok)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

// rent is the edit Ada makes to the rent: a new total, shared equally.
func rent(ada, ben uuid.UUID, total int64, at *time.Time) expense.Input {
	return expense.Input{Description: "Rent", Total: total, Date: "2026-03-01", Method: expense.MethodEqual, UpdatedAt: at,
		Payers: []expense.Payer{{UserID: ada, Amount: total}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}}
}

func TestUpdateKeepsConcurrentDelete(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	e := series(t, env, ada, ben, &g.ID, "Rent", "2026-03-01", expense.RecurNone)
	deleted := make(chan error, 1)
	finished := make(chan struct{})
	// Once the edit has read the expense, Ben deletes it. The edit goes on
	// when the delete has finished or is waiting for the edit to finish.
	svc := wired(env, held{repositories.NewExpenses(env.DB), &sync.Once{}, func() {
		go func() {
			deleted <- env.Services.Expenses.Delete(ctx, ben, e.ID)
			close(finished)
		}()
		for {
			select {
			case <-finished:
				return
			default:
			}
			if blocked(t, env) {
				return
			}
		}
	}})
	_, err := svc.Update(ctx, ada, e.ID, rent(ada, ben, 120000, nil))
	if err != nil && !errs.Is(err, errs.KindConflict) {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	got, err := env.Services.Expenses.Get(ctx, ada, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeletedAt == nil {
		t.Fatal("the edit brought back an expense Ben deleted")
	}
}

func TestUpdateChecksVersion(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services.Expenses
	e := series(t, env, ada, ben, &g.ID, "Rent", "2026-03-01", expense.RecurNone)
	seen := e.UpdatedAt
	env.Clock.Add(time.Minute)
	if _, err := svc.Update(ctx, ben, e.ID, rent(ada, ben, 110000, &seen)); err != nil {
		t.Fatalf("an edit of the current version must be saved: %v", err)
	}
	env.Clock.Add(time.Minute)
	_, err := svc.Update(ctx, ada, e.ID, rent(ada, ben, 120000, &seen))
	if !errs.Is(err, errs.KindConflict) {
		t.Fatalf("an edit of an older version must be refused with a conflict, got %v", err)
	}
	got, err := svc.Get(ctx, ada, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 110000 {
		t.Fatalf("the refused edit changed the expense: total %d", got.Total)
	}
	if _, err := svc.Update(ctx, ada, e.ID, rent(ada, ben, 120000, nil)); err != nil {
		t.Fatalf("an edit without a version must be saved: %v", err)
	}
}
