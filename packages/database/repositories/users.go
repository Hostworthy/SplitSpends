// Package repositories implements core repository ports with pgx.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Users implements user.Repository.
type Users struct{ db *database.DB }

// NewUsers builds Users.
func NewUsers(db *database.DB) *Users { return &Users{db: db} }

const userColumns = `id, created_at, updated_at, email, name, default_currency, avatar_color, timezone, status, notifications, created_by, gravatar, role, email_hash`

func scanUser(row pgx.Row) (*user.User, error) {
	var u user.User
	var notif []byte
	var email, hash *string
	if err := row.Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt, &email, &u.Name, &u.DefaultCurrency, &u.AvatarColor, &u.Timezone, &u.Status, &notif, &u.CreatedBy, &u.Gravatar, &u.Role, &hash); err != nil {
		return nil, err
	}
	if email != nil {
		u.Email = *email
	}
	if hash != nil {
		u.EmailHash = *hash
	}
	u.Notifications = user.DefaultNotifications()
	if len(notif) > 0 {
		if err := json.Unmarshal(notif, &u.Notifications); err != nil {
			return nil, fmt.Errorf("database.user.notifications.invalid: %w", err)
		}
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
	// A disabled account keeps only the hash of its address.
	u, err := scanUser(r.db.Q(ctx).QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1) OR email_hash = $2
		ORDER BY email IS NULL LIMIT 1`, email, user.HashEmail(email)))
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
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO users (`+userColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		u.ID, u.CreatedAt, u.UpdatedAt, nullable(u.Email), u.Name, u.DefaultCurrency, u.AvatarColor, u.Timezone, u.Status, notif, u.CreatedBy, u.Gravatar, u.Role, nullable(u.EmailHash))
	if database.IsUniqueViolation(err) {
		return errs.Conflict("A user with that email already exists.")
	}
	return err
}

func (r *Users) Update(ctx context.Context, u *user.User) error {
	notif, _ := json.Marshal(u.Notifications)
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE users SET updated_at=$2, name=$3, default_currency=$4, avatar_color=$5, timezone=$6, status=$7, notifications=$8, email=$9, gravatar=$10, role=$11, email_hash=$12 WHERE id=$1`,
		u.ID, u.UpdatedAt, u.Name, u.DefaultCurrency, u.AvatarColor, u.Timezone, u.Status, notif, nullable(u.Email), u.Gravatar, u.Role, nullable(u.EmailHash))
	if database.IsUniqueViolation(err) {
		return errs.Conflict("A user with that email already exists.")
	}
	return err
}

func (r *Users) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM users WHERE status = 'active'`).Scan(&n)
	return n, err
}

// adminLock is the first key of the transaction advisory lock on who runs
// the server; it never collides with other advisory locks.
const adminLock = 1_384_202

func (r *Users) Lock(ctx context.Context) error {
	_, err := r.db.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock($1, 0)`, adminLock)
	return err
}

func (r *Users) Placeholders(ctx context.Context, createdBy uuid.UUID) (int, error) {
	var n int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM users WHERE created_by = $1 AND status = 'placeholder'`, createdBy).Scan(&n)
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

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Staff lists active administrators and managers, who hear about people
// waiting for approval.
func (r *Users) Staff(ctx context.Context) ([]*user.User, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+userColumns+` FROM users WHERE status = 'active' AND role IN ('admin', 'manager') ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanUser)
}

// Admins counts active administrators.
func (r *Users) Admins(ctx context.Context) (int, error) {
	var n int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM users WHERE status = 'active' AND role = 'admin'`).Scan(&n)
	return n, err
}

// Accounts lists people who can sign in or could (everyone but name-only
// placeholders), filtered by status and a name or email search. A search
// for a whole address also finds the account that keeps only its hash.
func (r *Users) Accounts(ctx context.Context, status, q string, limit, offset int) ([]*user.User, int, error) {
	where := `status <> 'placeholder' AND ($1 = '' OR status = $1)
		AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR email ILIKE '%' || $2 || '%' OR email_hash = $3)`
	hash := user.HashEmail(q)
	var total int
	if err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM users WHERE `+where, status, q, hash).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+userColumns+` FROM users WHERE `+where+`
		ORDER BY CASE role WHEN 'admin' THEN 0 WHEN 'manager' THEN 1 ELSE 2 END, lower(name), id LIMIT $4 OFFSET $5`, status, q, hash, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	list, err := collect(rows, scanUser)
	return list, total, err
}
