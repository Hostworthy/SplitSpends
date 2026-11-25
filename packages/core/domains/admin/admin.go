// Package admin is server administration: who may sign up, approving people
// who wait to join, roles, and disabling accounts.
package admin

import (
	"context"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/google/uuid"
)

// MaxPending bounds the requests waiting for a decision, so the queue stays
// one administrators can work through.
const MaxPending = 500

// PendingTTL is how long a request waits for a decision before it is
// removed. The person can ask again.
const PendingTTL = 30 * 24 * time.Hour

// Settings are the sign-up rules an administrator chose.
type Settings struct {
	auth.Signup
	UpdatedAt time.Time
	UpdatedBy *uuid.UUID
}

// Status of a request to join.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDeclined Status = "declined"
)

// Request is someone who proved their email and waits for approval.
type Request struct {
	ID          uuid.UUID
	Email       string
	Name        string
	Status      Status
	RequestedAt time.Time
	DecidedAt   *time.Time
	DecidedBy   *uuid.UUID
}

// SettingsRepository stores the one row of sign-up settings.
type SettingsRepository interface {
	// Get returns nil when an administrator never changed the settings.
	Get(ctx context.Context) (*Settings, error)
	Save(ctx context.Context, s *Settings) error
}

// RequestRepository stores requests to join, one per email.
type RequestRepository interface {
	// Find returns nil when email never asked to join.
	Find(ctx context.Context, email string) (*Request, error)
	Get(ctx context.Context, id uuid.UUID) (*Request, error)
	Save(ctx context.Context, r *Request) error
	// List returns a page of requests with status, newest first, and how
	// many there are in all.
	List(ctx context.Context, status Status, limit, offset int) ([]*Request, int, error)
	// Expire removes pending requests filed before before.
	Expire(ctx context.Context, before time.Time) error
	// Delete removes the request of email, if there is one.
	Delete(ctx context.Context, email string) error
}

// Accounts are the user queries administration needs.
type Accounts interface {
	Staff(ctx context.Context) ([]*user.User, error)
	Admins(ctx context.Context) (int, error)
	Accounts(ctx context.Context, status, q string, limit, offset int) ([]*user.User, int, error)
}

// Credentials removes everything that lets a person in: sessions, API
// tokens, push devices and the login codes sent to their email. API tokens
// are sessions of kind token, so removing a person's sessions removes them
// too; tokens stored anywhere else must be removed here as well.
type Credentials interface {
	Revoke(ctx context.Context, userID uuid.UUID, email string) error
}
