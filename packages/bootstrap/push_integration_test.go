package bootstrap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/storage"
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

func pushSetup(t *testing.T, now time.Time) (*Services, *fakeNotifier, *fakeSender, *database.DB) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, url, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	blobs, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db := &database.DB{Pool: pool}
	notes := &fakeNotifier{codes: map[string]string{}}
	sender := &fakeSender{sent: map[string][]push.Message{}}
	cfg := &Config{Secret: []byte("0123456789abcdef0123456789abcdef"), SignupMode: "open", BaseURL: "http://test",
		AppName: "SplitSpends", Serverless: true, VAPIDPublicKey: "test-key"}
	svc := WireWithPush(db, notes, blobs, nil, sender, clock.Fixed{At: now}, cfg, slog.New(slog.DiscardHandler))
	return svc, notes, sender, db
}

func signIn(t *testing.T, svc *Services, notes *fakeNotifier, email, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if err := svc.Auth.RequestCode(ctx, email); err != nil {
		t.Fatal(err)
	}
	login, err := svc.Auth.Verify(ctx, email, notes.codes[email], name, "test")
	if err != nil {
		t.Fatal(err)
	}
	return login.User.ID
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
	svc, notes, sender, _ := pushSetup(t, now)
	ada := signIn(t, svc, notes, "ada@example.com", "Ada")
	ben := signIn(t, svc, notes, "ben@example.com", "Ben")
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
	for _, n := range notes.sent {
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
	_, _, _, db := pushSetup(t, time.Now())
	ctx := context.Background()
	keys := repositories.NewVAPIDKeys(db)
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
