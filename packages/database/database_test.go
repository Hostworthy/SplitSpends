package database_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/jackc/pgx/v5/pgxpool"
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
