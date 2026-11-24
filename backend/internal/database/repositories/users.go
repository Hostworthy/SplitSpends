// Package repositories implements core repository ports with pgx.
package repositories

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/atb00ker/splitspends/backend/internal/core/domains/user"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Users implements user.Repository.
type Users struct{ db *database.DB }

// NewUsers builds Users.
func NewUsers(db *database.DB) *Users { return &Users{db: db} }

const userColumns = `id, created_at, updated_at, email, name, default_currency, avatar_color, timezone, status, notifications`

func scanUser(row pgx.Row) (*user.User, error) {
	var u user.User
	var notif []byte
	if err := row.Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt, &u.Email, &u.Name, &u.DefaultCurrency, &u.AvatarColor, &u.Timezone, &u.Status, &notif); err != nil {
		return nil, err
	}
	u.Notifications = user.DefaultNotifications()
	if len(notif) > 0 {
		_ = json.Unmarshal(notif, &u.Notifications)
	}
	return &u, nil
}

func (r *Users) Get(ctx context.Context, id uuid.UUID) (*user.User, error) {
	u, err := scanUser(r.db.Q(ctx).QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("User")
	}
	return u, err
}

func (r *Users) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	u, err := scanUser(r.db.Q(ctx).QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (r *Users) List(ctx context.Context, ids []uuid.UUID) ([]*user.User, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+userColumns+` FROM users WHERE id = ANY($1) ORDER BY lower(name)`, ids)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanUser)
}

func (r *Users) Create(ctx context.Context, u *user.User) error {
	notif, _ := json.Marshal(u.Notifications)
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO users (`+userColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		u.ID, u.CreatedAt, u.UpdatedAt, u.Email, u.Name, u.DefaultCurrency, u.AvatarColor, u.Timezone, u.Status, notif)
	if database.IsUniqueViolation(err) {
		return errs.Conflict("A user with that email already exists.")
	}
	return err
}

func (r *Users) Update(ctx context.Context, u *user.User) error {
	notif, _ := json.Marshal(u.Notifications)
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE users SET updated_at=$2, name=$3, default_currency=$4, avatar_color=$5, timezone=$6, status=$7, notifications=$8 WHERE id=$1`,
		u.ID, u.UpdatedAt, u.Name, u.DefaultCurrency, u.AvatarColor, u.Timezone, u.Status, notif)
	return err
}

func (r *Users) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM users WHERE status = 'active'`).Scan(&n)
	return n, err
}

// collect scans every row with scan.
func collect[T any](rows pgx.Rows, scan func(pgx.Row) (T, error)) ([]T, error) {
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
