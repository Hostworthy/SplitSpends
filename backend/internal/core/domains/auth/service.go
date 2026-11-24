package auth

import (
	"context"
	"crypto/subtle"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/backend/internal/core/domains/user"
	"github.com/Hostworthy/SplitSpends/backend/internal/core/ports"
	"github.com/Hostworthy/SplitSpends/backend/internal/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/backend/internal/core/shared/errs"
	"github.com/google/uuid"
)

// Service implements login and credential management.
type Service struct {
	policy     Policy
	challenges ChallengeRepository
	sessions   SessionRepository
	users      user.Repository
	notifier   ports.Notifier
	tx         ports.Tx
	clock      clock.Clock
}

// NewService builds a Service.
func NewService(p Policy, challenges ChallengeRepository, sessions SessionRepository, users user.Repository, notifier ports.Notifier, tx ports.Tx, c clock.Clock) *Service {
	return &Service{policy: p, challenges: challenges, sessions: sessions, users: users, notifier: notifier, tx: tx, clock: c}
}

// Mode returns the configured signup mode.
func (s *Service) Mode() SignupMode { return s.policy.Mode }

// CodeTTL returns how long login codes stay valid.
func (s *Service) CodeTTL() time.Duration { return s.policy.CodeTTL }

// allowed decides whether email may log in. Existing users always may.
func (s *Service) allowed(ctx context.Context, email string) (bool, error) {
	existing, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return false, err
	}
	if existing != nil {
		return true, nil
	}
	switch s.policy.Mode {
	case SignupOpen:
		return true, nil
	case SignupDomain:
		if s.policy.domainAllowed(email) {
			return true, nil
		}
	}
	count, err := s.users.Count(ctx)
	if err != nil {
		return false, err
	}
	// The very first account bootstraps the instance.
	return count == 0, nil
}

// RequestCode emails a login code. It returns nil for disallowed emails too,
// so callers cannot probe which addresses have accounts.
func (s *Service) RequestCode(ctx context.Context, rawEmail string) error {
	email, err := user.NormalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	recent, err := s.challenges.CountSince(ctx, email, now.Add(-time.Hour))
	if err != nil {
		return err
	}
	if recent >= s.policy.MaxCodesHour {
		return errs.RateLimited("Too many codes requested. Try again in an hour.")
	}
	ok, err := s.allowed(ctx, email)
	if err != nil || !ok {
		return err
	}
	code, err := newCode()
	if err != nil {
		return err
	}
	c := &Challenge{ID: uuid.New(), Email: email, ExpiresAt: now.Add(s.policy.CodeTTL), CreatedAt: now}
	c.CodeHash = hashCode(s.policy.Secret, c.ID, email, code)
	if err := s.challenges.Create(ctx, c); err != nil {
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

	c, err := s.challenges.Latest(ctx, email)
	if err != nil {
		return nil, err
	}
	if c == nil || !c.Usable(now, s.policy.MaxAttempts) {
		return nil, invalid
	}
	expected := hashCode(s.policy.Secret, c.ID, email, code)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(c.CodeHash)) != 1 {
		c.Attempts++
		if err := s.challenges.Update(ctx, c); err != nil {
			return nil, err
		}
		return nil, invalid
	}

	var login *Login
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		c.ConsumedAt = &now
		if err := s.challenges.Update(ctx, c); err != nil {
			return err
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
	if u == nil {
		u, err = user.New(email, name, user.StatusActive, now)
		if err != nil {
			return nil, false, err
		}
		return u, true, s.users.Create(ctx, u)
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
