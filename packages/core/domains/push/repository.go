package push

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Repository persists subscriptions.
type Repository interface {
	// Save inserts s, or refreshes the subscription with the same endpoint
	// (keys, platform, label, owner and session), resetting its failures only when
	// the keys changed: the same device registering again is not a fresh
	// start, a new subscription on it is. It fills s.ID, s.CreatedAt,
	// s.LastSuccessAt and s.Failures from the stored row and reports
	// whether a new row was created. When s.UserID already has limit other
	// subscriptions it stores nothing and returns ErrFull; the check and
	// the write are atomic, so concurrent calls cannot pass the limit. A
	// subscription is deleted together with its session (s.SessionID).
	Save(ctx context.Context, s *Subscription, limit int) (created bool, err error)
	// List returns userID's subscriptions, newest first.
	List(ctx context.Context, userID uuid.UUID) ([]*Subscription, error)
	// Delete removes userID's subscription id; errs.NotFound when there is
	// none.
	Delete(ctx context.Context, userID, id uuid.UUID) error
	// Remove deletes a subscription the push service no longer knows.
	Remove(ctx context.Context, id uuid.UUID) error
	// Succeeded records a delivery the push service accepted.
	Succeeded(ctx context.Context, id uuid.UUID, at time.Time) error
	// Failed counts a failed delivery and returns the new failure count.
	Failed(ctx context.Context, id uuid.UUID) (int, error)
}

// ErrFull is returned by Repository.Save when the user has as many
// subscriptions as allowed.
var ErrFull = errors.New("push.subscriptions.full")

// ErrGone is returned by a Sender when the push service answers 404 or 410:
// the subscription expired or the user revoked it.
var ErrGone = errors.New("push.subscription.gone")

// ErrTooLarge is returned by a Sender for a payload that does not fit in
// one Web Push message. It is this server's fault, not the device's.
var ErrTooLarge = errors.New("push.payload.too-large")

// Sender encrypts and delivers one message (RFC 8291 and RFC 8292). The
// webpush adapter implements it.
type Sender interface {
	Send(ctx context.Context, s *Subscription, payload []byte) error
}
