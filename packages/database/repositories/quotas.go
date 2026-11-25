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
	err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO quotas (key, started_at, used) VALUES ($1, $2, 1)
		ON CONFLICT (key) DO UPDATE SET
			used = CASE WHEN quotas.started_at <= $3 THEN 1 ELSE quotas.used + 1 END,
			started_at = CASE WHEN quotas.started_at <= $3 THEN EXCLUDED.started_at ELSE quotas.started_at END
		WHERE quotas.started_at <= $3 OR quotas.used < $4
		RETURNING used`, key, now, now.Add(-window), limit).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
