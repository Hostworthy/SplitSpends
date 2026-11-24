package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ChallengeRepository persists login codes.
type ChallengeRepository interface {
	Create(ctx context.Context, c *Challenge) error
	// Latest returns the newest challenge for email or nil.
	Latest(ctx context.Context, email string) (*Challenge, error)
	Update(ctx context.Context, c *Challenge) error
	CountSince(ctx context.Context, email string, since time.Time) (int, error)
	DeleteBefore(ctx context.Context, before time.Time) error
}

// SessionRepository persists sessions and API tokens.
type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	// FindByHash returns nil when no session matches.
	FindByHash(ctx context.Context, hash string) (*Session, error)
	Touch(ctx context.Context, id uuid.UUID, at time.Time) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListTokens(ctx context.Context, userID uuid.UUID) ([]*Session, error)
	DeleteToken(ctx context.Context, userID, id uuid.UUID) (bool, error)
	DeleteExpired(ctx context.Context, now time.Time) error
}
