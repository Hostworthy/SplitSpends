package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PushSubscriptions implements push.Repository.
type PushSubscriptions struct{ db *database.DB }

// NewPushSubscriptions builds PushSubscriptions.
func NewPushSubscriptions(db *database.DB) *PushSubscriptions { return &PushSubscriptions{db: db} }

const pushColumns = `id, user_id, session_id, created_at, updated_at, last_success_at, failures, platform, endpoint, p256dh, auth, label`

func scanPush(row pgx.Row) (*push.Subscription, error) {
	var s push.Subscription
	var session *uuid.UUID
	err := row.Scan(&s.ID, &s.UserID, &session, &s.CreatedAt, &s.UpdatedAt, &s.LastSuccessAt, &s.Failures, &s.Platform, &s.Endpoint, &s.P256dh, &s.Auth, &s.Label)
	if session != nil {
		s.SessionID = *session
	}
	return &s, err
}

// optional stores uuid.Nil as NULL.
func optional(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// Save holds a lock on the user while it counts and writes, so concurrent
// registrations for one user see each other's rows.
func (r *PushSubscriptions) Save(ctx context.Context, s *push.Subscription, limit int) (bool, error) {
	var created bool
	err := database.NewTx(r.db).Do(ctx, func(ctx context.Context) error {
		q := r.db.Q(ctx)
		if _, err := q.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR NO KEY UPDATE`, s.UserID); err != nil {
			return err
		}
		var others int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = $1 AND endpoint <> $2`,
			s.UserID, s.Endpoint).Scan(&others); err != nil {
			return err
		}
		if others >= limit {
			return push.ErrFull
		}
		return q.QueryRow(ctx, `
		INSERT INTO push_subscriptions (id, user_id, session_id, created_at, updated_at, platform, endpoint, p256dh, auth, label)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = EXCLUDED.user_id, session_id = EXCLUDED.session_id, updated_at = EXCLUDED.updated_at,
			platform = EXCLUDED.platform, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth, label = EXCLUDED.label,
			failures = CASE WHEN push_subscriptions.p256dh = EXCLUDED.p256dh AND push_subscriptions.auth = EXCLUDED.auth
				THEN push_subscriptions.failures ELSE 0 END
		RETURNING id, created_at, last_success_at, failures, (xmax = 0)`,
			s.ID, s.UserID, optional(s.SessionID), s.CreatedAt, s.UpdatedAt, s.Platform, s.Endpoint, s.P256dh, s.Auth, s.Label,
		).Scan(&s.ID, &s.CreatedAt, &s.LastSuccessAt, &s.Failures, &created)
	})
	if database.IsForeignKeyViolation(err, "push_subscriptions_session_id_fkey") {
		return false, push.ErrSignedOut
	}
	return created, err
}

func (r *PushSubscriptions) List(ctx context.Context, userID uuid.UUID) ([]*push.Subscription, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+pushColumns+` FROM push_subscriptions WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanPush)
}

func (r *PushSubscriptions) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errs.NotFound("Device")
	}
	return nil
}

func (r *PushSubscriptions) Remove(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id)
	return err
}

func (r *PushSubscriptions) Succeeded(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE push_subscriptions SET last_success_at = $2, failures = 0 WHERE id = $1`, id, at)
	return err
}

func (r *PushSubscriptions) Failed(ctx context.Context, id uuid.UUID) (int, error) {
	var n int
	err := r.db.Q(ctx).QueryRow(ctx, `UPDATE push_subscriptions SET failures = failures + 1 WHERE id = $1 RETURNING failures`, id).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// VAPIDKeys stores the server's generated VAPID key pair.
type VAPIDKeys struct{ db *database.DB }

// NewVAPIDKeys builds VAPIDKeys.
func NewVAPIDKeys(db *database.DB) *VAPIDKeys { return &VAPIDKeys{db: db} }

// Ensure stores the candidate pair unless one is stored already, and
// returns the stored pair. Concurrent first starts agree on one pair.
func (r *VAPIDKeys) Ensure(ctx context.Context, public, private string) (string, string, error) {
	q := r.db.Q(ctx)
	if _, err := q.Exec(ctx, `INSERT INTO vapid_keys (id, public_key, private_key) VALUES (1, $1, $2) ON CONFLICT (id) DO NOTHING`, public, private); err != nil {
		return "", "", fmt.Errorf("database.vapid-keys.save.failed: %w", err)
	}
	if err := q.QueryRow(ctx, `SELECT public_key, private_key FROM vapid_keys WHERE id = 1`).Scan(&public, &private); err != nil {
		return "", "", fmt.Errorf("database.vapid-keys.load.failed: %w", err)
	}
	return public, private, nil
}
