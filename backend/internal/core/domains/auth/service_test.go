package auth

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/backend/internal/core/domains/user"
	"github.com/Hostworthy/SplitSpends/backend/internal/core/ports"
	"github.com/Hostworthy/SplitSpends/backend/internal/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/backend/internal/core/shared/errs"
	"github.com/google/uuid"
)

type memChallenges struct{ list []*Challenge }

func (m *memChallenges) Create(_ context.Context, c *Challenge) error {
	cp := *c
	m.list = append(m.list, &cp)
	return nil
}
func (m *memChallenges) Latest(_ context.Context, email string) (*Challenge, error) {
	for i := len(m.list) - 1; i >= 0; i-- {
		if m.list[i].Email == email {
			cp := *m.list[i]
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *memChallenges) Update(_ context.Context, c *Challenge) error {
	for i, x := range m.list {
		if x.ID == c.ID {
			cp := *c
			m.list[i] = &cp
		}
	}
	return nil
}
func (m *memChallenges) CountSince(_ context.Context, email string, since time.Time) (int, error) {
	n := 0
	for _, c := range m.list {
		if c.Email == email && !c.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}
func (m *memChallenges) DeleteBefore(context.Context, time.Time) error { return nil }

type memSessions struct{ byHash map[string]*Session }

func (m *memSessions) Create(_ context.Context, s *Session) error {
	m.byHash[s.TokenHash] = s
	return nil
}
func (m *memSessions) FindByHash(_ context.Context, h string) (*Session, error) {
	return m.byHash[h], nil
}
func (m *memSessions) Touch(context.Context, uuid.UUID, time.Time) error { return nil }
func (m *memSessions) Delete(_ context.Context, id uuid.UUID) error {
	for h, s := range m.byHash {
		if s.ID == id {
			delete(m.byHash, h)
		}
	}
	return nil
}
func (m *memSessions) ListTokens(context.Context, uuid.UUID) ([]*Session, error) { return nil, nil }
func (m *memSessions) DeleteToken(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}
func (m *memSessions) DeleteExpired(context.Context, time.Time) error { return nil }

type memUsers struct{ byEmail map[string]*user.User }

func (m *memUsers) Get(_ context.Context, id uuid.UUID) (*user.User, error) {
	for _, u := range m.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errs.NotFound("User")
}
func (m *memUsers) FindByEmail(_ context.Context, e string) (*user.User, error) {
	return m.byEmail[e], nil
}
func (m *memUsers) List(context.Context, []uuid.UUID) ([]*user.User, error) { return nil, nil }
func (m *memUsers) Create(_ context.Context, u *user.User) error            { m.byEmail[u.Email] = u; return nil }
func (m *memUsers) Update(_ context.Context, u *user.User) error            { m.byEmail[u.Email] = u; return nil }
func (m *memUsers) Count(context.Context) (int, error) {
	n := 0
	for _, u := range m.byEmail {
		if u.Status == user.StatusActive {
			n++
		}
	}
	return n, nil
}

type codes map[string]string

func (c codes) LoginCode(_ context.Context, to, code string, _ time.Duration) error {
	c[to] = code
	return nil
}
func (codes) Invite(context.Context, string, string) error { return nil }
func (codes) Notify(context.Context, ports.Notification)   {}

type noTx struct{}

func (noTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func service(mode SignupMode) (*Service, *memUsers, codes) {
	p := DefaultPolicy()
	p.Secret, p.Mode = []byte("test-secret-test-secret-test-secret"), mode
	users := &memUsers{byEmail: map[string]*user.User{}}
	sent := codes{}
	now := clock.Fixed{At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}
	return NewService(p, &memChallenges{}, &memSessions{byHash: map[string]*Session{}}, users, sent, noTx{}, now), users, sent
}

func TestLoginFlow(t *testing.T) {
	ctx := context.Background()
	s, _, sent := service(SignupInvite)
	if err := s.RequestCode(ctx, " Ada@Example.com "); err != nil {
		t.Fatal(err)
	}
	code := sent["ada@example.com"]
	if len(code) != 6 {
		t.Fatalf("expected a 6 digit code, got %q", code)
	}
	login, err := s.Verify(ctx, "ada@example.com", code, "Ada", "test")
	if err != nil || !login.IsNew || login.User.Name != "Ada" {
		t.Fatalf("verify: %+v %v", login, err)
	}
	p, err := s.Authenticate(ctx, login.Token)
	if err != nil || p.UserID != login.User.ID {
		t.Fatalf("authenticate: %v", err)
	}
	if _, err := s.Verify(ctx, "ada@example.com", code, "", ""); !errs.Is(err, errs.KindUnauthorized) {
		t.Fatalf("a code must work only once, got %v", err)
	}
	if err := s.Logout(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, login.Token); !errs.Is(err, errs.KindUnauthorized) {
		t.Fatal("token must be revoked after logout")
	}
}

func TestInviteModeIgnoresStrangers(t *testing.T) {
	ctx := context.Background()
	s, users, sent := service(SignupInvite)
	first, _ := user.New("first@example.com", "First", user.StatusActive, time.Now())
	_ = users.Create(ctx, first)
	if err := s.RequestCode(ctx, "stranger@example.com"); err != nil {
		t.Fatalf("strangers get the same response, got %v", err)
	}
	if _, ok := sent["stranger@example.com"]; ok {
		t.Fatal("no code may be sent to strangers in invite mode")
	}
	invited, _ := user.New("friend@example.com", "Friend", user.StatusInvited, time.Now())
	_ = users.Create(ctx, invited)
	if err := s.RequestCode(ctx, "friend@example.com"); err != nil || sent["friend@example.com"] == "" {
		t.Fatal("invited users must receive a code")
	}
}

func TestAttemptsAreLimited(t *testing.T) {
	ctx := context.Background()
	s, _, sent := service(SignupOpen)
	_ = s.RequestCode(ctx, "ada@example.com")
	for i := 0; i < 5; i++ {
		if _, err := s.Verify(ctx, "ada@example.com", "999999x", "", ""); err == nil {
			t.Fatal("wrong code accepted")
		}
	}
	if _, err := s.Verify(ctx, "ada@example.com", sent["ada@example.com"], "", ""); !errs.Is(err, errs.KindUnauthorized) {
		t.Fatal("the right code must fail after too many attempts")
	}
}

func TestCodeRequestsAreRateLimited(t *testing.T) {
	ctx := context.Background()
	s, _, _ := service(SignupOpen)
	for i := 0; i < 5; i++ {
		if err := s.RequestCode(ctx, "ada@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RequestCode(ctx, "ada@example.com"); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("sixth request in an hour must be limited, got %v", err)
	}
}

func TestDomainMode(t *testing.T) {
	ctx := context.Background()
	s, users, sent := service(SignupDomain)
	s.policy.AllowedDomains = []string{"example.org"}
	first, _ := user.New("first@example.com", "First", user.StatusActive, time.Now())
	_ = users.Create(ctx, first)
	_ = s.RequestCode(ctx, "x@example.org")
	_ = s.RequestCode(ctx, "x@evil.org")
	if sent["x@example.org"] == "" || sent["x@evil.org"] != "" {
		t.Fatalf("domain policy not applied: %v", sent)
	}
}
