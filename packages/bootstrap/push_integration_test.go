package bootstrap_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/webpush"
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

// pushSetup wires services with push turned on and delivered before Notify
// returns.
func pushSetup(t *testing.T, now time.Time) (*bootstraptest.Env, *fakeSender) {
	t.Helper()
	cfg := bootstraptest.Defaults()
	cfg.Serverless, cfg.VAPIDPublicKey = true, "test-key"
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
	ben := env.SignIn(t, "ben@example.com", "Ben").User.ID
	p256dh, auth := deviceKeys(t)

	phone := push.Input{Endpoint: "https://ntfy.example.com/upAbc?up=1", P256dh: p256dh, Auth: auth, Platform: "unifiedpush", Label: "Pixel"}
	first, created, err := svc.Push.Register(ctx, ben, phone)
	if err != nil || !created {
		t.Fatalf("register: created=%v err=%v", created, err)
	}
	again, created, err := svc.Push.Register(ctx, ben, phone)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("re-register must be idempotent: created=%v id=%v err=%v", created, again.ID, err)
	}
	if _, _, err := svc.Push.Register(ctx, ben, push.Input{Endpoint: "https://push.example.com/gone", P256dh: p256dh, Auth: auth}); err != nil {
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
