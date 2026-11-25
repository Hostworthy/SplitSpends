package bootstrap_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

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
	cfg := bootstraptest.Defaults()
	cfg.SignupMode = auth.SignupInvite
	env := bootstraptest.Open(t, bootstraptest.Options{Config: cfg, Now: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)})
	svc, c := env.Services, env.Clock

	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
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
	if got := env.Notifier.Subjects("ben@example.com"); len(got) != 1 || !strings.Contains(got[0], "Ada sent you a reminder") {
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
	c.Add(25 * time.Hour)
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
	if err := svc.Auth.RequestCode(ctx, "cy@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Auth.RequestCode(ctx, "cy@example.com", "not-a-real-token", ""); err != nil {
		t.Fatal(err)
	}
	if env.Notifier.Code("cy@example.com") != "" {
		t.Fatal("strangers without a valid link must not get a code")
	}
	cy := env.Join(t, "cy@example.com", "Cy", issued.Token).User.ID
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
	c.Add(15 * 24 * time.Hour)
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

// TestRemoveCoMemberFriendConflicts checks that removing someone who is a
// friend only because of a shared group says so instead of reporting a
// removal that did not happen.
func TestRemoveCoMemberFriendConflicts(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, _ := flat(t)
	svc := env.Services
	if err := svc.Friends.Remove(ctx, ada, ben); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("removing a co-member must be a conflict, got %v", err)
	}
	if ok, err := svc.Friends.Connected(ctx, ada, ben); err != nil || !ok {
		t.Fatalf("Ada and Ben still share the flat: %v %v", ok, err)
	}

	cy := env.SignIn(t, "cy@example.com", "Cy").User.ID
	if _, err := svc.Friends.Add(ctx, ada, "cy@example.com", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Friends.Remove(ctx, ada, cy); err != nil {
		t.Fatalf("removing an explicit friend works: %v", err)
	}
	if ok, err := svc.Friends.Connected(ctx, ada, cy); err != nil || ok {
		t.Fatalf("Ada and Cy are no longer connected: %v %v", ok, err)
	}
}
