package bootstrap_test

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/transfer"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestImportsRunOneAtATime holds imports open in their transactions and
// checks that a second import for the same person is refused, and that the
// server runs at most transfer.MaxRunning imports at once.
func TestImportsRunOneAtATime(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	ben := env.SignIn(t, "ben@example.com", "Ben").User.ID
	cara := env.SignIn(t, "cara@example.com", "Cara").User.ID
	bundle := func() transfer.Bundle {
		return transfer.Bundle{Me: uuid.New(), People: []transfer.Person{{ID: uuid.New(), Name: "Sam"}}}
	}

	// Adding a name-only person checks the reference to its creator, which
	// waits while this transaction holds the creator's row. Ada's and Ben's
	// imports have taken their locks by then and stay open until it ends.
	hold, err := env.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback(ctx) //nolint:errcheck
	if _, err := hold.Exec(ctx, `SELECT 1 FROM users WHERE id = ANY($1) FOR UPDATE`, []uuid.UUID{ada, ben}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	for _, id := range []uuid.UUID{ada, ben} {
		go func() {
			_, err := svc.Transfer.Import(ctx, id, bundle())
			done <- err
		}()
	}
	waiting(t, env.DB.Pool, 2)

	// Refused imports answer at once; the deadline only stops one that waits.
	refused, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := svc.Transfer.Import(refused, ada, bundle()); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("a second import for the same person must be refused, got %v", err)
	}
	if _, err := svc.Transfer.Import(refused, cara, bundle()); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("an import beyond %d at once must be refused, got %v", transfer.MaxRunning, err)
	}
	if err := hold.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("the imports that held the locks must finish: %v", err)
		}
	}
	if _, err := svc.Transfer.Import(ctx, cara, bundle()); err != nil {
		t.Fatalf("imports run again once the others end: %v", err)
	}
}

// waiting returns once n other sessions wait for a row lock. Advisory lock
// waits are left out: tests in other packages queue on the shared test
// database lock and would otherwise be counted.
func waiting(t *testing.T, db *pgxpool.Pool, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		var got int
		err := db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND wait_event <> 'advisory'`).Scan(&got)
		if err != nil {
			t.Fatalf("waiting for %d blocked sessions: %v", n, err)
		}
		if got >= n {
			return
		}
	}
}
