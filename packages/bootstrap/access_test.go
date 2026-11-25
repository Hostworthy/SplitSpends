package bootstrap_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// TestOthersObjectsAreNotFound checks that someone who may not see an object
// gets the same answer as for an id that does not exist.
func TestOthersObjectsAreNotFound(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID
	rent := series(t, env, ada, ben, &g.ID, "Rent", "2026-03-01", expense.RecurNone)
	note, err := svc.Expenses.Comment(ctx, ada, rent.ID, "Paid on time")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Expenses.DeleteComment(ctx, mal, note.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("a comment on an expense Mal cannot see must be not found, got %v", err)
	}
	if err := svc.Expenses.DeleteComment(ctx, ben, note.ID); !errs.Is(err, errs.KindForbidden) {
		t.Fatalf("Ben sees Ada's comment but may not delete it, got %v", err)
	}
	if list, _, err := svc.Expenses.Comments(ctx, ada, rent.ID, 0, 0); err != nil || len(list) != 1 {
		t.Fatalf("the comment must stay: %v %v", list, err)
	}

	sam, err := svc.Friends.Add(ctx, ada, "", "Sam")
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]uuid.UUID{"Ada's placeholder": sam.User.ID, "an account": ben, "an unknown id": uuid.New()} {
		if _, err := svc.Users.Invite(ctx, mal, id, "sam@example.com"); !errs.Is(err, errs.KindNotFound) {
			t.Errorf("inviting %s must be not found, got %v", name, err)
		}
	}
	if _, err := svc.Users.Invite(ctx, ada, sam.User.ID, "sam@example.com"); err != nil {
		t.Fatalf("Ada gives her own placeholder an email: %v", err)
	}
	if _, err := svc.Users.Invite(ctx, ada, sam.User.ID, "sam2@example.com"); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("someone Ada added who already has an email is a conflict, got %v", err)
	}

	_, unknown := svc.Expenses.RecordPayment(ctx, mal, expense.PaymentInput{From: mal, To: uuid.New(), Amount: 100,
		Currency: "EUR", Date: "2026-03-15"})
	_, stranger := svc.Expenses.RecordPayment(ctx, mal, expense.PaymentInput{From: mal, To: ada, Amount: 100,
		Currency: "EUR", Date: "2026-03-15"})
	if unknown == nil || stranger == nil || unknown.Error() != stranger.Error() {
		t.Fatalf("paying an unknown id and a stranger must look the same: %q and %q", unknown, stranger)
	}
}

// TestGroupExpenseRoles checks who may read and change a group expense: current
// members may do everything, people who left keep reading the expenses they
// were part of, and nobody else sees them. Listing goes through the
// repository's own query, so this also checks that it agrees with Get.
func TestGroupExpenseRoles(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{Now: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)})
	svc := env.Services
	id := map[string]uuid.UUID{}
	for _, name := range []string{"Ada", "Ben", "Cy", "Dee", "Eve", "Mal"} {
		id[name] = env.SignIn(t, strings.ToLower(name)+"@example.com", name).User.ID
	}
	ada := id["Ada"]
	title := "Flat"
	g, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &title}, Members: []group.Invitee{
		{Email: "ben@example.com", Name: "Ben"}, {Email: "cy@example.com", Name: "Cy"},
		{Email: "dee@example.com", Name: "Dee"}, {Email: "eve@example.com", Name: "Eve"}}})
	if err != nil {
		t.Fatal(err)
	}
	// Ben was part of the rent and left once settled; Dee left without ever
	// being part of an expense; Eve shares the groceries with Ada.
	rent := series(t, env, ada, id["Ben"], &g.ID, "Rent", "2026-03-01", expense.RecurNone)
	settle(t, env, ada, id["Ben"], &g.ID)
	groceries := series(t, env, ada, id["Eve"], &g.ID, "Groceries", "2026-03-02", expense.RecurNone)
	for _, who := range []string{"Ben", "Dee"} {
		if err := svc.Groups.RemoveMember(ctx, ada, g.ID, id[who]); err != nil {
			t.Fatal(err)
		}
	}

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	edit := func(actor uuid.UUID, e *expense.Expense, receipt *uuid.UUID) error {
		splits := make([]expense.SplitInput, 0, len(e.Shares))
		for _, sh := range e.Shares {
			splits = append(splits, expense.SplitInput{UserID: sh.UserID})
		}
		_, err := svc.Expenses.Update(ctx, actor, e.ID, expense.Input{Description: e.Description + " (edited)", Total: e.Total,
			Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ada, Amount: e.Total}}, Splits: splits, ReceiptID: receipt})
		return err
	}
	ops := []struct {
		name string
		run  func(actor uuid.UUID, e *expense.Expense) error
	}{
		{"get", func(actor uuid.UUID, e *expense.Expense) error {
			_, err := svc.Expenses.Get(ctx, actor, e.ID)
			return err
		}},
		{"list", func(actor uuid.UUID, e *expense.Expense) error {
			list, _, err := svc.Expenses.List(ctx, expense.Filter{UserID: actor, Limit: 50})
			if err != nil {
				return err
			}
			for _, x := range list {
				if x.ID == e.ID {
					return nil
				}
			}
			return errs.NotFound("Expense")
		}},
		{"comment", func(actor uuid.UUID, e *expense.Expense) error {
			_, err := svc.Expenses.Comment(ctx, actor, e.ID, "Noted")
			return err
		}},
		{"update", func(actor uuid.UUID, e *expense.Expense) error { return edit(actor, e, nil) }},
		{"attach receipt", func(actor uuid.UUID, e *expense.Expense) error {
			r, err := svc.Receipts.Upload(ctx, actor, png)
			if err != nil {
				return err
			}
			return edit(actor, e, &r.ID)
		}},
		{"delete", func(actor uuid.UUID, e *expense.Expense) error { return svc.Expenses.Delete(ctx, actor, e.ID) }},
		{"restore", func(actor uuid.UUID, e *expense.Expense) error {
			_, err := svc.Expenses.Restore(ctx, actor, e.ID)
			return err
		}},
	}
	// Roles run in this order so that the writes that succeed come last and
	// leave the expense as the others found it.
	roles := []struct {
		name    string
		actor   uuid.UUID
		subject *expense.Expense
		allowed map[string]bool
	}{
		{"outsider", id["Mal"], groceries, map[string]bool{}},
		{"former member without a share", id["Dee"], groceries, map[string]bool{}},
		{"former member with a share", id["Ben"], rent, map[string]bool{"get": true, "list": true}},
		{"current member without a share", id["Cy"], groceries, map[string]bool{"get": true, "list": true, "comment": true,
			"update": true, "attach receipt": true, "delete": true, "restore": true}},
	}
	for _, r := range roles {
		for _, op := range ops {
			err := op.run(r.actor, r.subject)
			if r.allowed[op.name] && err != nil {
				t.Errorf("%s: %s must be allowed, got %v", r.name, op.name, err)
			}
			if !r.allowed[op.name] && !errs.Is(err, errs.KindNotFound) {
				t.Errorf("%s: %s must be not found, got %v", r.name, op.name, err)
			}
		}
	}
}
