package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Challenges implements auth.ChallengeRepository.
type Challenges struct{ db *database.DB }

// NewChallenges builds Challenges.
func NewChallenges(db *database.DB) *Challenges { return &Challenges{db: db} }

func (r *Challenges) Create(ctx context.Context, c *auth.Challenge) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO login_challenges (id, expires_at, consumed_at, created_at, attempts, email, code_hash) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.ExpiresAt, c.ConsumedAt, c.CreatedAt, c.Attempts, c.Email, c.CodeHash)
	return err
}

func (r *Challenges) Latest(ctx context.Context, email string) (*auth.Challenge, error) {
	var c auth.Challenge
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT id, expires_at, consumed_at, created_at, attempts, email, code_hash FROM login_challenges WHERE email = $1 ORDER BY created_at DESC LIMIT 1`, email).
		Scan(&c.ID, &c.ExpiresAt, &c.ConsumedAt, &c.CreatedAt, &c.Attempts, &c.Email, &c.CodeHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}

func (r *Challenges) Update(ctx context.Context, c *auth.Challenge) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE login_challenges SET attempts=$2, consumed_at=$3 WHERE id=$1`, c.ID, c.Attempts, c.ConsumedAt)
	return err
}

func (r *Challenges) CountSince(ctx context.Context, email string, since time.Time) (int, error) {
	var n int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM login_challenges WHERE email=$1 AND created_at >= $2`, email, since).Scan(&n)
	return n, err
}

func (r *Challenges) DeleteBefore(ctx context.Context, before time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM login_challenges WHERE created_at < $1`, before)
	return err
}

// Sessions implements auth.SessionRepository.
type Sessions struct{ db *database.DB }

// NewSessions builds Sessions.
func NewSessions(db *database.DB) *Sessions { return &Sessions{db: db} }

const sessionColumns = `id, user_id, expires_at, last_used_at, created_at, kind, name, token_hash, user_agent`

func scanSession(row pgx.Row) (*auth.Session, error) {
	var s auth.Session
	err := row.Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.LastUsedAt, &s.CreatedAt, &s.Kind, &s.Name, &s.TokenHash, &s.UserAgent)
	return &s, err
}

func (r *Sessions) Create(ctx context.Context, s *auth.Session) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO sessions (`+sessionColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		s.ID, s.UserID, s.ExpiresAt, s.LastUsedAt, s.CreatedAt, s.Kind, s.Name, s.TokenHash, s.UserAgent)
	return err
}

func (r *Sessions) FindByHash(ctx context.Context, hash string) (*auth.Session, error) {
	s, err := scanSession(r.db.Q(ctx).QueryRow(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE token_hash=$1`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func (r *Sessions) Touch(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE sessions SET last_used_at=$2 WHERE id=$1`, id, at)
	return err
}

func (r *Sessions) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM sessions WHERE id=$1`, id)
	return err
}

func (r *Sessions) ListTokens(ctx context.Context, userID uuid.UUID) ([]*auth.Session, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE user_id=$1 AND kind='token' ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSession)
}

func (r *Sessions) DeleteToken(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	tag, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM sessions WHERE id=$1 AND user_id=$2 AND kind='token'`, id, userID)
	return tag.RowsAffected() > 0, err
}

func (r *Sessions) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM sessions WHERE expires_at IS NOT NULL AND expires_at < $1`, now)
	return err
}
