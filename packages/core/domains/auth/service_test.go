package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type memChallenges struct{ list []*Challenge }

func (m *memChallenges) Create(_ context.Context, c *Challenge) error {
	cp := *c
	m.list = append(m.list, &cp)
	return nil
}
func (m *memChallenges) Lock(context.Context, string) error { return nil }
func (m *memChallenges) CountSince(_ context.Context, email, client string, since time.Time) (int, int, error) {
	total, own := 0, 0
	for _, c := range m.list {
		if c.Email == email && !c.CreatedAt.Before(since) {
			total++
			if c.Client == client {
				own++
			}
		}
	}
	return total, own, nil
}
func (m *memChallenges) Attempt(_ context.Context, email string, now time.Time, limit int) ([]*Challenge, error) {
	var live []*Challenge
	for _, c := range m.list {
		if c.Email == email && c.Usable(now, limit) {
			c.Attempts++
			cp := *c
			live = append(live, &cp)
		}
	}
	return live, nil
}
func (m *memChallenges) Consume(_ context.Context, id uuid.UUID, at time.Time) (bool, error) {
	for _, c := range m.list {
		if c.ID == id && c.ConsumedAt == nil {
			c.ConsumedAt = &at
			return true, nil
		}
	}
	return false, nil
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
func (m *memUsers) Placeholders(context.Context, uuid.UUID) (int, error) { return 0, nil }
func (m *memUsers) Lock(context.Context) error                           { return nil }

type codes map[string]string

func (c codes) LoginCode(_ context.Context, to, code string, _ time.Duration) error {
	c[to] = code
	return nil
}
func (codes) Invite(context.Context, string, string) error { return nil }
func (codes) Notify(context.Context, ports.Notification)   {}

// links are the invite tokens that are currently valid. A link's storage
// hash is its token with a "hash:" prefix.
type links map[string]bool

func (l links) Lookup(_ context.Context, token string) (string, error) {
	if !l[token] {
		return "", nil
	}
	return "hash:" + token, nil
}
func (l links) Live(_ context.Context, hash string) (bool, error) {
	token, ok := strings.CutPrefix(hash, "hash:")
	return ok && l[token], nil
}

type noTx struct{}

func (noTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func service(mode SignupMode) (*Service, *memUsers, codes) {
	s, users, sent, _ := withLinks(mode)
	return s, users, sent
}

func withLinks(mode SignupMode) (*Service, *memUsers, codes, links) {
	p := DefaultPolicy()
	p.Secret, p.Mode = []byte("test-secret-test-secret-test-secret"), mode
	users := &memUsers{byEmail: map[string]*user.User{}}
	sent := codes{}
	now := clock.Fixed{At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}
	invites := links{"valid-link": true}
	return NewService(p, &memChallenges{}, &memSessions{byHash: map[string]*Session{}}, users, invites, sent, noTx{}, now), users, sent, invites
}

func TestLoginFlow(t *testing.T) {
	ctx := context.Background()
	s, _, sent := service(SignupInvite)
	if err := s.RequestCode(ctx, " Ada@Example.com ", "", ""); err != nil {
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
	if err := s.RequestCode(ctx, "stranger@example.com", "", ""); err != nil {
		t.Fatalf("strangers get the same response, got %v", err)
	}
	if _, ok := sent["stranger@example.com"]; ok {
		t.Fatal("no code may be sent to strangers in invite mode")
	}
	invited, _ := user.New("friend@example.com", "Friend", user.StatusInvited, time.Now())
	_ = users.Create(ctx, invited)
	if err := s.RequestCode(ctx, "friend@example.com", "", ""); err != nil || sent["friend@example.com"] == "" {
		t.Fatal("invited users must receive a code")
	}
}

func TestAttemptsAreLimited(t *testing.T) {
	ctx := context.Background()
	s, _, sent := service(SignupOpen)
	_ = s.RequestCode(ctx, "ada@example.com", "", "")
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
		if err := s.RequestCode(ctx, "ada@example.com", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RequestCode(ctx, "ada@example.com", "", ""); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("sixth request in an hour must be limited, got %v", err)
	}
}

func TestDomainMode(t *testing.T) {
	ctx := context.Background()
	s, users, sent := service(SignupDomain)
	s.policy.AllowedDomains = []string{"example.org"}
	first, _ := user.New("first@example.com", "First", user.StatusActive, time.Now())
	_ = users.Create(ctx, first)
	_ = s.RequestCode(ctx, "x@example.org", "", "")
	_ = s.RequestCode(ctx, "x@evil.org", "", "")
	if sent["x@example.org"] == "" || sent["x@evil.org"] != "" {
		t.Fatalf("domain policy not applied: %v", sent)
	}
}

func TestInviteLinkLetsStrangersSignUp(t *testing.T) {
	ctx := context.Background()
	s, users, sent := service(SignupInvite)
	first, _ := user.New("first@example.com", "First", user.StatusActive, time.Now())
	_ = users.Create(ctx, first)

	// Same response with a wrong token, and nothing is sent.
	if err := s.RequestCode(ctx, "stranger@example.com", "expired-or-unknown", ""); err != nil {
		t.Fatalf("an invalid link must look like any other request, got %v", err)
	}
	if _, ok := sent["stranger@example.com"]; ok {
		t.Fatal("an invalid link must not let a stranger in")
	}
	if err := s.RequestCode(ctx, "stranger@example.com", "valid-link", ""); err != nil {
		t.Fatal(err)
	}
	code := sent["stranger@example.com"]
	if code == "" {
		t.Fatal("a valid invite link must let a stranger request a code")
	}
	login, err := s.Verify(ctx, "stranger@example.com", code, "Stranger", "test")
	if err != nil || !login.IsNew {
		t.Fatalf("verify: %+v %v", login, err)
	}
}

func TestInviteLinkWorksInDomainMode(t *testing.T) {
	ctx := context.Background()
	s, users, sent := service(SignupDomain)
	s.policy.AllowedDomains = []string{"example.org"}
	first, _ := user.New("first@example.com", "First", user.StatusActive, time.Now())
	_ = users.Create(ctx, first)
	_ = s.RequestCode(ctx, "x@evil.org", "", "")
	_ = s.RequestCode(ctx, "y@evil.org", "valid-link", "")
	if sent["x@evil.org"] != "" || sent["y@evil.org"] == "" {
		t.Fatalf("only the invited address may sign up: %v", sent)
	}
}

func TestStrangerCannotLockOutOwner(t *testing.T) {
	ctx := context.Background()
	s, _, sent := service(SignupOpen)
	// The owner asks for a code; then someone else, on another network, asks
	// for as many as they may and guesses wrong.
	if err := s.RequestCode(ctx, "ada@example.com", "", "owner"); err != nil {
		t.Fatal(err)
	}
	held := sent["ada@example.com"]
	for i := 0; i < s.policy.MaxCodesHour; i++ {
		if err := s.RequestCode(ctx, "ada@example.com", "", "stranger"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RequestCode(ctx, "ada@example.com", "", "stranger"); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("the stranger's network must run out of codes, got %v", err)
	}
	if _, err := s.Verify(ctx, "ada@example.com", held, "", ""); err != nil {
		t.Fatalf("newer codes must not hide the one the owner holds: %v", err)
	}

	// Wrong guesses use up every live code; the owner's network can still
	// get a new one, and it works.
	for i := 0; i < s.policy.MaxAttempts; i++ {
		if _, err := s.Verify(ctx, "ada@example.com", "000000x", "", ""); !errs.Is(err, errs.KindUnauthorized) {
			t.Fatalf("wrong code: %v", err)
		}
	}
	if err := s.RequestCode(ctx, "ada@example.com", "", "owner"); err != nil {
		t.Fatalf("the owner must still get a code: %v", err)
	}
	if _, err := s.Verify(ctx, "ada@example.com", sent["ada@example.com"], "", ""); err != nil {
		t.Fatalf("the owner's new code must work: %v", err)
	}
}

func TestCodeRequestsAreLimitedPerEmail(t *testing.T) {
	ctx := context.Background()
	s, _, _ := service(SignupOpen)
	for i := 0; i < s.policy.MaxCodesEmail; i++ {
		if err := s.RequestCode(ctx, "ada@example.com", "", fmt.Sprintf("network-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RequestCode(ctx, "ada@example.com", "", "another-network"); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("all networks together must be limited, got %v", err)
	}
	if err := s.RequestCode(ctx, "ben@example.com", "", "another-network"); err != nil {
		t.Fatalf("other addresses are not affected: %v", err)
	}
}

func TestVerifyAppliesTheRulesAgain(t *testing.T) {
	ctx := context.Background()
	s, users, sent, invites := withLinks(SignupInvite)
	first, _ := user.New("first@example.com", "First", user.StatusActive, time.Now())
	_ = users.Create(ctx, first)
	friend, _ := user.New("friend@example.com", "Friend", user.StatusInvited, time.Now())
	_ = users.Create(ctx, friend)
	for _, email := range []string{"friend@example.com", "stranger@example.com"} {
		if err := s.RequestCode(ctx, email, "valid-link", ""); err != nil {
			t.Fatal(err)
		}
	}

	// The link is revoked: the stranger's code no longer creates an account.
	delete(invites, "valid-link")
	if _, err := s.Verify(ctx, "stranger@example.com", sent["stranger@example.com"], "", ""); !errs.Is(err, errs.KindForbidden) {
		t.Fatalf("a revoked link must not let a stranger in, got %v", err)
	}
	if users.byEmail["stranger@example.com"] != nil {
		t.Fatal("no account may be created")
	}

	// Sign-up closes: the invited person's code no longer activates them.
	s.policy.Mode = SignupClosed
	if _, err := s.Verify(ctx, "friend@example.com", sent["friend@example.com"], "", ""); !errs.Is(err, errs.KindForbidden) {
		t.Fatalf("closed mode must not activate accounts, got %v", err)
	}
	if users.byEmail["friend@example.com"].Status != user.StatusInvited {
		t.Fatal("the invited account must stay invited")
	}
}

// kind is the kind of err, or "" for nil.
func kind(err error) errs.Kind {
	if e, ok := errs.As(err); ok {
		return e.Kind
	}
	return ""
}

func TestRequestCodeLooksTheSameForStrangers(t *testing.T) {
	ctx := context.Background()
	s, users, sent := service(SignupInvite)
	first, _ := user.New("first@example.com", "First", user.StatusActive, time.Now())
	_ = users.Create(ctx, first)
	friend, _ := user.New("friend@example.com", "Friend", user.StatusInvited, time.Now())
	_ = users.Create(ctx, friend)
	for i := 0; i <= s.policy.MaxCodesHour; i++ {
		stranger := s.RequestCode(ctx, "stranger@example.com", "", "")
		invited := s.RequestCode(ctx, "friend@example.com", "", "")
		if kind(stranger) != kind(invited) {
			t.Fatalf("request %d: stranger got %v, invited person got %v", i+1, stranger, invited)
		}
	}
	if _, ok := sent["stranger@example.com"]; ok {
		t.Fatal("no code may be sent to strangers")
	}
	if _, err := s.Verify(ctx, "stranger@example.com", "", "", ""); !errs.Is(err, errs.KindUnauthorized) {
		t.Fatalf("a refused request has no code to verify, got %v", err)
	}
}
