package bootstrap_test

import (
	"context"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// departed sets up the flat after Ben has settled and been removed: the rent
// Ada paid and shared with Ben stays in the group.
func departed(t *testing.T) (env *bootstraptest.Env, ada, ben uuid.UUID, g *group.View, rent *expense.Expense) {
	t.Helper()
	env, ada, ben, g = flat(t)
	rent = series(t, env, ada, ben, &g.ID, "Rent", "2026-03-01", expense.RecurNone)
	settle(t, env, ada, ben, &g.ID)
	if err := env.Services.Groups.RemoveMember(context.Background(), ada, g.ID, ben); err != nil {
		t.Fatal(err)
	}
	return env, ada, ben, g, rent
}

// settled fails the test unless everyone in the group, members or not, is
// settled.
func settled(t *testing.T, env *bootstraptest.Env, actor, groupID uuid.UUID) {
	t.Helper()
	sheet, err := env.Services.Groups.Balances(context.Background(), actor, groupID)
	if err != nil {
		t.Fatal(err)
	}
	for id, b := range sheet.Members {
		for currency, amount := range b {
			if amount != 0 {
				t.Fatalf("%s should be settled, has %d %s", id, amount, currency)
			}
		}
	}
	if len(sheet.Debts) != 0 {
		t.Fatalf("no debts should remain, got %+v", sheet.Debts)
	}
}

func TestFormerMemberCannotDeleteGroupExpense(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g, rent := departed(t)
	svc := env.Services.Expenses
	if err := svc.Delete(ctx, ben, rent.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("a former member must not delete the group's expenses, got %v", err)
	}
	if _, err := svc.Update(ctx, ben, rent.ID, expense.Input{Description: "Rent", Total: 100000, Method: expense.MethodEqual,
		Payers: []expense.Payer{{UserID: ada, Amount: 100000}}, Splits: []expense.SplitInput{{UserID: ada}}}); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("a former member must not edit the group's expenses, got %v", err)
	}
	got, err := svc.Get(ctx, ben, rent.ID)
	if err != nil {
		t.Fatalf("a former member still sees the expenses they were part of: %v", err)
	}
	if got.DeletedAt != nil || got.Total != 100000 {
		t.Fatalf("the expense changed: %+v", got)
	}
	settled(t, env, ada, g.ID)
}

func TestFormerMemberCannotRestoreGroupExpense(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	taxi := series(t, env, ada, ben, &g.ID, "Taxi", "2026-03-01", expense.RecurNone)
	if err := svc.Expenses.Delete(ctx, ada, taxi.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Groups.RemoveMember(ctx, ada, g.ID, ben); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Expenses.Restore(ctx, ben, taxi.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("a former member must not restore the group's expenses, got %v", err)
	}
}

func TestFormerMemberCannotComment(t *testing.T) {
	ctx := context.Background()
	env, _, ben, _, rent := departed(t)
	svc := env.Services.Expenses
	if _, err := svc.Comment(ctx, ben, rent.ID, "Still here"); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("a former member must not comment on the group's expenses, got %v", err)
	}
	list, _, err := svc.Comments(ctx, ben, rent.ID, 0, 0)
	if err != nil {
		t.Fatalf("a former member still reads the comments: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("the refused comment was saved: %+v", list)
	}
}

func TestCannotDeleteExpenseOfDepartedMember(t *testing.T) {
	ctx := context.Background()
	env, ada, _, g, rent := departed(t)
	svc := env.Services.Expenses
	if err := svc.Delete(ctx, ada, rent.ID); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("deleting an expense of someone who left must be refused, got %v", err)
	}
	got, err := svc.Get(ctx, ada, rent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeletedAt != nil {
		t.Fatal("the refused delete was saved")
	}
	settled(t, env, ada, g.ID)
}

func TestRemovedMemberCannotRejoinWithOwnLink(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	issued, err := svc.Invites.Create(ctx, ben, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Groups.RemoveMember(ctx, ada, g.ID, ben); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invites.Accept(ctx, ben, issued.Token); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("a removed member must not rejoin through their own link, got %v", err)
	}
	v, err := svc.Groups.View(ctx, ada, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.HasMember(ben) {
		t.Fatal("Ben is back in the group")
	}
}

// TestFailedCreateInvitesNobody checks invitations go out only once the
// group is saved: a create that fails on its second invitee neither keeps
// the first one nor emails them.
func TestFailedCreateInvitesNobody(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	env.Notifier.Reset()
	name := "Trip"
	_, err := env.Services.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name},
		Members: []group.Invitee{{Email: "new@example.com"}, {Email: "not an email"}}})
	if !errs.Is(err, errs.KindValidation) {
		t.Fatalf("want a validation error for the second invitee, got %v", err)
	}
	if got := env.Notifier.Invites(); len(got) != 0 {
		t.Fatalf("a group that was not saved must invite nobody, got %v", got)
	}
	var kept int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = 'new@example.com'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 0 {
		t.Fatal("the first invitee must not be kept when the group is not saved")
	}
}

// TestQuietAddMemberSendsNothing checks adding someone to a group honours a
// quiet context the way creating a group with them does.
func TestQuietAddMemberSendsNothing(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	env.SignIn(t, "ben@example.com", "Ben")
	name := "Trip"
	g, err := env.Services.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	env.Notifier.Reset()
	for _, email := range []string{"ben@example.com", "new@example.com"} {
		if _, err := env.Services.Groups.AddMember(ports.Quiet(ctx), ada, g.ID, group.Invitee{Email: email}); err != nil {
			t.Fatal(err)
		}
	}
	if sent, invites := env.Notifier.Sent(), env.Notifier.Invites(); len(sent) != 0 || len(invites) != 0 {
		t.Fatalf("a quiet add must send nothing, got %d notifications and invitations %v", len(sent), invites)
	}
}
