package bootstrap_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/reminder"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/transfer"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// TestSocialEmailIsRateLimited checks the daily caps on email that adding
// people sends: "added you" emails per sender and recipient, invitations per
// inviter, and reminders per sender and recipient across groups.
func TestSocialEmailIsRateLimited(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	svc, notes := env.Services, env.Notifier
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	ben := env.SignIn(t, "ben@example.com", "Ben").User.ID
	notes.Reset()

	var groups []uuid.UUID
	for i := range 20 {
		name := fmt.Sprintf("Trip %d", i)
		v, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name},
			Members: []group.Invitee{{Email: "ben@example.com"}}})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		groups = append(groups, v.ID)
	}
	if got := notes.Subjects("ben@example.com"); len(got) != group.MaxAddedEmails {
		t.Fatalf("Ben should get %d emails about being added, got %d: %v", group.MaxAddedEmails, len(got), got)
	}

	// Invitations: a group that would go over the budget invites nobody,
	// and nothing is emailed for it.
	invitees := func(prefix string, n int) []group.Invitee {
		out := make([]group.Invitee, n)
		for i := range out {
			out[i] = group.Invitee{Email: fmt.Sprintf("%s%d@example.com", prefix, i)}
		}
		return out
	}
	name := "Too many"
	_, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}, Members: invitees("crowd", user.MaxInvites+1)})
	if !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("inviting more than %d people a day must be refused, got %v", user.MaxInvites, err)
	}
	if got := notes.Invites(); len(got) != 0 {
		t.Fatalf("a refused group must send no invitations, got %v", got)
	}
	for i, inv := range invitees("friend", user.MaxInvites) {
		if _, err := svc.Friends.Add(ctx, ada, inv.Email, ""); err != nil {
			t.Fatalf("friend %d: %v", i, err)
		}
	}
	if _, err := svc.Friends.Add(ctx, ada, "one-more@example.com", ""); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("invitation %d in a day must be refused, got %v", user.MaxInvites+1, err)
	}
	if _, err := svc.Friends.Add(ctx, ada, "ben@example.com", ""); err != nil {
		t.Fatalf("people who already have an account can still be added: %v", err)
	}
	sam, err := svc.Friends.Add(ctx, ada, "", "Sam")
	if err != nil {
		t.Fatalf("people without an email can still be added: %v", err)
	}
	if _, err := svc.Users.Invite(ctx, ada, sam.User.ID, "sam@example.com"); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("giving a person an email counts as an invitation, got %v", err)
	}
	if got := notes.Invites(); len(got) != user.MaxInvites || got[0] != "friend0@example.com" {
		t.Fatalf("want %d invitations, got %v", user.MaxInvites, got)
	}
	env.Clock.Add(user.InviteWindow)
	if _, err := svc.Users.Invite(ctx, ada, sam.User.ID, "sam@example.com"); err != nil {
		t.Fatalf("the budget refills after a day: %v", err)
	}

	// Reminders: one per group a day, but at most a few to the same person.
	notes.Reset()
	for i, g := range groups[:reminder.MaxPerPerson+1] {
		_, err := svc.Expenses.Create(ctx, ada, expense.Input{GroupID: &g, Description: "Dinner", Currency: "EUR", Total: 1000,
			Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ada, Amount: 1000}},
			Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.Reminders.Send(ctx, ada, ben, &g)
		if limited := errs.Is(err, errs.KindRateLimited); limited != (i == reminder.MaxPerPerson) {
			t.Fatalf("reminder %d: %v", i+1, err)
		}
	}
	reminders := 0
	for _, s := range notes.Subjects("ben@example.com") {
		if strings.Contains(s, "reminder") {
			reminders++
		}
	}
	if reminders != reminder.MaxPerPerson {
		t.Fatalf("Ben should get %d reminder emails, got %d", reminder.MaxPerPerson, reminders)
	}
	env.Clock.Add(reminder.Window + time.Minute)
	if _, err := svc.Reminders.Send(ctx, ada, ben, &groups[reminder.MaxPerPerson]); err != nil {
		t.Fatalf("reminders are allowed again the next day: %v", err)
	}
}

// TestCreateGroupMemberLimit checks that a group holds at most
// group.MaxMembers people, however they arrive, and that a group refused for
// its size creates and invites nobody.
func TestCreateGroupMemberLimit(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	cara := env.SignIn(t, "cara@example.com", "Cara").User.ID
	users := func() int {
		var n int
		if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := users()

	crowd := make([]group.Invitee, group.MaxMembers)
	for i := range crowd {
		crowd[i] = group.Invitee{Email: fmt.Sprintf("p%d@example.com", i)}
	}
	name := "Crowd"
	_, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}, Members: crowd})
	wantField(t, err, "members")
	if n := users(); n != before {
		t.Fatalf("a refused group must create nobody, got %d new people", n-before)
	}
	if got := env.Notifier.Invites(); len(got) != 0 {
		t.Fatalf("a refused group must invite nobody, got %d invitations", len(got))
	}

	for i := range crowd[:group.MaxMembers-1] {
		crowd[i] = group.Invitee{Name: fmt.Sprintf("Person %d", i)}
	}
	v, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}, Members: crowd[:group.MaxMembers-1]})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Members) != group.MaxMembers {
		t.Fatalf("want a full group of %d, got %d", group.MaxMembers, len(v.Members))
	}
	_, err = svc.Groups.AddMember(ctx, ada, v.ID, group.Invitee{Name: "One more"})
	wantField(t, err, "members")
	if _, err := svc.Groups.Join(ctx, cara, v.ID); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("joining a full group must be refused, got %v", err)
	}
}

// TestPlaceholderLimit checks that one account adds at most
// user.MaxPlaceholders people by name, that retrying an import of them still
// works at the limit, and that someone else is not affected.
func TestPlaceholderLimit(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	ben := env.SignIn(t, "ben@example.com", "Ben").User.ID

	bundle := transfer.Bundle{Me: uuid.New()}
	for i := range user.MaxPlaceholders {
		bundle.People = append(bundle.People, transfer.Person{ID: uuid.New(), Name: fmt.Sprintf("Person %d", i)})
	}
	for attempt := range 2 {
		if _, err := svc.Transfer.Import(ctx, ada, bundle); err != nil {
			t.Fatalf("import %d: %v", attempt, err)
		}
	}
	if _, err := svc.Friends.Add(ctx, ada, "", "One more"); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("person %d added by name must be refused, got %v", user.MaxPlaceholders+1, err)
	}
	name := "Trip"
	_, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}, Members: []group.Invitee{{Name: "Sam"}}})
	if !errs.Is(err, errs.KindConflict) {
		t.Fatalf("groups cannot add people by name past the limit either, got %v", err)
	}
	if _, err := svc.Friends.Add(ctx, ben, "", "Sam"); err != nil {
		t.Fatalf("the limit is per account: %v", err)
	}
}
