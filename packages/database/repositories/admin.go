package repositories

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/admin"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SignupSettings implements admin.SettingsRepository.
type SignupSettings struct{ db *database.DB }

// NewSignupSettings builds SignupSettings.
func NewSignupSettings(db *database.DB) *SignupSettings { return &SignupSettings{db: db} }

func (r *SignupSettings) Get(ctx context.Context) (*admin.Settings, error) {
	var s admin.Settings
	var mode string
	var allowed []byte
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT mode, allowed, updated_at, updated_by FROM signup_settings WHERE id`).
		Scan(&mode, &allowed, &s.UpdatedAt, &s.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Mode = auth.SignupMode(mode)
	if err := json.Unmarshal(allowed, &s.Allowed); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SignupSettings) Save(ctx context.Context, s *admin.Settings) error {
	allowed, err := json.Marshal(s.Allowed)
	if err != nil {
		return err
	}
	_, err = r.db.Q(ctx).Exec(ctx, `INSERT INTO signup_settings (id, mode, allowed, updated_at, updated_by) VALUES (true, $1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET mode = EXCLUDED.mode, allowed = EXCLUDED.allowed, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`,
		string(s.Mode), allowed, s.UpdatedAt, s.UpdatedBy)
	return err
}

// SignupRequests implements admin.RequestRepository.
type SignupRequests struct{ db *database.DB }

// NewSignupRequests builds SignupRequests.
func NewSignupRequests(db *database.DB) *SignupRequests { return &SignupRequests{db: db} }

const requestColumns = `id, email, name, status, requested_at, decided_at, decided_by`

func scanRequest(row pgx.Row) (*admin.Request, error) {
	var r admin.Request
	err := row.Scan(&r.ID, &r.Email, &r.Name, &r.Status, &r.RequestedAt, &r.DecidedAt, &r.DecidedBy)
	return &r, err
}

func (r *SignupRequests) Find(ctx context.Context, email string) (*admin.Request, error) {
	req, err := scanRequest(r.db.Q(ctx).QueryRow(ctx, `SELECT `+requestColumns+` FROM signup_requests WHERE lower(email) = lower($1)`, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return req, err
}

func (r *SignupRequests) Get(ctx context.Context, id uuid.UUID) (*admin.Request, error) {
	req, err := scanRequest(r.db.Q(ctx).QueryRow(ctx, `SELECT `+requestColumns+` FROM signup_requests WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("Request")
	}
	return req, err
}

func (r *SignupRequests) Save(ctx context.Context, req *admin.Request) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO signup_requests (`+requestColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, status = EXCLUDED.status, decided_at = EXCLUDED.decided_at, decided_by = EXCLUDED.decided_by`,
		req.ID, req.Email, req.Name, req.Status, req.RequestedAt, req.DecidedAt, req.DecidedBy)
	return err
}

func (r *SignupRequests) List(ctx context.Context, status admin.Status) ([]*admin.Request, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+requestColumns+` FROM signup_requests WHERE status = $1 ORDER BY requested_at DESC LIMIT 500`, status)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanRequest)
}

// Credentials implements admin.Credentials: it deletes a person's sessions,
// API tokens, push devices and login codes.
type Credentials struct{ db *database.DB }

// NewCredentials builds Credentials.
func NewCredentials(db *database.DB) *Credentials { return &Credentials{db: db} }

func (r *Credentials) Revoke(ctx context.Context, userID uuid.UUID, email string) error {
	if _, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM login_challenges WHERE email = lower($1)`, email); err != nil {
		return err
	}
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1`, userID)
	return err
}
