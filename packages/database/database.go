// Package database implements the core repository ports on PostgreSQL with
// pgx. Repositories hold no business rules.
package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Connect opens a pool and verifies it.
func Connect(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database.config.invalid: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database.connect.failed: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database.ping.failed: %w", err)
	}
	return pool, nil
}

// migrateLock is a fixed advisory lock id so concurrent cold starts (several
// serverless instances) apply migrations one at a time.
const migrateLock = 7_301_451_982

// migrateTimeout bounds waiting for another instance's migrations and
// applying ours, so a stuck start fails instead of hanging.
const migrateTimeout = 5 * time.Minute

// Migrate applies every pending migration. The lock is a session lock on a
// connection of its own, outside the pool, so the migrations can use the
// pool even when it holds a single connection; that connection must reach
// PostgreSQL directly or through a session pooler.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, migrateTimeout)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	if err != nil {
		return fmt.Errorf("database.migrate.lock.failed: %w", err)
	}
	// Closing the connection also releases the lock.
	defer conn.Close(context.Background()) //nolint:errcheck
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, migrateLock).Scan(&locked); err != nil {
		return fmt.Errorf("database.migrate.lock.failed: %w", err)
	}
	if !locked {
		log.Info("database.migrate.waiting")
		if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLock); err != nil {
			return fmt.Errorf("database.migrate.lock.failed: %w", err)
		}
	}
	files, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return fmt.Errorf("database.migrate.failed: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("database.migrate.failed: %w", err)
	}
	return nil
}

// Querier is satisfied by both the pool and a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type txKey struct{}

// DB wraps the pool and resolves the active transaction from ctx.
type DB struct {
	Pool *pgxpool.Pool
}

// Q returns the transaction in ctx, or the pool.
func (d *DB) Q(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return d.Pool
}

// Tx implements ports.Tx.
type Tx struct{ db *DB }

// NewTx builds a Tx.
func NewTx(db *DB) *Tx { return &Tx{db: db} }

// Do runs fn in a transaction, joining an outer one if present.
func (t *Tx) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	return pgx.BeginFunc(ctx, t.db.Pool, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// TryLock implements ports.Locker with a transaction-level advisory lock.
func (t *Tx) TryLock(ctx context.Context, key string) (bool, error) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	if !ok {
		return false, errors.New("database.lock.outside-transaction")
	}
	var got bool
	err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, key).Scan(&got)
	return got, err
}

// IsUniqueViolation reports a unique constraint error.
func IsUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
