package user

import (
	"context"

	"github.com/atb00ker/splitspends/backend/internal/core/ports"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/clock"
	"github.com/google/uuid"
)

// Service exposes user use cases.
type Service struct {
	repo     Repository
	notifier ports.Notifier
	clock    clock.Clock
}

// NewService builds a Service.
func NewService(repo Repository, notifier ports.Notifier, c clock.Clock) *Service {
	return &Service{repo: repo, notifier: notifier, clock: c}
}

// Get returns a user by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.Get(ctx, id)
}

// List returns users by id, in no particular order.
func (s *Service) List(ctx context.Context, ids []uuid.UUID) ([]*User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.repo.List(ctx, ids)
}

// Index returns users keyed by id.
func (s *Service) Index(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*User, error) {
	users, err := s.List(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]*User, len(users))
	for _, u := range users {
		out[u.ID] = u
	}
	return out, nil
}

// Update applies a profile patch.
func (s *Service) Update(ctx context.Context, id uuid.UUID, p Patch) (*User, error) {
	u, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := u.Apply(p, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Ensure returns the user with email, creating an invited user (and sending
// an invite email) when none exists. created reports whether a new user was made.
func (s *Service) Ensure(ctx context.Context, email, name string, inviter *User) (u *User, created bool, err error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return nil, false, err
	}
	existing, err := s.repo.FindByEmail(ctx, normalized)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}
	u, err = New(normalized, name, StatusInvited, s.clock.Now())
	if err != nil {
		return nil, false, err
	}
	if inviter != nil {
		u.DefaultCurrency = inviter.DefaultCurrency
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, false, err
	}
	if inviter != nil && s.notifier != nil {
		_ = s.notifier.Invite(ctx, u.Email, inviter.Name)
	}
	return u, true, nil
}
