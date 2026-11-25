package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestMoneyChecks checks that the schema refuses amounts, currencies and
// split settings the domain never writes, so a bad write fails instead of
// changing balances.
func TestMoneyChecks(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	group, expense := uuid.New(), uuid.New()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO groups (id, created_by, created_at, updated_at, name, kind, default_currency)
			VALUES ($1, $2, now(), now(), 'Trip', 'trip', 'EUR')`, []any{group, ada}},
		{`INSERT INTO expenses (id, created_by, updated_by, created_at, updated_at, total, date, kind, description, category, currency, method)
			VALUES ($1, $2, $2, now(), now(), 100, '2026-01-01', 'expense', 'Lunch', 'general', 'EUR', 'equal')`, []any{expense, ada}},
		{`INSERT INTO expense_shares (expense_id, user_id, paid, owed) VALUES ($1, $2, 100, 100)`, []any{expense, ada}},
	} {
		if _, err := env.DB.Pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		sql  string
		arg  uuid.UUID
	}{
		{"negative owed", `UPDATE expense_shares SET owed = -1 WHERE expense_id = $1`, expense},
		{"negative paid", `UPDATE expense_shares SET paid = -1 WHERE expense_id = $1`, expense},
		{"negative tax", `UPDATE expenses SET tax = -1 WHERE id = $1`, expense},
		{"negative tip", `UPDATE expenses SET tip = -1 WHERE id = $1`, expense},
		{"lower-case currency", `UPDATE expenses SET currency = 'eur' WHERE id = $1`, expense},
		{"long currency", `UPDATE expenses SET currency = 'EURO' WHERE id = $1`, expense},
		{"unknown method", `UPDATE expenses SET method = 'split' WHERE id = $1`, expense},
		{"unknown recurrence", `UPDATE expenses SET recurrence = 'daily' WHERE id = $1`, expense},
		{"user currency", `UPDATE users SET default_currency = 'usd' WHERE id = $1`, ada},
		{"group currency", `UPDATE groups SET default_currency = '' WHERE id = $1`, group},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := env.DB.Pool.Exec(ctx, tt.sql, tt.arg)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("err = %v, want a check violation (23514)", err)
			}
		})
	}
}
