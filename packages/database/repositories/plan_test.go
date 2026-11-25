package repositories_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
)

// plan returns the plan PostgreSQL makes for query when every way to read a
// table but a bitmap index scan is turned off. The plan then names an index
// exactly when the query's condition can use it, whatever the table holds
// and whatever the costs, since a sequential scan is only chosen when nothing
// else is possible.
func plan(t *testing.T, env *bootstraptest.Env, query string, args ...any) string {
	t.Helper()
	ctx := context.Background()
	tx, err := env.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, setting := range []string{"enable_seqscan", "enable_indexscan", "enable_indexonlyscan"} {
		if _, err := tx.Exec(ctx, `SELECT set_config($1, 'off', true)`, setting); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := tx.Query(ctx, `EXPLAIN `+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}
