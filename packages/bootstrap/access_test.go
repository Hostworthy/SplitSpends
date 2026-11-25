package bootstrap_test

import (
	"context"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
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
	if list, err := svc.Expenses.Comments(ctx, ada, rent.ID); err != nil || len(list) != 1 {
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
