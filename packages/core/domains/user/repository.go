package user

import (
	"context"

	"github.com/google/uuid"
)

// Repository persists users.
type Repository interface {
	// Get returns errs.NotFound when the user does not exist.
	Get(ctx context.Context, id uuid.UUID) (*User, error)
	// FindByEmail returns nil, nil when no user has the email.
	FindByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, ids []uuid.UUID) ([]*User, error)
	Create(ctx context.Context, u *User) error
	Update(ctx context.Context, u *User) error
	// Count counts active accounts.
	Count(ctx context.Context) (int, error)
	// Lock waits for the lock on who runs the server and holds it until the
	// transaction in ctx ends, so the first account and changes to
	// administrators happen one at a time. It must be called inside Tx.Do.
	Lock(ctx context.Context) error
	// Placeholders counts the people createdBy added by name only.
	Placeholders(ctx context.Context, createdBy uuid.UUID) (int, error)
}
