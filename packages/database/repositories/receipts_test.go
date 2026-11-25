package repositories_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/google/uuid"
)

// TestReceiptLookupUsesIndex checks that finding the expenses that use a
// receipt, which every receipt view by another member and every receipt
// removal does, reads the index instead of every expense.
func TestReceiptLookupUsesIndex(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	receipt := uuid.New()
	if _, err := env.DB.Pool.Exec(ctx, `INSERT INTO receipts (id, uploaded_by, created_at, size, content_type, storage_key)
		SELECT CASE WHEN n = 1 THEN $1 ELSE gen_random_uuid() END, $2, now(), 1, 'image/png', 'key' || n
		FROM generate_series(1, 100) n`, receipt, ada); err != nil {
		t.Fatal(err)
	}
	// Thousands of expenses, one in fifty with a receipt.
	if _, err := env.DB.Pool.Exec(ctx, `INSERT INTO expenses (id, receipt_id, created_by, updated_by, created_at, updated_at,
			total, date, kind, description, category, currency, method)
		SELECT gen_random_uuid(), CASE WHEN n % 50 = 0 THEN (SELECT id FROM receipts ORDER BY id OFFSET n / 50 % 100 LIMIT 1) END,
			$1, $1, now(), now(), 100, '2026-01-01', 'expense', 'Lunch', 'general', 'USD', 'equal'
		FROM generate_series(1, 5000) n`, ada); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `ANALYZE expenses`); err != nil {
		t.Fatal(err)
	}
	rows, err := env.DB.Pool.Query(ctx, `EXPLAIN SELECT 1 FROM expenses e WHERE e.receipt_id = $1`, receipt)
	if err != nil {
		t.Fatal(err)
	}
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if text := strings.Join(plan, "\n"); !strings.Contains(text, "expenses_receipt_idx") {
		t.Fatalf("looking up expenses by receipt should use expenses_receipt_idx:\n%s", text)
	}
}
