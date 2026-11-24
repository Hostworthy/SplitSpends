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
	Count(ctx context.Context) (int, error)
}
