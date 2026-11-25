package bootstrap_test

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// owes is a non-group expense of 100.00 that payer paid, owed in full by
// debtor, booked by someone with a zero share.
func owes(payer, debtor, booker uuid.UUID) expense.Input {
	return expense.Input{Description: "Concert", Currency: "EUR", Total: 10000, Method: expense.MethodExact,
		Payers: []expense.Payer{{UserID: payer, Amount: 10000}},
		Splits: []expense.SplitInput{{UserID: debtor, Value: "10000"}, {UserID: booker, Value: "0"}}}
}

func TestNonGroupDebtsNeedConnectedPeople(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{Now: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)})
	svc := env.Services
	xia := env.SignIn(t, "xia@example.com", "Xia").User.ID
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	ben := env.SignIn(t, "ben@example.com", "Ben").User.ID
	for _, email := range []string{"ada@example.com", "ben@example.com"} {
		if _, err := svc.Friends.Add(ctx, xia, email, ""); err != nil {
			t.Fatal(err)
		}
	}

	_, err := svc.Expenses.Create(ctx, xia, owes(ada, ben, xia))
	if de, ok := errs.As(err); !ok || de.Kind != errs.KindValidation || de.Field != "splits" {
		t.Fatalf("a debt between two people who are not connected must be refused on splits, got %v", err)
	}
	if _, n, err := svc.Expenses.List(ctx, expense.Filter{UserID: ben, Limit: 10}); err != nil || n != 0 {
		t.Fatalf("the refused expense was saved: %d expenses, %v", n, err)
	}

	if _, err := svc.Expenses.Create(ctx, xia, expense.Input{Description: "Taxi", Currency: "EUR", Total: 9000,
		Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: xia, Amount: 9000}},
		Splits: []expense.SplitInput{{UserID: xia}, {UserID: ada}, {UserID: ben}}}); err != nil {
		t.Fatalf("paying for two friends who do not know each other must work: %v", err)
	}

	if _, err := svc.Friends.Add(ctx, ada, "ben@example.com", ""); err != nil {
		t.Fatal(err)
	}
	monthly := owes(ada, ben, xia)
	monthly.Date, monthly.Recurrence = "2026-03-01", expense.RecurMonthly
	if _, err := svc.Expenses.Create(ctx, xia, monthly); err != nil {
		t.Fatalf("once Ada and Ben are connected the expense is allowed: %v", err)
	}

	// The series stops once the two people it leaves owing each other are
	// no longer connected, even though both are still Xia's friends.
	if _, err := svc.Expenses.RecordPayment(ctx, ben, expense.PaymentInput{From: ben, To: ada, Amount: 10000,
		Currency: "EUR", Date: "2026-03-15"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Friends.Remove(ctx, ada, ben); err != nil {
		t.Fatal(err)
	}
	env.Clock.Add(30 * 24 * time.Hour)
	if n := drain(t, svc.Expenses); n != 0 {
		t.Fatalf("the job booked %d copies between people who are no longer connected", n)
	}
}
