package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/atb00ker/splitspends/backend/internal/core/domains/group"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/receipt"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Friends implements friend.Repository.
type Friends struct{ db *database.DB }

// NewFriends builds Friends.
func NewFriends(db *database.DB) *Friends { return &Friends{db: db} }

func ordered(a, b uuid.UUID) (uuid.UUID, uuid.UUID) {
	if a.String() < b.String() {
		return a, b
	}
	return b, a
}

func (r *Friends) Add(ctx context.Context, a, b uuid.UUID) error {
	x, y := ordered(a, b)
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO friendships (user_a, user_b, created_at) VALUES ($1,$2,now()) ON CONFLICT DO NOTHING`, x, y)
	return err
}

func (r *Friends) Remove(ctx context.Context, a, b uuid.UUID) error {
	x, y := ordered(a, b)
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM friendships WHERE user_a=$1 AND user_b=$2`, x, y)
	return err
}

func (r *Friends) List(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `
		SELECT user_b FROM friendships WHERE user_a = $1
		UNION SELECT user_a FROM friendships WHERE user_b = $1
		UNION SELECT other.user_id FROM group_members mine
			JOIN group_members other ON other.group_id = mine.group_id AND other.left_at IS NULL
			WHERE mine.user_id = $1 AND mine.left_at IS NULL AND other.user_id <> $1`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanID)
}

func (r *Friends) Connected(ctx context.Context, a, b uuid.UUID) (bool, error) {
	x, y := ordered(a, b)
	var ok bool
	err := r.db.Q(ctx).QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM friendships WHERE user_a=$1 AND user_b=$2)
		    OR EXISTS (SELECT 1 FROM group_members m1 JOIN group_members m2 ON m1.group_id = m2.group_id
		               WHERE m1.user_id=$1 AND m2.user_id=$2 AND m1.left_at IS NULL AND m2.left_at IS NULL)`, x, y).Scan(&ok)
	return ok, err
}

func scanID(row pgx.Row) (uuid.UUID, error) {
	var id uuid.UUID
	err := row.Scan(&id)
	return id, err
}

// Groups implements group.Repository.
type Groups struct{ db *database.DB }

// NewGroups builds Groups.
func NewGroups(db *database.DB) *Groups { return &Groups{db: db} }

const groupColumns = `g.id, g.created_by, g.archived_at, g.created_at, g.updated_at, g.simplify_debts, g.name, g.kind, g.default_currency, g.emoji, g.color,
	COALESCE((SELECT array_agg(m.user_id ORDER BY m.joined_at) FROM group_members m WHERE m.group_id = g.id AND m.left_at IS NULL), '{}')`

func scanGroup(row pgx.Row) (*group.Group, error) {
	var g group.Group
	err := row.Scan(&g.ID, &g.CreatedBy, &g.ArchivedAt, &g.CreatedAt, &g.UpdatedAt, &g.SimplifyDebts, &g.Name, &g.Kind, &g.DefaultCurrency, &g.Emoji, &g.Color, &g.Members)
	return &g, err
}

func (r *Groups) Create(ctx context.Context, g *group.Group) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO groups (id, created_by, archived_at, created_at, updated_at, simplify_debts, name, kind, default_currency, emoji, color)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		g.ID, g.CreatedBy, g.ArchivedAt, g.CreatedAt, g.UpdatedAt, g.SimplifyDebts, g.Name, g.Kind, g.DefaultCurrency, g.Emoji, g.Color)
	return err
}

func (r *Groups) Get(ctx context.Context, id uuid.UUID) (*group.Group, error) {
	g, err := scanGroup(r.db.Q(ctx).QueryRow(ctx, `SELECT `+groupColumns+` FROM groups g WHERE g.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("Group")
	}
	return g, err
}

func (r *Groups) Update(ctx context.Context, g *group.Group) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE groups SET archived_at=$2, updated_at=$3, simplify_debts=$4, name=$5, kind=$6, default_currency=$7, emoji=$8, color=$9 WHERE id=$1`,
		g.ID, g.ArchivedAt, g.UpdatedAt, g.SimplifyDebts, g.Name, g.Kind, g.DefaultCurrency, g.Emoji, g.Color)
	return err
}

func (r *Groups) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM groups WHERE id=$1`, id)
	return err
}

func (r *Groups) ForUser(ctx context.Context, userID uuid.UUID, archived bool) ([]*group.Group, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+groupColumns+` FROM groups g
		JOIN group_members me ON me.group_id = g.id AND me.user_id = $1 AND me.left_at IS NULL
		WHERE (g.archived_at IS NOT NULL) = $2 ORDER BY g.updated_at DESC`, userID, archived)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanGroup)
}

func (r *Groups) AddMember(ctx context.Context, groupID, userID uuid.UUID, at time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO group_members (group_id, user_id, joined_at) VALUES ($1,$2,$3)
		ON CONFLICT (group_id, user_id) DO UPDATE SET left_at = NULL, joined_at = EXCLUDED.joined_at`, groupID, userID, at)
	return err
}

func (r *Groups) RemoveMember(ctx context.Context, groupID, userID uuid.UUID, at time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE group_members SET left_at=$3 WHERE group_id=$1 AND user_id=$2`, groupID, userID, at)
	return err
}

// Receipts implements receipt.Repository and expense.Receipts.
type Receipts struct{ db *database.DB }

// NewReceipts builds Receipts.
func NewReceipts(db *database.DB) *Receipts { return &Receipts{db: db} }

func (r *Receipts) Create(ctx context.Context, rc *receipt.Receipt) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO receipts (id, uploaded_by, created_at, size, content_type, storage_key) VALUES ($1,$2,$3,$4,$5,$6)`,
		rc.ID, rc.UploadedBy, rc.CreatedAt, rc.Size, rc.ContentType, rc.StorageKey)
	return err
}

func (r *Receipts) Get(ctx context.Context, id uuid.UUID) (*receipt.Receipt, error) {
	var rc receipt.Receipt
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT id, uploaded_by, created_at, size, content_type, storage_key FROM receipts WHERE id=$1`, id).
		Scan(&rc.ID, &rc.UploadedBy, &rc.CreatedAt, &rc.Size, &rc.ContentType, &rc.StorageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("Receipt")
	}
	return &rc, err
}

func (r *Receipts) Owned(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	var ok bool
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM receipts WHERE id=$1 AND uploaded_by=$2)
		OR EXISTS (SELECT 1 FROM expenses e WHERE e.receipt_id=$1 AND (
			EXISTS (SELECT 1 FROM expense_shares s WHERE s.expense_id=e.id AND s.user_id=$2)
			OR EXISTS (SELECT 1 FROM group_members m WHERE m.group_id=e.group_id AND m.user_id=$2 AND m.left_at IS NULL)))`, id, userID).Scan(&ok)
	return ok, err
}

func (r *Receipts) Visible(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	return r.Owned(ctx, id, userID)
}
