package auth

import (
	"context"
	"crypto/subtle"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// Invites checks group invite links, which let new people sign up.
type Invites interface {
	Valid(ctx context.Context, token string) (bool, error)
}

// Gate is the sign-up configuration administrators change at runtime, and
// the approvals they give.
type Gate interface {
	Signup(ctx context.Context) (Signup, error)
	// Approved reports whether an administrator or manager approved email.
	Approved(ctx context.Context, email string) (bool, error)
	// Request records that email waits for approval and reports whether an
	// earlier request was declined.
	Request(ctx context.Context, email, name string) (declined bool, err error)
}

// Service implements login and credential management.
type Service struct {
	gate       Gate
	policy     Policy
	challenges ChallengeRepository
	sessions   SessionRepository
	users      user.Repository
	invites    Invites
	notifier   ports.Notifier
	tx         ports.Tx
	clock      clock.Clock
}

// NewService builds a Service. invites may be nil when links are not used.
func NewService(p Policy, challenges ChallengeRepository, sessions SessionRepository, users user.Repository, invites Invites, notifier ports.Notifier, tx ports.Tx, c clock.Clock) *Service {
	return &Service{policy: p, challenges: challenges, sessions: sessions, users: users, invites: invites, notifier: notifier, tx: tx, clock: c}
}

// Use makes g decide who may sign up, instead of the static policy.
func (s *Service) Use(g Gate) { s.gate = g }

// Mode returns the current sign-up mode.
func (s *Service) Mode(ctx context.Context) SignupMode {
	cfg, err := s.signup(ctx)
	if err != nil {
		return s.policy.Signup().Mode
	}
	return cfg.Mode
}

func (s *Service) signup(ctx context.Context) (Signup, error) {
	if s.gate == nil {
		return s.policy.Signup(), nil
	}
	return s.gate.Signup(ctx)
}

// CodeTTL returns how long login codes stay valid.
func (s *Service) CodeTTL() time.Duration { return s.policy.CodeTTL }

// decision is what happens when someone signs in.
type decision int

const (
	deny     decision = iota
	allow             // sign in, creating or activating the account
	approval          // prove the email, then wait for an administrator
)

// decide applies the sign-up rules to email. Active accounts always sign
// in and disabled ones never do; the very first account bootstraps the
// server whatever the mode.
func (s *Service) decide(ctx context.Context, email, invite string) (decision, error) {
	existing, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return deny, err
	}
	if existing != nil {
		switch {
		case existing.Status == user.StatusActive:
			return allow, nil
		case existing.Status == user.StatusDisabled:
			return deny, nil
		case existing.Email == "" && existing.EmailHash != "":
			// Re-enabled by an administrator: known by the hash of this email.
			return allow, nil
		}
	}
	count, err := s.users.Count(ctx)
	if err != nil {
		return deny, err
	}
	if count == 0 {
		return allow, nil
	}
	cfg, err := s.signup(ctx)
	if err != nil {
		return deny, err
	}
	switch cfg.Mode {
	case SignupOpen:
		return allow, nil
	case SignupClosed:
		return deny, nil
	case SignupApproval:
		if cfg.Allows(email) {
			return allow, nil
		}
		if s.gate != nil {
			ok, err := s.gate.Approved(ctx, email)
			if err != nil || ok {
				return allow, err
			}
		}
		return approval, nil
	}
	// Invite mode: the allowed list, people someone added by email, and
	// holders of a group invite link.
	if cfg.Allows(email) || existing != nil {
		return allow, nil
	}
	if invite != "" && s.invites != nil {
		ok, err := s.invites.Valid(ctx, invite)
		if err != nil || ok {
			return allow, err
		}
	}
	return deny, nil
}

// RequestCode emails a login code. It returns nil for disallowed emails too,
// so callers cannot probe which addresses have accounts. invite is an
// optional group invite token that lets a new person sign up. client is the
// caller's network address; the hourly limit counts per network as well as
// per email, so requests from elsewhere do not use up the owner's share.
func (s *Service) RequestCode(ctx context.Context, rawEmail, invite, client string) error {
	email, err := user.NormalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	client = hashClient(s.policy.Secret, client)
	var code string
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.challenges.Lock(ctx, email); err != nil {
			return err
		}
		total, own, err := s.challenges.CountSince(ctx, email, client, now.Add(-time.Hour))
		if err != nil {
			return err
		}
		if own >= s.policy.MaxCodesHour || total >= s.policy.MaxCodesEmail {
			return errs.RateLimited("Too many codes requested. Try again in an hour.")
		}
		d, err := s.decide(ctx, email, invite)
		if err != nil || d == deny {
			return err
		}
		if code, err = newCode(); err != nil {
			return err
		}
		c := &Challenge{ID: uuid.New(), Email: email, ExpiresAt: now.Add(s.policy.CodeTTL), CreatedAt: now, Client: client, Pending: d == approval}
		c.CodeHash = hashCode(s.policy.Secret, c.ID, email, code)
		return s.challenges.Create(ctx, c)
	})
	if err != nil || code == "" {
		return err
	}
	return s.notifier.LoginCode(ctx, email, code, s.policy.CodeTTL)
}

// Login is the result of a successful verification.
type Login struct {
	Token   string
	Session *Session
	User    *user.User
	IsNew   bool
}

// Verify checks a code and opens a session.
func (s *Service) Verify(ctx context.Context, rawEmail, code, name, userAgent string) (*Login, error) {
	email, err := user.NormalizeEmail(rawEmail)
	if err != nil {
		return nil, err
	}
	code = strings.TrimSpace(code)
	invalid := errs.Unauthorized("That code is invalid or expired. Request a new one.")
	now := s.clock.Now()

	// Every live code of the address counts this attempt before it is
	// compared, so codes requested by someone else do not hide the one the
	// owner holds, and no code is compared more than MaxAttempts times.
	live, err := s.challenges.Attempt(ctx, email, now, s.policy.MaxAttempts)
	if err != nil {
		return nil, err
	}
	var c *Challenge
	for _, x := range live {
		expected := hashCode(s.policy.Secret, x.ID, email, code)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(x.CodeHash)) == 1 {
			c = x
		}
	}
	if c == nil {
		return nil, invalid
	}

	// The code was only sent because the rules allowed it (an invite link,
	// say); a code for someone waiting for approval is checked again, in
	// case they were approved in the meantime.
	d := allow
	if c.Pending {
		if d, err = s.decide(ctx, email, ""); err != nil {
			return nil, err
		}
	}
	if d != allow {
		ok, err := s.challenges.Consume(ctx, c.ID, now)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, invalid
		}
		if d == deny || s.gate == nil {
			return nil, errs.Forbidden("New accounts are not being created on this server right now.")
		}
		declined, err := s.gate.Request(ctx, email, name)
		if err != nil {
			return nil, err
		}
		if declined {
			return nil, errs.Declined("Your request to join was declined. Ask an administrator of this server if you think that is a mistake.")
		}
		return nil, errs.Pending("Thanks! Your account is waiting for approval. You can sign in once an administrator or manager approves it.")
	}

	var login *Login
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		// A code opens one session, however many requests carry it.
		ok, err := s.challenges.Consume(ctx, c.ID, now)
		if err != nil {
			return err
		}
		if !ok {
			return invalid
		}
		u, isNew, err := s.activate(ctx, email, name, now)
		if err != nil {
			return err
		}
		token, hash, err := newToken("")
		if err != nil {
			return err
		}
		expires := now.Add(s.policy.SessionTTL)
		sess := &Session{ID: uuid.New(), UserID: u.ID, Kind: KindSession, TokenHash: hash, UserAgent: truncate(userAgent, 200), ExpiresAt: &expires, CreatedAt: now}
		if err := s.sessions.Create(ctx, sess); err != nil {
			return err
		}
		login = &Login{Token: token, Session: sess, User: u, IsNew: isNew}
		return nil
	})
	return login, err
}

func (s *Service) activate(ctx context.Context, email, name string, now time.Time) (*user.User, bool, error) {
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil, false, err
	}
	count, err := s.users.Count(ctx)
	if err != nil {
		return nil, false, err
	}
	if u == nil {
		u, err = user.New(email, name, user.StatusActive, now)
		if err != nil {
			return nil, false, err
		}
		if count == 0 {
			// The first account runs the server.
			u.Role = user.RoleAdmin
		}
		return u, true, s.users.Create(ctx, u)
	}
	u.Attach(email)
	if count == 0 && u.Status != user.StatusActive {
		u.Role = user.RoleAdmin
	}
	if u.Status == user.StatusActive {
		return u, false, nil
	}
	if err := u.Activate(name, now); err != nil {
		return nil, false, err
	}
	return u, true, s.users.Update(ctx, u)
}

// Authenticate resolves a bearer token to a principal.
func (s *Service) Authenticate(ctx context.Context, token string) (*Principal, error) {
	unauthorized := errs.Unauthorized("Sign in to continue.")
	if token == "" {
		return nil, unauthorized
	}
	sess, err := s.sessions.FindByHash(ctx, HashToken(token))
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if sess == nil || sess.Expired(now) {
		return nil, unauthorized
	}
	if sess.LastUsedAt == nil || now.Sub(*sess.LastUsedAt) > time.Minute {
		_ = s.sessions.Touch(ctx, sess.ID, now)
	}
	return &Principal{UserID: sess.UserID, SessionID: sess.ID, Kind: sess.Kind}, nil
}

// Logout revokes a session.
func (s *Service) Logout(ctx context.Context, p *Principal) error {
	return s.sessions.Delete(ctx, p.SessionID)
}

// IssuedToken is a freshly created API token; Token is only available now.
type IssuedToken struct {
	Session *Session
	Token   string
}

// CreateToken issues a personal API token.
func (s *Service) CreateToken(ctx context.Context, userID uuid.UUID, name string) (*IssuedToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 60 {
		return nil, errs.Validation("name", "Give the token a name of up to 60 characters.")
	}
	token, hash, err := newToken(TokenPrefix)
	if err != nil {
		return nil, err
	}
	sess := &Session{ID: uuid.New(), UserID: userID, Kind: KindToken, Name: name, TokenHash: hash, CreatedAt: s.clock.Now()}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	return &IssuedToken{Session: sess, Token: token}, nil
}

// Tokens lists a user's API tokens.
func (s *Service) Tokens(ctx context.Context, userID uuid.UUID) ([]*Session, error) {
	return s.sessions.ListTokens(ctx, userID)
}

// RevokeToken deletes one of the user's API tokens.
func (s *Service) RevokeToken(ctx context.Context, userID, id uuid.UUID) error {
	ok, err := s.sessions.DeleteToken(ctx, userID, id)
	if err != nil {
		return err
	}
	if !ok {
		return errs.NotFound("Token")
	}
	return nil
}

// Cleanup removes expired sessions and old challenges.
func (s *Service) Cleanup(ctx context.Context) error {
	now := s.clock.Now()
	if err := s.sessions.DeleteExpired(ctx, now); err != nil {
		return err
	}
	return s.challenges.DeleteBefore(ctx, now.Add(-24*time.Hour))
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
