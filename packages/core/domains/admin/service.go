package admin

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// Deps are the Service's collaborators.
type Deps struct {
	Settings    SettingsRepository
	Requests    RequestRepository
	Users       user.Repository
	Accounts    Accounts
	Credentials Credentials
	Notifier    ports.Notifier
	Tx          ports.Tx
	Clock       clock.Clock
	Log         *slog.Logger
	// Defaults are the environment's sign-up settings, used until an
	// administrator saves their own.
	Defaults auth.Signup
	BaseURL  string
	AppName  string
}

// Service runs administration. It also implements auth.Gate.
type Service struct{ d Deps }

// NewService builds a Service.
func NewService(d Deps) *Service { return &Service{d: d} }

var _ auth.Gate = (*Service)(nil)

// Signup returns the sign-up rules in force.
func (s *Service) Signup(ctx context.Context) (auth.Signup, error) {
	stored, err := s.d.Settings.Get(ctx)
	if err != nil {
		return auth.Signup{}, err
	}
	if stored == nil {
		return s.d.Defaults, nil
	}
	return stored.Signup, nil
}

// Approved reports whether email was approved to join.
func (s *Service) Approved(ctx context.Context, email string) (bool, error) {
	r, err := s.d.Requests.Find(ctx, email)
	return r != nil && r.Status == StatusApproved, err
}

// announceEvery spaces the notices about new requests: staff hear about a
// request unless another one already waited from less than this ago, so a
// burst of requests sends one notice instead of one each.
const announceEvery = time.Hour

// Request files a request to join and tells administrators and managers
// about it. An earlier request stands as it is. It reports whether email was
// declined.
func (s *Service) Request(ctx context.Context, email, name string) (bool, error) {
	r, err := s.d.Requests.Find(ctx, email)
	if err != nil {
		return false, err
	}
	if r != nil {
		// An approval that landed after the caller checked lets the person
		// in on their next sign-in, so it needs nothing here.
		return r.Status == StatusDeclined, nil
	}
	name, err = user.NormalizeName(name, email)
	if err != nil {
		return false, err
	}
	now := s.d.Clock.Now()
	r = &Request{ID: uuid.New(), Email: email, Name: name, Status: StatusPending, RequestedAt: now}
	// Counted and filed under the queue's lock, so requests at the same time
	// cannot both take the last place.
	var latest []*Request
	err = s.d.Tx.Do(ctx, func(ctx context.Context) error {
		if err := s.d.Requests.Lock(ctx); err != nil {
			return err
		}
		var pending int
		if latest, pending, err = s.d.Requests.List(ctx, StatusPending, 1, 0); err != nil {
			return err
		}
		if pending >= MaxPending {
			return errs.RateLimited("Too many people are waiting to join right now. Try again in a few days.")
		}
		return s.d.Requests.Save(ctx, r)
	})
	if err != nil {
		return false, err
	}
	if len(latest) == 0 || !now.Before(latest[0].RequestedAt.Add(announceEvery)) {
		s.announce(ctx, r)
	}
	return false, nil
}

// announce tells staff about r. The request is filed already, so a failed
// lookup is logged rather than returned.
func (s *Service) announce(ctx context.Context, r *Request) {
	if ports.IsQuiet(ctx) {
		return
	}
	staff, err := s.d.Accounts.Staff(ctx)
	if err != nil {
		s.d.Log.ErrorContext(ctx, "admin.announce.failed", "request", r.ID, "error", err)
		return
	}
	who := r.Email
	if r.Name != "" {
		who = r.Name + " (" + r.Email + ")"
	}
	for _, u := range staff {
		n, ok := u.Notice(user.Approvals, who+" wants to join "+s.d.AppName,
			[]string{who + " asked to join " + s.d.AppName + ".", "Approve or decline them in Account, Administration."},
			s.d.BaseURL+"/account/admin")
		if ok {
			s.d.Notifier.Notify(ctx, n)
		}
	}
}

// actor loads the caller and checks they may act.
func (s *Service) actor(ctx context.Context, id uuid.UUID, approveOnly bool) (*user.User, error) {
	u, err := s.d.Users.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.IsAdmin() || (approveOnly && u.CanApprove()) {
		return u, nil
	}
	if approveOnly {
		return nil, errs.Forbidden("Only administrators and managers can do that.")
	}
	return nil, errs.Forbidden("Only administrators can do that.")
}

// Settings returns the sign-up settings for an administrator.
func (s *Service) Settings(ctx context.Context, actor uuid.UUID) (*Settings, error) {
	if _, err := s.actor(ctx, actor, false); err != nil {
		return nil, err
	}
	stored, err := s.d.Settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return &Settings{Signup: s.d.Defaults}, nil
	}
	return stored, nil
}

// UpdateSettings changes the sign-up mode and allowed list.
func (s *Service) UpdateSettings(ctx context.Context, actor uuid.UUID, mode string, allowed []string) (*Settings, error) {
	if _, err := s.actor(ctx, actor, false); err != nil {
		return nil, err
	}
	signup, err := auth.Normalize(mode, allowed)
	if err != nil {
		return nil, err
	}
	st := &Settings{Signup: signup, UpdatedAt: s.d.Clock.Now(), UpdatedBy: &actor}
	return st, s.d.Settings.Save(ctx, st)
}

// Requests lists a page of requests with status (pending when empty) and
// how many there are in all.
func (s *Service) Requests(ctx context.Context, actor uuid.UUID, status string, limit, offset int) ([]*Request, int, error) {
	if _, err := s.actor(ctx, actor, true); err != nil {
		return nil, 0, err
	}
	st := Status(status)
	switch st {
	case "":
		st = StatusPending
	case StatusPending, StatusApproved, StatusDeclined:
	default:
		return nil, 0, errs.Validation("status", "Use pending, approved or declined.")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.d.Requests.List(ctx, st, limit, max(offset, 0))
}

// Cleanup removes requests that waited longer than PendingTTL.
func (s *Service) Cleanup(ctx context.Context) error {
	return s.d.Requests.Expire(ctx, s.d.Clock.Now().Add(-PendingTTL))
}

// Approve lets the person sign in and emails them. Approvals count in
// approval, invite and open modes; while sign-up is closed nobody new can
// sign in, so approving is refused rather than sending a misleading email.
func (s *Service) Approve(ctx context.Context, actor, id uuid.UUID) (*Request, error) {
	r, err := s.decide(ctx, actor, id, StatusApproved)
	if err != nil || ports.IsQuiet(ctx) {
		return r, err
	}
	s.d.Notifier.Notify(ctx, ports.Notification{Kind: "approved", To: r.Email, Name: r.Name,
		Subject: "You can now sign in to " + s.d.AppName,
		Lines:   []string{"Your request to join " + s.d.AppName + " was approved.", "Sign in with this email address to start."},
		Link:    s.d.BaseURL})
	return r, nil
}

// Decline refuses the request; the person is told when they sign in again.
func (s *Service) Decline(ctx context.Context, actor, id uuid.UUID) (*Request, error) {
	return s.decide(ctx, actor, id, StatusDeclined)
}

func (s *Service) decide(ctx context.Context, actor, id uuid.UUID, to Status) (*Request, error) {
	if _, err := s.actor(ctx, actor, true); err != nil {
		return nil, err
	}
	r, err := s.d.Requests.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if to == StatusApproved {
		signup, err := s.Signup(ctx)
		if err != nil {
			return nil, err
		}
		if signup.Mode == auth.SignupClosed {
			return nil, errs.Conflict("Sign-up is closed, so " + r.Email + " could not sign in yet. Choose another sign-up mode in Administration, then approve the request.")
		}
	}
	if r.Status == to {
		return r, nil
	}
	now := s.d.Clock.Now()
	r.Status, r.DecidedAt, r.DecidedBy = to, &now, &actor
	return r, s.d.Requests.Save(ctx, r)
}

// Accounts lists accounts for an administrator.
func (s *Service) Accounts(ctx context.Context, actor uuid.UUID, status, q string, limit, offset int) ([]*user.User, int, error) {
	if _, err := s.actor(ctx, actor, false); err != nil {
		return nil, 0, err
	}
	switch user.Status(status) {
	case "", user.StatusActive, user.StatusInvited, user.StatusDisabled:
	default:
		return nil, 0, errs.Validation("status", "Use active, invited or disabled.")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.d.Accounts.Accounts(ctx, status, strings.TrimSpace(q), limit, max(offset, 0))
}

// SetRole makes someone a user, manager or administrator. The last active
// administrator cannot step down.
func (s *Service) SetRole(ctx context.Context, actor, id uuid.UUID, role string) (*user.User, error) {
	r, err := user.ParseRole(role)
	if err != nil {
		return nil, err
	}
	var out *user.User
	err = s.change(ctx, actor, id, func(ctx context.Context, u *user.User) error {
		switch u.Status {
		case user.StatusPlaceholder:
			return errs.Validation("userId", "People added by name only have no account.")
		case user.StatusDisabled:
			return errs.Conflict("Enable " + u.Name + " before changing their role.")
		}
		if err := s.keepAnAdmin(ctx, u, r != user.RoleAdmin); err != nil {
			return err
		}
		u.Role, u.UpdatedAt = r, s.d.Clock.Now()
		out = u
		return s.d.Users.Update(ctx, u)
	})
	return out, err
}

// Disable turns an account off and signs it out everywhere.
func (s *Service) Disable(ctx context.Context, actor, id uuid.UUID) (*user.User, error) {
	if actor == id {
		return nil, errs.Validation("userId", "You cannot disable your own account.")
	}
	var out *user.User
	err := s.change(ctx, actor, id, func(ctx context.Context, u *user.User) error {
		if err := s.keepAnAdmin(ctx, u, true); err != nil {
			return err
		}
		email := u.Email
		if err := u.Disable(s.d.Clock.Now()); err != nil {
			return err
		}
		if err := s.d.Users.Update(ctx, u); err != nil {
			return err
		}
		out = u
		// The request to join holds the address in the clear; a re-enabled
		// account is recognised by the hash, so the request is not needed.
		if err := s.d.Requests.Delete(ctx, email); err != nil {
			return err
		}
		return s.d.Credentials.Revoke(ctx, u.ID, email)
	})
	return out, err
}

// Enable lets a disabled account sign in again with its old email.
func (s *Service) Enable(ctx context.Context, actor, id uuid.UUID) (*user.User, error) {
	var out *user.User
	err := s.change(ctx, actor, id, func(ctx context.Context, u *user.User) error {
		if err := u.Enable(s.d.Clock.Now()); err != nil {
			return err
		}
		out = u
		return s.d.Users.Update(ctx, u)
	})
	return out, err
}

// change runs fn on account id for administrator actor. Changes to accounts
// take the users lock first and check the actor and read the account under
// it, so concurrent changes see each other: two administrators cannot each
// remove the other and leave the server with none.
func (s *Service) change(ctx context.Context, actor, id uuid.UUID, fn func(ctx context.Context, u *user.User) error) error {
	return s.d.Tx.Do(ctx, func(ctx context.Context) error {
		if err := s.d.Users.Lock(ctx); err != nil {
			return err
		}
		if _, err := s.actor(ctx, actor, false); err != nil {
			return err
		}
		u, err := s.d.Users.Get(ctx, id)
		if err != nil {
			return err
		}
		return fn(ctx, u)
	})
}

// keepAnAdmin refuses to remove the last active administrator.
func (s *Service) keepAnAdmin(ctx context.Context, u *user.User, removing bool) error {
	if !removing || !u.IsAdmin() {
		return nil
	}
	n, err := s.d.Accounts.Admins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return errs.Conflict(u.Name + " is the only administrator. Make someone else an administrator first.")
	}
	return nil
}
