package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/receipt"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
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

func (r *Friends) Remove(ctx context.Context, a, b uuid.UUID) (bool, error) {
	x, y := ordered(a, b)
	tag, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM friendships WHERE user_a=$1 AND user_b=$2`, x, y)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// pair names the advisory lock that guards whether a and b are connected.
func pair(a, b uuid.UUID) string {
	x, y := ordered(a, b)
	return "friendship:" + x.String() + ":" + y.String()
}

func (r *Friends) Lock(ctx context.Context, a, b uuid.UUID) error {
	_, err := r.db.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, pair(a, b))
	return err
}

func (r *Friends) Hold(ctx context.Context, a, b uuid.UUID) error {
	_, err := r.db.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtextextended($1, 0))`, pair(a, b))
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
	g.default_split, COALESCE((SELECT array_agg(m.user_id ORDER BY m.joined_at) FROM group_members m WHERE m.group_id = g.id AND m.left_at IS NULL), '{}')`

// splitRow is the stored JSON shape of a group's default split.
type splitRow struct {
	Method group.SplitMethod    `json:"method"`
	Values map[uuid.UUID]string `json:"values"`
}

func splitJSON(d *group.DefaultSplit) (any, error) {
	if d == nil {
		return nil, nil
	}
	values := d.Values
	if values == nil {
		values = map[uuid.UUID]string{}
	}
	b, err := json.Marshal(splitRow{Method: d.Method, Values: values})
	if err != nil {
		return nil, fmt.Errorf("database.group.default-split.encode: %w", err)
	}
	return b, nil
}

func scanGroup(row pgx.Row) (*group.Group, error) {
	var g group.Group
	var split []byte
	err := row.Scan(&g.ID, &g.CreatedBy, &g.ArchivedAt, &g.CreatedAt, &g.UpdatedAt, &g.SimplifyDebts, &g.Name, &g.Kind, &g.DefaultCurrency, &g.Emoji, &g.Color, &split, &g.Members)
	if err != nil {
		return &g, err
	}
	if len(split) > 0 {
		var s splitRow
		if err := json.Unmarshal(split, &s); err != nil {
			return nil, fmt.Errorf("database.group.default-split.invalid: %w", err)
		}
		g.DefaultSplit = &group.DefaultSplit{Method: s.Method, Values: s.Values}
	}
	return &g, nil
}

func (r *Groups) Create(ctx context.Context, g *group.Group) error {
	split, err := splitJSON(g.DefaultSplit)
	if err != nil {
		return err
	}
	tag, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO groups (id, created_by, archived_at, created_at, updated_at, simplify_debts, name, kind, default_currency, emoji, color, default_split)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (id) DO NOTHING`,
		g.ID, g.CreatedBy, g.ArchivedAt, g.CreatedAt, g.UpdatedAt, g.SimplifyDebts, g.Name, g.Kind, g.DefaultCurrency, g.Emoji, g.Color, split)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errs.Taken()
	}
	return nil
}

func (r *Groups) Get(ctx context.Context, id uuid.UUID) (*group.Group, error) {
	g, err := scanGroup(r.db.Q(ctx).QueryRow(ctx, `SELECT `+groupColumns+` FROM groups g WHERE g.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("Group")
	}
	return g, err
}

func (r *Groups) Lock(ctx context.Context, id uuid.UUID) (*group.Group, error) {
	g, err := scanGroup(r.db.Q(ctx).QueryRow(ctx, `SELECT `+groupColumns+` FROM groups g WHERE g.id=$1 FOR UPDATE OF g`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("Group")
	}
	return g, err
}

func (r *Groups) Hold(ctx context.Context, id uuid.UUID) error {
	var one int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT 1 FROM groups WHERE id=$1 FOR KEY SHARE`, id).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.NotFound("Group")
	}
	return err
}

func (r *Groups) Update(ctx context.Context, g *group.Group) error {
	split, err := splitJSON(g.DefaultSplit)
	if err != nil {
		return err
	}
	_, err = r.db.Q(ctx).Exec(ctx, `UPDATE groups SET archived_at=$2, updated_at=$3, simplify_debts=$4, name=$5, kind=$6, default_currency=$7, emoji=$8, color=$9,
		default_split=$10 WHERE id=$1`,
		g.ID, g.ArchivedAt, g.UpdatedAt, g.SimplifyDebts, g.Name, g.Kind, g.DefaultCurrency, g.Emoji, g.Color, split)
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

func (r *Groups) Joined(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM group_members WHERE user_id = $1 AND left_at IS NULL`, userID).Scan(&n)
	return n, err
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
