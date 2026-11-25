// Package bootstraptest wires the services against a disposable PostgreSQL
// database for tests in any package. Production code never imports it.
//
// Every test that calls Open gets an empty, fully migrated schema. Tests in
// different packages run in parallel processes against the same database, so
// Open holds a database-wide lock until the test ends: database tests run one
// at a time and never see each other's rows.
package bootstraptest

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/storage"
)

// testLock serialises database tests across test binaries.
const testLock = 7_301_451_983

// Options adjust the environment Open builds. The zero value is an open
// instance whose clock stands at a fixed instant.
type Options struct {
	// Config replaces Defaults(). Secret and BaseURL are filled in when empty.
	Config *bootstrap.Config
	// Now is where the clock starts; zero means 2026-01-01 12:00 UTC.
	Now time.Time
	// Sender delivers Web Push; nil turns push off unless the config has keys.
	Sender push.Sender
	// Reader reads receipts and drafts expenses; nil turns AI off.
	Reader ports.Assistant
	// Blobs wraps the store images are kept in, which is on disk; nil
	// keeps it as it is.
	Blobs func(ports.BlobStore) ports.BlobStore
}

// Env is a wired set of services on a fresh schema.
type Env struct {
	Services *bootstrap.Services
	Config   *bootstrap.Config
	DB       *database.DB
	Blobs    ports.BlobStore
	Notifier *Notifier
	Clock    *Clock
}

// Defaults is the configuration tests start from: open sign-up and a fixed
// secret.
func Defaults() *bootstrap.Config {
	return &bootstrap.Config{AppName: "SplitSpends", Secret: []byte("0123456789abcdef0123456789abcdef"),
		SignupMode: auth.SignupOpen, BaseURL: "http://test", AIRateLimit: 30}
}

// Open migrates an empty schema on TEST_DATABASE_URL and wires the services
// on it. It skips the test when TEST_DATABASE_URL is not set.
func Open(t testing.TB, o Options) *Env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, url, 9)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	lock, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The lock belongs to this connection's session, so it is released when
	// the connection goes back to the pool and the pool closes.
	if _, err := lock.Exec(ctx, "SELECT pg_advisory_lock($1)", testLock); err != nil {
		lock.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = lock.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", testLock)
		lock.Release()
	})
	if _, err := lock.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	var blobs ports.BlobStore
	if blobs, err = storage.NewLocal(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if o.Blobs != nil {
		blobs = o.Blobs(blobs)
	}

	cfg := o.Config
	if cfg == nil {
		cfg = Defaults()
	}
	if len(cfg.Secret) == 0 {
		cfg.Secret = Defaults().Secret
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = Defaults().BaseURL
	}
	now := o.Now
	if now.IsZero() {
		now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	}
	env := &Env{Config: cfg, DB: &database.DB{Pool: pool}, Blobs: blobs, Notifier: NewNotifier(), Clock: NewClock(now)}
	env.Services = bootstrap.WireWithPush(env.DB, env.Notifier, blobs, o.Reader, o.Sender, env.Clock, cfg, slog.New(slog.DiscardHandler))
	return env
}

// SignIn requests a code for email and verifies it, creating the account
// when it is new. It fails the test when either step fails.
func (e *Env) SignIn(t testing.TB, email, name string) *auth.Login {
	t.Helper()
	return e.Join(t, email, name, "")
}

// Join signs in like SignIn with an invite link token, which invite-only
// instances need for new accounts.
func (e *Env) Join(t testing.TB, email, name, invite string) *auth.Login {
	t.Helper()
	ctx := context.Background()
	e.Notifier.Forget(email)
	if err := e.Services.Auth.RequestCode(ctx, email, invite, ""); err != nil {
		t.Fatalf("request a code for %s: %v", email, err)
	}
	code := e.Notifier.Code(email)
	if code == "" {
		t.Fatalf("no code was sent to %s", email)
	}
	login, err := e.Services.Auth.Verify(ctx, email, code, name, "test")
	if err != nil {
		t.Fatalf("verify the code for %s: %v", email, err)
	}
	return login
}

// Notifier records login codes, invitations and notifications instead of
// sending them.
type Notifier struct {
	mu      sync.Mutex
	codes   map[string]string
	invites []string
	sent    []ports.Notification
}

// NewNotifier returns an empty Notifier.
func NewNotifier() *Notifier { return &Notifier{codes: map[string]string{}} }

func (n *Notifier) LoginCode(_ context.Context, to, code string, _ time.Duration) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.codes[to] = code
	return nil
}

func (n *Notifier) Invite(_ context.Context, to, _ string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.invites = append(n.invites, to)
	return nil
}

// Invites returns the addresses invitations were sent to, in order.
func (n *Notifier) Invites() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.invites...)
}

func (n *Notifier) Notify(_ context.Context, msg ports.Notification) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, msg)
}

// Code returns the last login code sent to email, or "" when none was.
func (n *Notifier) Code(email string) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.codes[email]
}

// Forget drops the login code sent to email, so a later Code shows whether a
// new one went out.
func (n *Notifier) Forget(email string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.codes, email)
}

// Sent returns the notifications recorded so far.
func (n *Notifier) Sent() []ports.Notification {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]ports.Notification(nil), n.sent...)
}

// Subjects returns the subjects of the notifications emailed to email.
func (n *Notifier) Subjects(email string) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var subjects []string
	for _, msg := range n.sent {
		if msg.To == email {
			subjects = append(subjects, msg.Subject)
		}
	}
	return subjects
}

// Reset drops the recorded invitations and notifications.
func (n *Notifier) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.invites, n.sent = nil, nil
}

// Clock is a clock that moves only when a test moves it.
type Clock struct {
	mu sync.Mutex
	at time.Time
}

// NewClock returns a Clock standing at at.
func NewClock(at time.Time) *Clock { return &Clock{at: at} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

// Add moves the clock forward by d.
func (c *Clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}
