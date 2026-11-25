package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ChallengeRepository persists login codes.
type ChallengeRepository interface {
	Create(ctx context.Context, c *Challenge) error
	// Lock holds the codes of email until the transaction ends, so that
	// concurrent requests for the same address count each other.
	Lock(ctx context.Context, email string) error
	// CountSince returns how many codes were requested for email since
	// since: in total, and from client.
	CountSince(ctx context.Context, email, client string, since time.Time) (total, own int, err error)
	// Attempt counts one attempt against every code of email that is
	// usable at now with limit attempts (Challenge.Usable), and returns
	// those codes. Counting and checking are one step, so concurrent
	// attempts never take a code past limit.
	Attempt(ctx context.Context, email string, now time.Time, limit int) ([]*Challenge, error)
	// Consume marks a code used and reports whether it was still unused.
	Consume(ctx context.Context, id uuid.UUID, at time.Time) (bool, error)
	DeleteBefore(ctx context.Context, before time.Time) error
}

// SessionRepository persists sessions and API tokens.
type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	// FindByHash returns nil when no session matches.
	FindByHash(ctx context.Context, hash string) (*Session, error)
	Touch(ctx context.Context, id uuid.UUID, at time.Time) error
	Delete(ctx context.Context, id uuid.UUID) error
	// Lock holds the API tokens of userID until the transaction ends, so
	// that concurrent requests to create one count each other.
	Lock(ctx context.Context, userID uuid.UUID) error
	ListTokens(ctx context.Context, userID uuid.UUID) ([]*Session, error)
	DeleteToken(ctx context.Context, userID, id uuid.UUID) (bool, error)
	DeleteExpired(ctx context.Context, now time.Time) error
}
