package user

import (
	"context"
	"strings"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
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
// an invite email) when none exists. Without an email it creates a
// placeholder known only by name. created reports whether a new user was made.
func (s *Service) Ensure(ctx context.Context, email, name string, inviter *User) (u *User, created bool, err error) {
	if strings.TrimSpace(email) == "" {
		if inviter == nil {
			return nil, false, errs.Validation("email", "Email is required.")
		}
		u, err := s.Placeholder(ctx, nil, name, inviter.ID)
		return u, err == nil, err
	}
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
	if inviter != nil && s.notifier != nil && !ports.IsQuiet(ctx) {
		_ = s.notifier.Invite(ctx, u.Email, inviter.Name)
	}
	return u, true, nil
}

// Placeholder creates (or, when id already names the creator's placeholder,
// returns) a person known only by name.
func (s *Service) Placeholder(ctx context.Context, id *uuid.UUID, name string, createdBy uuid.UUID) (*User, error) {
	if id != nil {
		existing, err := s.repo.Get(ctx, *id)
		if err == nil {
			if existing.Status == StatusPlaceholder && existing.CreatedBy != nil && *existing.CreatedBy == createdBy {
				return existing, nil
			}
			return nil, errs.Conflict("That person id is already in use.")
		}
		if !errs.Is(err, errs.KindNotFound) {
			return nil, err
		}
	}
	u, err := NewPlaceholder(id, name, createdBy, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if inviter, err := s.repo.Get(ctx, createdBy); err == nil {
		u.DefaultCurrency = inviter.DefaultCurrency
	}
	return u, s.repo.Create(ctx, u)
}

// Invite attaches an email to a placeholder the actor created, turning it
// into an invited user who can sign in.
func (s *Service) Invite(ctx context.Context, actor, id uuid.UUID, email string) (*User, error) {
	u, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.Status != StatusPlaceholder || u.CreatedBy == nil || *u.CreatedBy != actor {
		return nil, errs.Conflict("Only people you added by name can be given an email.")
	}
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.FindByEmail(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errs.Conflict(existing.Name + " already has an account with that email. Add them directly instead.")
	}
	u.Email, u.Status, u.UpdatedAt = normalized, StatusInvited, s.clock.Now()
	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}
	if inviter, err := s.repo.Get(ctx, actor); err == nil && s.notifier != nil {
		_ = s.notifier.Invite(ctx, u.Email, inviter.Name)
	}
	return u, nil
}
