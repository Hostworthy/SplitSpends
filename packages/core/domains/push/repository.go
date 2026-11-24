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
	// (keys, platform, label and owner) and resets its failures. It fills
	// s.ID, s.CreatedAt, s.LastSuccessAt and s.Failures from the stored row
	// and reports whether a new row was created.
	Save(ctx context.Context, s *Subscription) (created bool, err error)
	// List returns userID's subscriptions, newest first.
	List(ctx context.Context, userID uuid.UUID) ([]*Subscription, error)
	// Count returns how many subscriptions userID has.
	Count(ctx context.Context, userID uuid.UUID) (int, error)
	// Exists reports whether endpoint is already registered.
	Exists(ctx context.Context, endpoint string) (bool, error)
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

// ErrGone is returned by a Sender when the push service answers 404 or 410:
// the subscription expired or the user revoked it.
var ErrGone = errors.New("push.subscription.gone")

// Sender encrypts and delivers one message (RFC 8291 and RFC 8292). The
// webpush adapter implements it.
type Sender interface {
	Send(ctx context.Context, s *Subscription, payload []byte) error
}
