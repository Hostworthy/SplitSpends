package bootstrap

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/storage"
	"github.com/google/uuid"
)

type fakeNotifier struct {
	mu    sync.Mutex
	codes map[string]string
	sent  []ports.Notification
}

func (f *fakeNotifier) LoginCode(_ context.Context, to, code string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes[to] = code
	return nil
}
func (f *fakeNotifier) Invite(context.Context, string, string) error { return nil }
func (f *fakeNotifier) Notify(_ context.Context, n ports.Notification) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, n)
}

// setup needs TEST_DATABASE_URL pointing at a disposable database.
func setup(t *testing.T, now time.Time) (*Services, *fakeNotifier) {
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
	n := &fakeNotifier{codes: map[string]string{}}
	cfg := &Config{Secret: []byte("0123456789abcdef0123456789abcdef"), SignupMode: "open", BaseURL: "http://test"}
	return Wire(&database.DB{Pool: pool}, n, blobs, nil, clock.Fixed{At: now}, cfg), n
}

func TestRecurringAndNotifications(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	svc, notes := setup(t, now)

	if err := svc.Auth.RequestCode(ctx, "ada@example.com", ""); err != nil {
		t.Fatal(err)
	}
	login, err := svc.Auth.Verify(ctx, "ada@example.com", notes.codes["ada@example.com"], "Ada", "test")
	if err != nil {
		t.Fatal(err)
	}
	ada := login.User.ID
	name := "Flat"
	g, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}, Members: []group.Invitee{{Email: "ben@example.com", Name: "Ben"}}})
	if err != nil {
		t.Fatal(err)
	}
	var ben uuid.UUID
	for _, m := range g.Members {
		if m != ada {
			ben = m
		}
	}
	// Rent started on 1 January and repeats monthly: by 15 March the job must
	// have created the February and March instances.
	_, err = svc.Expenses.Create(ctx, ada, expense.Input{GroupID: &g.ID, Description: "Rent", Currency: "EUR", Total: 100000,
		Date: "2026-01-01", Method: expense.MethodEqual, Recurrence: expense.RecurMonthly,
		Payers: []expense.Payer{{UserID: ada, Amount: 100000}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := svc.Expenses.RunRecurring(ctx); err != nil {
			t.Fatal(err)
		}
	}
	list, total, err := svc.Expenses.List(ctx, expense.Filter{UserID: ada, GroupID: &g.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("want 3 rent instances, got %d", total)
	}
	scheduled := 0
	for _, e := range list {
		if e.Recurrence != expense.RecurNone {
			scheduled++
			if e.Date.Format(time.DateOnly) != "2026-03-01" || e.NextDate.Format(time.DateOnly) != "2026-04-01" {
				t.Fatalf("schedule should live on the newest copy: %s next %v", e.Date, e.NextDate)
			}
		}
	}
	if scheduled != 1 {
		t.Fatalf("exactly one expense should carry the schedule, got %d", scheduled)
	}

	sheet, err := svc.Groups.Balances(ctx, ada, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := sheet.Members[ada]["EUR"]; got != 150000 {
		t.Fatalf("Ada should be owed 1500.00, got %d", got)
	}
	// Ben never signed in, so he must not receive expense emails.
	for _, n := range notes.sent {
		if n.To == "ben@example.com" {
			t.Fatalf("invited user got a notification: %+v", n)
		}
	}
}
