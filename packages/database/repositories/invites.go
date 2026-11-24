package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/invite"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Invites implements invite.Repository.
type Invites struct{ db *database.DB }

// NewInvites builds Invites.
func NewInvites(db *database.DB) *Invites { return &Invites{db: db} }

const inviteColumns = `id, group_id, created_by, expires_at, created_at, token_hash`

func scanInvite(row pgx.Row) (*invite.Invite, error) {
	var i invite.Invite
	err := row.Scan(&i.ID, &i.GroupID, &i.CreatedBy, &i.ExpiresAt, &i.CreatedAt, &i.TokenHash)
	return &i, err
}

func (r *Invites) Create(ctx context.Context, i *invite.Invite) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO group_invites (`+inviteColumns+`) VALUES ($1,$2,$3,$4,$5,$6)`,
		i.ID, i.GroupID, i.CreatedBy, i.ExpiresAt, i.CreatedAt, i.TokenHash)
	return err
}

func (r *Invites) FindByHash(ctx context.Context, hash string) (*invite.Invite, error) {
	i, err := scanInvite(r.db.Q(ctx).QueryRow(ctx, `SELECT `+inviteColumns+` FROM group_invites WHERE token_hash = $1`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return i, err
}

func (r *Invites) Active(ctx context.Context, groupID uuid.UUID, now time.Time) ([]*invite.Invite, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+inviteColumns+` FROM group_invites
		WHERE group_id = $1 AND expires_at > $2 ORDER BY created_at DESC`, groupID, now)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanInvite)
}

func (r *Invites) Delete(ctx context.Context, groupID, id uuid.UUID) (bool, error) {
	tag, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM group_invites WHERE id = $1 AND group_id = $2`, id, groupID)
	return tag.RowsAffected() > 0, err
}

func (r *Invites) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM group_invites WHERE expires_at <= $1`, now)
	return err
}
