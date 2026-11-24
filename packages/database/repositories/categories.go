package repositories

import (
	"context"
	"errors"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/category"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Categories implements category.Repository.
type Categories struct{ db *database.DB }

// NewCategories builds Categories.
func NewCategories(db *database.DB) *Categories { return &Categories{db: db} }

const categoryColumns = `c.owner_id, c.created_at, c.updated_at, c.deleted_at, c.id, c.name, c.icon, c.group_name`

func scanCategory(row pgx.Row) (*category.Category, error) {
	var c category.Category
	err := row.Scan(&c.OwnerID, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt, &c.ID, &c.Name, &c.Icon, &c.Group)
	return &c, err
}

func (r *Categories) Create(ctx context.Context, c *category.Category) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO categories (owner_id, created_at, updated_at, deleted_at, id, name, icon, group_name)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, c.OwnerID, c.CreatedAt, c.UpdatedAt, c.DeletedAt, c.ID, c.Name, c.Icon, c.Group)
	return err
}

func (r *Categories) Get(ctx context.Context, id string) (*category.Category, error) {
	c, err := scanCategory(r.db.Q(ctx).QueryRow(ctx, `SELECT `+categoryColumns+` FROM categories c WHERE c.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("Category")
	}
	return c, err
}

func (r *Categories) Update(ctx context.Context, c *category.Category) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE categories SET updated_at=$2, deleted_at=$3, name=$4, icon=$5, group_name=$6 WHERE id=$1`,
		c.ID, c.UpdatedAt, c.DeletedAt, c.Name, c.Icon, c.Group)
	return err
}

func (r *Categories) Owned(ctx context.Context, owner uuid.UUID) ([]*category.Category, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+categoryColumns+` FROM categories c
		WHERE c.owner_id=$1 AND c.deleted_at IS NULL ORDER BY lower(c.name), c.id`, owner)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanCategory)
}

func (r *Categories) Visible(ctx context.Context, userID uuid.UUID) ([]*category.Category, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+categoryColumns+` FROM categories c
		WHERE c.deleted_at IS NULL AND (c.owner_id = $1 OR c.id IN (
			SELECT e.category FROM expenses e WHERE e.category LIKE 'custom.%' AND `+visible+`))
		ORDER BY lower(c.name), c.id`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanCategory)
}

func (r *Categories) Live(ctx context.Context, ids []string) ([]*category.Category, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+categoryColumns+` FROM categories c WHERE c.id = ANY($1) AND c.deleted_at IS NULL`, ids)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanCategory)
}
