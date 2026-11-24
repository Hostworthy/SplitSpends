package bootstrap

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/storage"
	"github.com/google/uuid"
)

// movingClock lets a test step through time.
type movingClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *movingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *movingClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// setupInvite wires services on a fresh schema with SIGNUP_MODE=invite.
func setupInvite(t *testing.T, c *movingClock) (*Services, *fakeNotifier) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, url, 8)
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
	cfg := &Config{Secret: []byte("0123456789abcdef0123456789abcdef"), SignupMode: auth.SignupInvite, BaseURL: "http://test"}
	return Wire(&database.DB{Pool: pool}, n, blobs, nil, c, cfg), n
}

func signUp(t *testing.T, svc *Services, n *fakeNotifier, email, name, invite string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if err := svc.Auth.RequestCode(ctx, email, invite); err != nil {
		t.Fatal(err)
	}
	n.mu.Lock()
	code := n.codes[email]
	n.mu.Unlock()
	if code == "" {
		t.Fatalf("no code was sent to %s", email)
	}
	login, err := svc.Auth.Verify(ctx, email, code, name, "test")
	if err != nil {
		t.Fatal(err)
	}
	return login.User.ID
}

func (f *fakeNotifier) to(email string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var subjects []string
	for _, n := range f.sent {
		if n.To == email {
			subjects = append(subjects, n.Subject)
		}
	}
	return subjects
}

func hasKind(feed []*activity.Activity, kind activity.Kind) bool {
	for _, a := range feed {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

func TestRemindersAndInviteLinks(t *testing.T) {
	ctx := context.Background()
	c := &movingClock{at: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)}
	svc, notes := setupInvite(t, c)

	ada := signUp(t, svc, notes, "ada@example.com", "Ada", "")
	name := "Flat"
	v, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name},
		Members: []group.Invitee{{Email: "ben@example.com", Name: "Ben"}, {Name: "Sam"}}})
	if err != nil {
		t.Fatal(err)
	}
	var ben, sam uuid.UUID
	for _, u := range v.People {
		switch u.Name {
		case "Ben":
			ben = u.ID
		case "Sam":
			sam = u.ID
		}
	}
	_, err = svc.Expenses.Create(ctx, ada, expense.Input{GroupID: &v.ID, Description: "Groceries", Currency: "EUR", Total: 9000,
		Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ada, Amount: 9000}},
		Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}, {UserID: sam}}})
	if err != nil {
		t.Fatal(err)
	}

	// Ben is invited (never signed in) but has an email, so he can be reminded.
	r, err := svc.Reminders.Send(ctx, ada, ben, &v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Emailed || r.Owed["EUR"] != 3000 {
		t.Fatalf("unexpected reminder %+v", r)
	}
	if got := notes.to("ben@example.com"); len(got) != 1 || !strings.Contains(got[0], "Ada sent you a reminder") {
		t.Fatalf("Ben should get one reminder email, got %v", got)
	}
	feed, _, err := svc.Activity.List(ctx, activity.Filter{UserID: ben, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(feed, activity.ReminderSent) {
		t.Fatal("Ben should see the reminder in his feed")
	}
	if _, err := svc.Reminders.Send(ctx, ada, ben, &v.ID); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("second reminder in 24 hours must be limited, got %v", err)
	}
	if _, err := svc.Reminders.Send(ctx, ada, sam, &v.ID); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("placeholders cannot be reminded, got %v", err)
	}

	// The database enforces the window: concurrent overall reminders send once.
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Reminders.Send(ctx, ada, ben, nil)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	sent, limited := 0, 0
	for err := range results {
		switch {
		case err == nil:
			sent++
		case errs.Is(err, errs.KindRateLimited):
			limited++
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if sent != 1 || limited != 5 {
		t.Fatalf("want exactly one concurrent reminder, sent %d limited %d", sent, limited)
	}
	c.add(25 * time.Hour)
	if _, err := svc.Reminders.Send(ctx, ada, ben, &v.ID); err != nil {
		t.Fatalf("a day later reminding is allowed again: %v", err)
	}

	// Invite links: strangers cannot sign up without one.
	issued, err := svc.Invites.Create(ctx, ada, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if issued.URL != "http://test/join/"+issued.Token {
		t.Fatalf("unexpected url %s", issued.URL)
	}
	if err := svc.Auth.RequestCode(ctx, "cy@example.com", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Auth.RequestCode(ctx, "cy@example.com", "not-a-real-token"); err != nil {
		t.Fatal(err)
	}
	if notes.codes["cy@example.com"] != "" {
		t.Fatal("strangers without a valid link must not get a code")
	}
	cy := signUp(t, svc, notes, "cy@example.com", "Cy", issued.Token)
	p, err := svc.Invites.Preview(ctx, issued.Token)
	if err != nil || p.Group.Name != "Flat" || p.Inviter.Name != "Ada" || len(p.Group.Members) != 3 {
		t.Fatalf("preview: %+v %v", p, err)
	}
	for i := 0; i < 2; i++ {
		joined, err := svc.Invites.Accept(ctx, cy, issued.Token)
		if err != nil {
			t.Fatal(err)
		}
		if len(joined.Members) != 4 {
			t.Fatalf("accepting is idempotent: %v", joined.Members)
		}
	}
	feed, _, err = svc.Activity.List(ctx, activity.Filter{UserID: ada, GroupID: &v.ID, Limit: 50})
	if err != nil || !hasKind(feed, activity.GroupMemberJoined) {
		t.Fatalf("joining is recorded: %v", err)
	}
	list, err := svc.Invites.List(ctx, cy, v.ID)
	if err != nil || len(list) != 1 || list[0].TokenHash == issued.Token {
		t.Fatalf("list: %+v %v", list, err)
	}

	// Revoked and expired links look the same as unknown ones.
	second, err := svc.Invites.Create(ctx, cy, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Invites.Revoke(ctx, ada, v.ID, issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invites.Preview(ctx, issued.Token); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("revoked link: %v", err)
	}
	c.add(15 * 24 * time.Hour)
	if _, err := svc.Invites.Preview(ctx, second.Token); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("expired link: %v", err)
	}
	if err := svc.Invites.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, err := svc.Invites.Valid(ctx, second.Token); ok || err != nil {
		t.Fatalf("expired link must be invalid: %v", err)
	}
}
