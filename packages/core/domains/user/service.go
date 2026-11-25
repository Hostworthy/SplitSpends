package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// Invitation emails go to addresses nobody has vouched for, so each account
// may send MaxInvites of them per InviteWindow.
const (
	MaxInvites   = 20
	InviteWindow = 24 * time.Hour
)

// MaxPlaceholders is how many people known only by name one account may
// add, as many as an import may bring.
const MaxPlaceholders = 1000

// Service exposes user use cases.
type Service struct {
	repo     Repository
	notifier ports.Notifier
	quota    ports.Quota
	clock    clock.Clock
}

// NewService builds a Service.
func NewService(repo Repository, notifier ports.Notifier, quota ports.Quota, c clock.Clock) *Service {
	return &Service{repo: repo, notifier: notifier, quota: quota, clock: c}
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

// Ensure returns the user with email, creating an invited user when none
// exists. Without an email it creates a placeholder known only by name.
// created reports whether a new user was made. Call it inside the caller's
// transaction and SendInvite once that has committed.
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
		if !ports.IsQuiet(ctx) {
			if err := s.spend(ctx, inviter.ID); err != nil {
				return nil, false, err
			}
		}
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, false, err
	}
	return u, true, nil
}

// SendInvite emails u, whom inviter has just invited through Ensure, unless
// u already had an account or has no email.
func (s *Service) SendInvite(ctx context.Context, u, inviter *User) {
	if u.Status != StatusInvited || u.Email == "" || s.notifier == nil || ports.IsQuiet(ctx) {
		return
	}
	_ = s.notifier.Invite(ctx, u.Email, inviter.Name)
}

// spend uses one of the inviter's invitation emails.
func (s *Service) spend(ctx context.Context, inviter uuid.UUID) error {
	ok, err := s.quota.Take(ctx, "invite:"+inviter.String(), MaxInvites, InviteWindow, s.clock.Now())
	if err != nil {
		return err
	}
	if !ok {
		return errs.RateLimited(fmt.Sprintf("You can invite up to %d new people by email a day. Try again tomorrow, or add them by name for now.", MaxInvites))
	}
	return nil
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
			return nil, errs.Taken()
		}
		if !errs.Is(err, errs.KindNotFound) {
			return nil, err
		}
	}
	u, err := NewPlaceholder(id, name, createdBy, s.clock.Now())
	if err != nil {
		return nil, err
	}
	n, err := s.repo.Placeholders(ctx, createdBy)
	if err != nil {
		return nil, err
	}
	if n >= MaxPlaceholders {
		return nil, errs.Conflict(fmt.Sprintf("You have added %d people by name, the most you can. Give some of them an email, or remove ones you no longer need.", MaxPlaceholders))
	}
	if inviter, err := s.repo.Get(ctx, createdBy); err == nil {
		u.DefaultCurrency = inviter.DefaultCurrency
	}
	return u, s.repo.Create(ctx, u)
}

// Invite attaches an email to a placeholder the actor created, turning it
// into an invited user who can sign in. Anyone the actor did not create is
// not found, like an id that does not exist.
func (s *Service) Invite(ctx context.Context, actor, id uuid.UUID, email string) (*User, error) {
	u, err := s.repo.Get(ctx, id)
	if err != nil && !errs.Is(err, errs.KindNotFound) {
		return nil, err
	}
	if err != nil || u.CreatedBy == nil || *u.CreatedBy != actor {
		return nil, errs.NotFound("Friend")
	}
	if u.Status != StatusPlaceholder {
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
		return nil, errs.Conflict("That email already belongs to an account. What you recorded for this person cannot be moved to that account, so add the account by email as a separate friend.")
	}
	if err := s.spend(ctx, actor); err != nil {
		return nil, err
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
