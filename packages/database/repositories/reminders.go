package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/reminder"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Reminders implements reminder.Repository.
type Reminders struct{ db *database.DB }

// NewReminders builds Reminders.
func NewReminders(db *database.DB) *Reminders { return &Reminders{db: db} }

// Claim upserts the scope's row only when its last reminder is not newer
// than since; no returned row means the window is still closed.
func (r *Reminders) Claim(ctx context.Context, rm *reminder.Reminder, since time.Time) (bool, error) {
	var id uuid.UUID
	err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO reminders (id, from_id, to_id, group_id, sent_at) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (from_id, to_id, (COALESCE(group_id, '00000000-0000-0000-0000-000000000000'::uuid)))
		DO UPDATE SET id = EXCLUDED.id, sent_at = EXCLUDED.sent_at WHERE reminders.sent_at <= $6
		RETURNING id`, rm.ID, rm.FromID, rm.ToID, rm.GroupID, rm.SentAt, since).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
