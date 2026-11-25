package bootstrap_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/category"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/transfer"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// wantKind fails the test unless err is a domain error of kind.
func wantKind(t *testing.T, what string, err error, kind errs.Kind) {
	t.Helper()
	if !errs.Is(err, kind) {
		t.Errorf("%s: want %s, got %v", what, kind, err)
	}
}

// TestGroupOperationsRequireMembership checks that someone outside a group
// cannot read or change it through any group operation, and that each
// refusal looks like a group that does not exist. The export route is
// covered by routes.TestOutsidersSeeNothing.
func TestGroupOperationsRequireMembership(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID
	rename := "Mal's"

	_, err := svc.Groups.View(ctx, mal, g.ID)
	wantKind(t, "View", err, errs.KindNotFound)
	_, err = svc.Groups.Update(ctx, mal, g.ID, group.Patch{Name: &rename})
	wantKind(t, "Update", err, errs.KindNotFound)
	wantKind(t, "Delete", svc.Groups.Delete(ctx, mal, g.ID), errs.KindNotFound)
	_, err = svc.Groups.AddMember(ctx, mal, g.ID, group.Invitee{Name: "Eve"})
	wantKind(t, "AddMember", err, errs.KindNotFound)
	wantKind(t, "RemoveMember", svc.Groups.RemoveMember(ctx, mal, g.ID, ben), errs.KindNotFound)
	_, err = svc.Groups.Balances(ctx, mal, g.ID)
	wantKind(t, "Balances", err, errs.KindNotFound)
	_, _, err = svc.Expenses.List(ctx, expense.Filter{UserID: mal, GroupID: &g.ID, Limit: 10})
	wantKind(t, "Expenses.List", err, errs.KindNotFound)
	_, err = svc.Invites.Create(ctx, mal, g.ID)
	wantKind(t, "Invites.Create", err, errs.KindNotFound)
	_, err = svc.Invites.List(ctx, mal, g.ID)
	wantKind(t, "Invites.List", err, errs.KindNotFound)

	v, err := svc.Groups.View(ctx, ada, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "Flat" || len(v.Members) != 2 {
		t.Fatalf("an outsider changed the group: %q with %d members", v.Name, len(v.Members))
	}
}

// TestCannotRemoveOrDeleteUnsettled checks that nobody leaves, is removed
// from or deletes a group while a balance in it is open.
func TestCannotRemoveOrDeleteUnsettled(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services.Groups
	series(t, env, ada, ben, &g.ID, "Rent", "2026-03-01", expense.RecurNone)

	wantKind(t, "removing Ben, who owes", svc.RemoveMember(ctx, ada, g.ID, ben), errs.KindConflict)
	wantKind(t, "Ben leaving while he owes", svc.RemoveMember(ctx, ben, g.ID, ben), errs.KindConflict)
	wantKind(t, "Ada leaving while she is owed", svc.RemoveMember(ctx, ada, g.ID, ada), errs.KindConflict)
	wantKind(t, "deleting the group", svc.Delete(ctx, ada, g.ID), errs.KindConflict)
	if v, err := svc.View(ctx, ada, g.ID); err != nil || len(v.Members) != 2 {
		t.Fatalf("the group and both members must stay: %v", err)
	}

	settle(t, env, ada, ben, &g.ID)
	if err := svc.RemoveMember(ctx, ada, g.ID, ben); err != nil {
		t.Fatalf("a settled member can be removed: %v", err)
	}
}

// TestCreateGroupWithForeignIdConflicts checks that a client-chosen id that
// belongs to someone else's group is refused and changes nothing.
func TestCreateGroupWithForeignIdConflicts(t *testing.T) {
	ctx := context.Background()
	env, ada, _, g := flat(t)
	svc := env.Services.Groups
	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID
	name := "Mine now"
	id := g.ID
	_, err := svc.Create(ctx, mal, group.CreateInput{ID: &id, Patch: group.Patch{Name: &name}})
	wantKind(t, "Create with a foreign id", err, errs.KindConflict)
	if mine, err := svc.Mine(ctx, mal); err != nil || len(mine) != 0 {
		t.Fatalf("Mal must not join or get a group: %d %v", len(mine), err)
	}
	if v, err := svc.View(ctx, ada, g.ID); err != nil || v.Name != "Flat" || v.HasMember(mal) {
		t.Fatalf("the group must stay as it was: %v", err)
	}
}

// TestFeedAfterRemoval checks that a removed member sees nothing recorded in
// the group after they left, and that a group's feed is empty for someone
// who never joined it.
func TestFeedAfterRemoval(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g, _ := departed(t)
	svc := env.Services
	env.Clock.Add(time.Hour)
	left := env.Clock.Now()
	rename := "New flat"
	if _, err := svc.Groups.Update(ctx, ada, g.ID, group.Patch{Name: &rename}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Groups.AddMember(ctx, ada, g.ID, group.Invitee{Name: "Cy"}); err != nil {
		t.Fatal(err)
	}

	events, _, err := svc.Activity.List(ctx, activity.Filter{UserID: ben, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("Ben keeps the events from while he was a member")
	}
	for _, a := range events {
		if !a.CreatedAt.Before(left) {
			t.Errorf("Ben sees %s, recorded after he left", a.Kind)
		}
	}

	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID
	if list, total, err := svc.Activity.List(ctx, activity.Filter{UserID: mal, GroupID: &g.ID, Limit: 100}); err != nil || total != 0 || len(list) != 0 {
		t.Fatalf("an outsider must see nothing of the group's feed: %d %v", total, err)
	}
}

// TestFriendDetailRequiresConnection checks that balances with someone are
// shown only to people connected with them, or still owing them.
func TestFriendDetailRequiresConnection(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, _, _ := departed(t)
	svc := env.Services.Friends
	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID

	_, err := svc.Detail(ctx, ada, mal)
	wantKind(t, "Detail of a stranger", err, errs.KindNotFound)
	_, err = svc.Detail(ctx, ada, ben)
	wantKind(t, "Detail of a settled former co-member", err, errs.KindNotFound)
	_, err = svc.Detail(ctx, ada, uuid.New())
	wantKind(t, "Detail of an unknown id", err, errs.KindNotFound)
}

// TestFriendRemoveNeedsSettled checks that a friendship ends only once
// everything between the two is settled.
func TestFriendRemoveNeedsSettled(t *testing.T) {
	ctx := context.Background()
	env, ada, _, _ := flat(t)
	svc := env.Services
	cy := env.SignIn(t, "cy@example.com", "Cy").User.ID
	if _, err := svc.Friends.Add(ctx, ada, "cy@example.com", ""); err != nil {
		t.Fatal(err)
	}
	concert := owes(ada, cy, ada)
	concert.Date = "2026-03-10"
	if _, err := svc.Expenses.Create(ctx, ada, concert); err != nil {
		t.Fatal(err)
	}

	wantKind(t, "removing a friend who owes", svc.Friends.Remove(ctx, ada, cy), errs.KindConflict)
	wantKind(t, "the friend removing", svc.Friends.Remove(ctx, cy, ada), errs.KindConflict)
	if ok, err := svc.Friends.Connected(ctx, ada, cy); err != nil || !ok {
		t.Fatalf("Ada and Cy must stay friends: %v %v", ok, err)
	}

	_, err := svc.Expenses.RecordPayment(ctx, cy, expense.PaymentInput{From: cy, To: ada, Amount: 10000, Currency: "EUR", Date: "2026-03-11"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Friends.Remove(ctx, ada, cy); err != nil {
		t.Fatalf("a settled friend can be removed: %v", err)
	}
}

// wantKindField fails the test unless err is a domain error of kind on
// field.
func wantKindField(t *testing.T, err error, kind errs.Kind, field string) {
	t.Helper()
	de, ok := errs.As(err)
	if !ok || de.Kind != kind || de.Field != field {
		t.Fatalf("want %s on %s, got %v", kind, field, err)
	}
}

// TestImportForeignGroupId checks that an import cannot take over someone
// else's group by naming its id, and writes nothing.
func TestImportForeignGroupId(t *testing.T) {
	ctx := context.Background()
	env, _, _, g := flat(t)
	svc := env.Services
	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID
	me, sam := uuid.New(), uuid.New()
	_, err := svc.Transfer.Import(ctx, mal, transfer.Bundle{Me: me, People: []transfer.Person{{ID: sam, Name: "Sam"}},
		Groups: []transfer.Group{{ID: g.ID, Members: []uuid.UUID{me, sam}, Patch: group.Patch{Name: str("Flat")}}}})
	wantKindField(t, err, errs.KindConflict, "groups[0]")
	if mine, err := svc.Groups.Mine(ctx, mal); err != nil || len(mine) != 0 {
		t.Fatalf("Mal must not join the group: %d %v", len(mine), err)
	}
	if friends, err := svc.Friends.List(ctx, mal); err != nil || len(friends) != 0 {
		t.Fatalf("a refused import must add nobody: %d %v", len(friends), err)
	}
}

// TestImportForeignPlaceholderId checks that an import cannot claim a
// person someone else added by name by reusing their id.
func TestImportForeignPlaceholderId(t *testing.T) {
	ctx := context.Background()
	env, ada, _, _ := flat(t)
	svc := env.Services
	sam, err := svc.Friends.Add(ctx, ada, "", "Sam")
	if err != nil {
		t.Fatal(err)
	}
	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID
	_, err = svc.Transfer.Import(ctx, mal, transfer.Bundle{Me: uuid.New(), People: []transfer.Person{{ID: sam.User.ID, Name: "Sam"}}})
	wantKindField(t, err, errs.KindConflict, "people[0]")
	if friends, err := svc.Friends.List(ctx, mal); err != nil || len(friends) != 0 {
		t.Fatalf("Mal must not befriend Ada's placeholder: %d %v", len(friends), err)
	}
}

// TestImportIsAtomic checks that an import whose last expense is invalid
// leaves no people, categories or groups behind.
func TestImportIsAtomic(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	svc := env.Services
	cara := env.SignIn(t, "cara@example.com", "Cara").User.ID
	me, dan, trip := uuid.New(), uuid.New(), uuid.New()
	plants := "custom." + uuid.NewString()
	good := expense.Input{GroupID: &trip, Description: "Dinner", Currency: "EUR", Total: 1000, Date: "2026-04-01", Method: expense.MethodEqual,
		Payers: []expense.Payer{{UserID: me, Amount: 1000}}, Splits: []expense.SplitInput{{UserID: me}, {UserID: dan}}}
	bad := good
	bad.Total = 0
	bad.Payers = []expense.Payer{{UserID: me, Amount: 0}}
	_, err := svc.Transfer.Import(ctx, cara, transfer.Bundle{Me: me,
		People:     []transfer.Person{{ID: dan, Name: "Dan"}},
		Categories: []transfer.Category{{Input: category.Input{ID: &plants, Name: str("Plants"), Icon: str("park")}}},
		Groups:     []transfer.Group{{ID: trip, Members: []uuid.UUID{me, dan}, Patch: group.Patch{Name: str("Trip")}}},
		Expenses: []transfer.Expense{{Kind: expense.KindExpense, Input: good},
			{Kind: expense.KindExpense, Input: bad}}})
	de, ok := errs.As(err)
	if !ok || de.Kind != errs.KindValidation || !strings.HasPrefix(de.Field, "expenses[1]") {
		t.Fatalf("want a validation error on expenses[1], got %v", err)
	}

	if mine, err := svc.Groups.Mine(ctx, cara); err != nil || len(mine) != 0 {
		t.Fatalf("the group must be rolled back: %d %v", len(mine), err)
	}
	if friends, err := svc.Friends.List(ctx, cara); err != nil || len(friends) != 0 {
		t.Fatalf("the people must be rolled back: %d %v", len(friends), err)
	}
	list, err := svc.Categories.List(ctx, cara)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.ID == plants {
			t.Fatal("the category must be rolled back")
		}
	}
	if _, total, err := svc.Expenses.List(ctx, expense.Filter{UserID: cara, Limit: 10}); err != nil || total != 0 {
		t.Fatalf("the first expense must be rolled back: %d %v", total, err)
	}
}

// TestImportLimits checks that an import over a limit is refused before
// anything is written.
func TestImportLimits(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	cara := env.SignIn(t, "cara@example.com", "Cara").User.ID
	for name, bundle := range map[string]transfer.Bundle{
		"people":     {People: make([]transfer.Person, transfer.MaxPeople+1)},
		"groups":     {Groups: make([]transfer.Group, transfer.MaxGroups+1)},
		"expenses":   {Expenses: make([]transfer.Expense, transfer.MaxExpenses+1)},
		"categories": {Categories: make([]transfer.Category, transfer.MaxCategories+1)},
	} {
		bundle.Me = uuid.New()
		// The empty items would fail validation on their own, so the error
		// must be the limit's, which names no item.
		_, err := env.Services.Transfer.Import(ctx, cara, bundle)
		if de, ok := errs.As(err); !ok || de.Kind != errs.KindValidation || de.Field != "" || !strings.Contains(de.Message, "at a time") {
			t.Errorf("too many %s: want the import limit, got %v", name, err)
		}
	}
}
