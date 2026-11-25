package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/jackc/pgx/v5"
)

// Quotas implements ports.Quota.
type Quotas struct{ db *database.DB }

// NewQuotas builds Quotas.
func NewQuotas(db *database.DB) *Quotas { return &Quotas{db: db} }

// Take starts a new window when the current one has ended and otherwise
// counts one more use while any are left; no returned row means none were.
func (r *Quotas) Take(ctx context.Context, key string, limit int, window time.Duration, now time.Time) (bool, error) {
	var used int
	err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO quotas (key, started_at, ends_at, used) VALUES ($1, $2, $3, 1)
		ON CONFLICT (key) DO UPDATE SET
			used = CASE WHEN quotas.ends_at <= $2 THEN 1 ELSE quotas.used + 1 END,
			started_at = CASE WHEN quotas.ends_at <= $2 THEN EXCLUDED.started_at ELSE quotas.started_at END,
			ends_at = CASE WHEN quotas.ends_at <= $2 THEN EXCLUDED.ends_at ELSE quotas.ends_at END
		WHERE quotas.ends_at <= $2 OR quotas.used < $4
		RETURNING used`, key, now, now.Add(window), limit).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Prune removes the budgets whose window ended by now. Take would start each
// of them afresh, so removing one changes no budget.
func (r *Quotas) Prune(ctx context.Context, now time.Time) (int64, error) {
	tag, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM quotas WHERE ends_at <= $1`, now)
	return tag.RowsAffected(), err
}
