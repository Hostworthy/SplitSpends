package database_test

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// empty drops the schema bootstraptest.Open migrated.
func empty(t *testing.T, env *bootstraptest.Env) {
	t.Helper()
	if _, err := env.DB.Pool.Exec(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatal(err)
	}
}

// connect opens a pool of at most conns connections on the test database.
func connect(t *testing.T, conns int32) *pgxpool.Pool {
	t.Helper()
	pool, err := database.Connect(context.Background(), os.Getenv("TEST_DATABASE_URL"), conns)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrateSingleConnection(t *testing.T) {
	env := bootstraptest.Open(t, bootstraptest.Options{})
	empty(t, env)
	pool := connect(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrating through a pool of one connection: %v", err)
	}
}

func TestMigrateConcurrent(t *testing.T) {
	env := bootstraptest.Open(t, bootstraptest.Options{})
	empty(t, env)
	pools := []*pgxpool.Pool{connect(t, 2), connect(t, 2)}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, len(pools))
	for _, pool := range pools {
		go func() {
			<-start
			results <- database.Migrate(ctx, pool, slog.New(slog.DiscardHandler))
		}()
	}
	close(start)
	for range pools {
		if err := <-results; err != nil {
			t.Fatalf("both starts should migrate: %v", err)
		}
	}
	var twice int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT version_id FROM goose_db_version
		GROUP BY version_id HAVING count(*) > 1) x`).Scan(&twice); err != nil {
		t.Fatal(err)
	}
	if twice != 0 {
		t.Fatalf("%d migrations were applied twice", twice)
	}
}

// TestMoneyChecksKeepLegacyRows checks that adding the money checks never
// stops a server from starting: currency codes are normalized, and a check
// that rows written before it would fail is left unvalidated but still
// refuses new writes.
func TestMoneyChecksKeepLegacyRows(t *testing.T) {
	env := bootstraptest.Open(t, bootstraptest.Options{})
	empty(t, env)
	ctx := context.Background()
	files, err := fs.Sub(database.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDBFromPool(env.DB.Pool)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		t.Fatal(err)
	}
	var before int64 = -1
	for _, s := range provider.ListSources() {
		if strings.HasSuffix(s.Path, "_money_checks.sql") {
			break
		}
		before = s.Version
	}
	if before < 0 {
		t.Fatal("no money checks migration")
	}
	if _, err := provider.UpTo(ctx, before); err != nil {
		t.Fatal(err)
	}
	user, expense := uuid.New(), uuid.New()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, created_at, updated_at, email, name, default_currency, avatar_color, status)
			VALUES ($1, now(), now(), 'ada@example.com', 'Ada', ' usd', '#000000', 'active')`, []any{user}},
		{`INSERT INTO expenses (id, created_by, updated_by, created_at, updated_at, total, tax, date, kind, description, category, currency, method)
			VALUES ($1, $2, $2, now(), now(), 100, -1, '2026-01-01', 'expense', 'Lunch', 'general', 'eur', 'equal')`, []any{expense, user}},
		{`INSERT INTO expense_shares (expense_id, user_id, paid, owed) VALUES ($1, $2, 100, 100)`, []any{expense, user}},
	} {
		if _, err := env.DB.Pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migrating a database with rows the checks refuse: %v", err)
	}

	var userCurrency, expenseCurrency string
	if err := env.DB.Pool.QueryRow(ctx, `SELECT u.default_currency, e.currency FROM users u, expenses e WHERE u.id = $1 AND e.id = $2`,
		user, expense).Scan(&userCurrency, &expenseCurrency); err != nil {
		t.Fatal(err)
	}
	if userCurrency != "USD" || expenseCurrency != "EUR" {
		t.Errorf("currencies = %q, %q, want USD, EUR", userCurrency, expenseCurrency)
	}
	rows, err := env.DB.Pool.Query(ctx, `SELECT conname FROM pg_constraint
		WHERE conname IN ('expenses_tax_check', 'expenses_tip_check', 'expenses_currency_check', 'users_default_currency_check')
		AND NOT convalidated`)
	if err != nil {
		t.Fatal(err)
	}
	unvalidated, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(unvalidated) != 1 || unvalidated[0] != "expenses_tax_check" {
		t.Errorf("unvalidated checks = %v, want only expenses_tax_check", unvalidated)
	}
	_, err = env.DB.Pool.Exec(ctx, `UPDATE expenses SET tax = -2 WHERE id = $1`, expense)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("a new negative tax: err = %v, want a check violation (23514)", err)
	}
}
