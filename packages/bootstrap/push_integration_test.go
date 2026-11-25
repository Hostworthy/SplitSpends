package bootstrap_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/webpush"
	"github.com/google/uuid"
)

// fakeSender stands in for the push service: it records what each
// endpoint got and answers 410 for endpoints ending in /gone.
type fakeSender struct {
	mu   sync.Mutex
	sent map[string][]push.Message
}

func (f *fakeSender) Send(_ context.Context, s *push.Subscription, payload []byte) error {
	if strings.HasSuffix(s.Endpoint, "/gone") {
		return push.ErrGone
	}
	var m push.Message
	if err := json.Unmarshal(payload, &m); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent[s.Endpoint] = append(f.sent[s.Endpoint], m)
	return nil
}

func (f *fakeSender) PublicKey() string { return "test-key" }

// pushSetup wires services with push turned on and delivered before Notify
// returns.
func pushSetup(t *testing.T, now time.Time) (*bootstraptest.Env, *fakeSender) {
	t.Helper()
	cfg := bootstraptest.Defaults()
	cfg.Serverless = true
	sender := &fakeSender{sent: map[string][]push.Message{}}
	return bootstraptest.Open(t, bootstraptest.Options{Config: cfg, Now: now, Sender: sender}), sender
}

func deviceKeys(t *testing.T) (string, string) {
	t.Helper()
	// Any valid uncompressed point will do for the fake sender.
	public, _, err := webpush.Keys()
	if err != nil {
		t.Fatal(err)
	}
	return public, base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))
}

func TestPushSubscriptionsAndDelivery(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	env, sender := pushSetup(t, now)
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	benLogin := env.SignIn(t, "ben@example.com", "Ben")
	ben, benSession := benLogin.User.ID, benLogin.Session.ID
	p256dh, auth := deviceKeys(t)

	phone := push.Input{Endpoint: "https://ntfy.example.com/upAbc?up=1", P256dh: p256dh, Auth: auth, Platform: "unifiedpush", Label: "Pixel"}
	first, created, err := svc.Push.Register(ctx, ben, benSession, phone)
	if err != nil || !created {
		t.Fatalf("register: created=%v err=%v", created, err)
	}
	again, created, err := svc.Push.Register(ctx, ben, benSession, phone)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("re-register must be idempotent: created=%v id=%v err=%v", created, again.ID, err)
	}
	if _, _, err := svc.Push.Register(ctx, ben, benSession, push.Input{Endpoint: "https://push.example.com/gone", P256dh: p256dh, Auth: auth}); err != nil {
		t.Fatal(err)
	}
	devices, err := svc.Push.List(ctx, ben)
	if err != nil || len(devices) != 2 {
		t.Fatalf("list: %d devices, err %v", len(devices), err)
	}
	if mine, _ := svc.Push.List(ctx, ada); len(mine) != 0 {
		t.Fatal("devices are private to their owner")
	}

	// Ben turns off email for new expenses but keeps push.
	off := false
	if _, err := svc.Users.Update(ctx, ben, user.Patch{Notifications: &user.NotificationsPatch{SwitchesPatch: user.SwitchesPatch{ExpenseAdded: &off}}}); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.Users.Get(ctx, ben)
	if err != nil || stored.Notifications.ExpenseAdded || !stored.Notifications.Push.ExpenseAdded {
		t.Fatalf("preferences did not round-trip: %+v %v", stored.Notifications, err)
	}

	name := "Flat"
	g, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}, Members: []group.Invitee{{Email: "ben@example.com"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Expenses.Create(ctx, ada, expense.Input{GroupID: &g.ID, Description: "Rent", Currency: "EUR", Total: 100000,
		Date: "2026-11-20", Method: expense.MethodEqual,
		Payers: []expense.Payer{{UserID: ada, Amount: 100000}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
	if err != nil {
		t.Fatal(err)
	}

	got := sender.sent[phone.Endpoint]
	if len(got) != 2 {
		t.Fatalf("Ben's phone should get 'added to group' and 'expense added', got %+v", got)
	}
	if !strings.Contains(got[1].Title, "Rent") || !strings.Contains(got[1].Body, "€500.00") || got[1].URL != "http://test" {
		t.Fatalf("unexpected expense push %+v", got[1])
	}
	for _, n := range env.Notifier.Sent() {
		if n.To == "ben@example.com" && strings.Contains(n.Subject, "Rent") {
			t.Fatalf("Ben turned off expense emails but got one: %+v", n)
		}
	}
	if devices, _ := svc.Push.List(ctx, ben); len(devices) != 1 || devices[0].LastSuccessAt == nil {
		t.Fatalf("the gone device should be removed and the phone marked delivered: %+v", devices)
	}

	report, err := svc.Push.Test(ctx, ben)
	if err != nil || report.Sent != 1 {
		t.Fatalf("test push: %+v %v", report, err)
	}
	if err := svc.Push.Remove(ctx, ada, first.ID); err == nil {
		t.Fatal("people cannot remove someone else's device")
	}
	if err := svc.Push.Remove(ctx, ben, first.ID); err != nil {
		t.Fatal(err)
	}
}

func TestVAPIDKeysAreShared(t *testing.T) {
	env, _ := pushSetup(t, time.Now())
	ctx := context.Background()
	keys := repositories.NewVAPIDKeys(env.DB)
	pub1, priv1, _ := webpush.Keys()
	gotPub, gotPriv, err := keys.Ensure(ctx, pub1, priv1)
	if err != nil || gotPub != pub1 || gotPriv != priv1 {
		t.Fatalf("first start should store its pair: %v", err)
	}
	pub2, priv2, _ := webpush.Keys()
	gotPub, gotPriv, err = keys.Ensure(ctx, pub2, priv2)
	if err != nil || gotPub != pub1 || gotPriv != priv1 {
		t.Fatal("later starts must reuse the stored pair")
	}
}

// TestGeneratedKeysEnablePush checks that a server without VAPID keys in
// its environment starts with push on, using the stored pair, and that the
// keys reach the push service through the sender alone: the configuration
// stays as it was loaded.
func TestGeneratedKeysEnablePush(t *testing.T) {
	env, _ := pushSetup(t, time.Now())
	ctx := context.Background()
	cfg := bootstraptest.Defaults()
	cfg.DatabaseURL, cfg.DatabaseConns, cfg.DataDir = os.Getenv("TEST_DATABASE_URL"), 2, t.TempDir()
	cfg.VAPIDSubject = "mailto:admin@example.com"
	rt, err := bootstrap.Load(ctx, cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	stored, _, err := repositories.NewVAPIDKeys(env.DB).Ensure(ctx, "unused", "unused")
	if err != nil {
		t.Fatal(err)
	}
	if !rt.Services.Push.Enabled() || rt.Services.Push.PublicKey() != stored {
		t.Fatalf("push should be on with the stored key %q, got enabled=%v key %q", stored, rt.Services.Push.Enabled(), rt.Services.Push.PublicKey())
	}
	if cfg.VAPIDPublicKey != "" || cfg.VAPIDPrivateKey != "" {
		t.Fatal("generated keys must not be written into the configuration")
	}
}

// TestDevicesEndWithTheirSession checks that a device stops receiving the
// account's notifications once the session or token that registered it
// ends: on sign-out, on revoking the token, and when it expires.
func TestDevicesEndWithTheirSession(t *testing.T) {
	ctx := context.Background()
	env, _ := pushSetup(t, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))
	svc := env.Services
	p256dh, keyAuth := deviceKeys(t)
	device := func(endpoint string) push.Input { return push.Input{Endpoint: endpoint, P256dh: p256dh, Auth: keyAuth} }
	principal := func(l *auth.Login) *auth.Principal {
		return &auth.Principal{UserID: l.User.ID, SessionID: l.Session.ID, Kind: l.Session.Kind}
	}
	endpoints := func(userID uuid.UUID) []string {
		t.Helper()
		subs, err := svc.Push.List(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, s := range subs {
			out = append(out, s.Endpoint)
		}
		return out
	}

	laptop := env.SignIn(t, "ben@example.com", "Ben")
	phone := env.SignIn(t, "ben@example.com", "Ben")
	ben := laptop.User.ID
	token, err := svc.Auth.CreateToken(ctx, principal(phone), "script")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		session  uuid.UUID
		endpoint string
	}{
		{laptop.Session.ID, "https://push.example.com/laptop"},
		{phone.Session.ID, "https://push.example.com/phone"},
		{token.Session.ID, "https://push.example.com/token"},
	} {
		if _, _, err := svc.Push.Register(ctx, ben, r.session, device(r.endpoint)); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.Auth.Logout(ctx, principal(laptop)); err != nil {
		t.Fatal(err)
	}
	if got := endpoints(ben); len(got) != 2 || slices.Contains(got, "https://push.example.com/laptop") {
		t.Fatalf("signing out should remove the laptop, got %v", got)
	}
	if err := svc.Auth.RevokeToken(ctx, principal(phone), token.Session.ID); err != nil {
		t.Fatal(err)
	}
	if got := endpoints(ben); !slices.Equal(got, []string{"https://push.example.com/phone"}) {
		t.Fatalf("revoking the token should remove its device, got %v", got)
	}

	// The phone's session expires and the cleanup job removes it.
	env.Clock.Add(phone.Session.ExpiresAt.Sub(env.Clock.Now()) + time.Minute)
	if err := svc.Auth.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if got := endpoints(ben); len(got) != 0 {
		t.Fatalf("an expired session should take its device with it, got %v", got)
	}

	// A device signed in again under a new session follows that session.
	again := env.SignIn(t, "ben@example.com", "Ben")
	if _, _, err := svc.Push.Register(ctx, ben, again.Session.ID, device("https://push.example.com/phone")); err != nil {
		t.Fatal(err)
	}
	if got := endpoints(ben); len(got) != 1 {
		t.Fatalf("registering again should add the device back, got %v", got)
	}
}

// TestRegisterKeepsFailures checks that the same device registering again
// keeps its count of failed deliveries, so one that never answers is still
// removed in the end, while a new subscription with new keys starts over.
func TestRegisterKeepsFailures(t *testing.T) {
	env, _ := pushSetup(t, time.Now())
	ctx := context.Background()
	benLogin := env.SignIn(t, "ben@example.com", "Ben")
	ben, benSession := benLogin.User.ID, benLogin.Session.ID
	p256dh, auth := deviceKeys(t)
	device := push.Input{Endpoint: "https://push.example.com/down", P256dh: p256dh, Auth: auth}
	sub, _, err := env.Services.Push.Register(ctx, ben, benSession, device)
	if err != nil {
		t.Fatal(err)
	}
	repo := repositories.NewPushSubscriptions(env.DB)
	for range 3 {
		if _, err := repo.Failed(ctx, sub.ID); err != nil {
			t.Fatal(err)
		}
	}
	again, _, err := env.Services.Push.Register(ctx, ben, benSession, device)
	if err != nil || again.Failures != 3 {
		t.Fatalf("the same keys should keep the failures: %+v %v", again, err)
	}
	device.P256dh, _ = deviceKeys(t)
	fresh, _, err := env.Services.Push.Register(ctx, ben, benSession, device)
	if err != nil || fresh.Failures != 0 {
		t.Fatalf("new keys should start over: %+v %v", fresh, err)
	}
}

// TestRegisterCapIsAtomic checks the device cap under concurrent
// registrations, and for devices moved from another account.
func TestRegisterCapIsAtomic(t *testing.T) {
	env, _ := pushSetup(t, time.Now())
	ctx := context.Background()
	adaLogin := env.SignIn(t, "ada@example.com", "Ada")
	ada, adaSession := adaLogin.User.ID, adaLogin.Session.ID
	benLogin := env.SignIn(t, "ben@example.com", "Ben")
	ben, benSession := benLogin.User.ID, benLogin.Session.ID
	p256dh, auth := deviceKeys(t)
	device := func(endpoint string) push.Input { return push.Input{Endpoint: endpoint, P256dh: p256dh, Auth: auth} }

	const attempts = push.MaxDevices + 15
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := env.Services.Push.Register(ctx, ben, benSession, device(fmt.Sprintf("https://push.example.com/%d", i)))
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	conflicts := 0
	for err := range results {
		switch {
		case errs.Is(err, errs.KindConflict):
			conflicts++
		case err != nil:
			t.Fatal(err)
		}
	}
	devices, err := env.Services.Push.List(ctx, ben)
	if err != nil || len(devices) != push.MaxDevices || conflicts != attempts-push.MaxDevices {
		t.Fatalf("want %d devices and %d conflicts, got %d and %d (%v)", push.MaxDevices, attempts-push.MaxDevices, len(devices), conflicts, err)
	}

	if _, _, err := env.Services.Push.Register(ctx, ada, adaSession, device("https://push.example.com/ada")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.Services.Push.Register(ctx, ben, benSession, device("https://push.example.com/ada")); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("moving a device past the cap should conflict, got %v", err)
	}
	if _, _, err := env.Services.Push.Register(ctx, ben, benSession, device(devices[0].Endpoint)); err != nil {
		t.Fatalf("refreshing a device at the cap: %v", err)
	}
}

// TestPushSubscriptionsStore checks the repository against PostgreSQL: the
// created flag, failure counting, success resetting it, and an endpoint
// moving to another account with its row.
func TestPushSubscriptionsStore(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	env, _ := pushSetup(t, now)
	ctx := context.Background()
	ada := env.SignIn(t, "ada@example.com", "Ada")
	ben := env.SignIn(t, "ben@example.com", "Ben")
	p256dh, keyAuth := deviceKeys(t)
	repo := repositories.NewPushSubscriptions(env.DB)
	sub := func(login *auth.Login) *push.Subscription {
		return &push.Subscription{ID: uuid.New(), UserID: login.User.ID, SessionID: login.Session.ID, Endpoint: "https://push.example.com/shared",
			P256dh: p256dh, Auth: keyAuth, Platform: push.PlatformWeb, CreatedAt: now, UpdatedAt: now}
	}

	first := sub(ada)
	created, err := repo.Save(ctx, first, push.MaxDevices)
	if err != nil || !created {
		t.Fatalf("first save: created=%v err=%v", created, err)
	}
	for want := 1; want <= 3; want++ {
		if n, err := repo.Failed(ctx, first.ID); err != nil || n != want {
			t.Fatalf("failure %d: got %d, %v", want, n, err)
		}
	}
	if err := repo.Succeeded(ctx, first.ID, now); err != nil {
		t.Fatal(err)
	}
	if n, err := repo.Failed(ctx, first.ID); err != nil || n != 1 {
		t.Fatalf("a success should reset the count: got %d, %v", n, err)
	}

	moved := sub(ben)
	created, err = repo.Save(ctx, moved, push.MaxDevices)
	if err != nil || created {
		t.Fatalf("saving a known endpoint should update it: created=%v err=%v", created, err)
	}
	if moved.ID != first.ID || moved.Failures != 1 || moved.LastSuccessAt == nil || !moved.LastSuccessAt.Equal(now) {
		t.Fatalf("the stored row should keep its id, failures and last success: %+v", moved)
	}
	if mine, _ := repo.List(ctx, ada.User.ID); len(mine) != 0 {
		t.Fatalf("the endpoint should have left its old account: %+v", mine)
	}
	if theirs, _ := repo.List(ctx, ben.User.ID); len(theirs) != 1 || theirs[0].SessionID != ben.Session.ID {
		t.Fatalf("the endpoint should belong to the new account and session: %+v", theirs)
	}

	if err := repo.Remove(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := repo.Failed(ctx, first.ID); err != nil || n != 0 {
		t.Fatalf("counting a failure for a removed device: got %d, %v", n, err)
	}
}
