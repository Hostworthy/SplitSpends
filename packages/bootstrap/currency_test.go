package bootstrap_test

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/google/uuid"
)

// TestCurrencyComesFromGroupOrPerson checks that the app never needs to send
// a currency: group expenses take the group's, others the person's own.
func TestCurrencyComesFromGroupOrPerson(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{Now: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)})
	svc := env.Services
	login := env.SignIn(t, "ada@example.com", "Ada")
	ada := login.User.ID
	inr := "INR"
	if _, err := svc.Users.Update(ctx, ada, user.Patch{DefaultCurrency: &inr}); err != nil {
		t.Fatal(err)
	}
	name, eur := "Lisbon", "EUR"
	g, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name, DefaultCurrency: &eur},
		Members: []group.Invitee{{Email: "ben@example.com", Name: "Ben"}}})
	if err != nil {
		t.Fatal(err)
	}
	var ben uuid.UUID
	for _, m := range g.Members {
		if m != ada {
			ben = m
		}
	}
	add := func(groupID *uuid.UUID) *expense.Expense {
		t.Helper()
		e, err := svc.Expenses.Create(ctx, ada, expense.Input{GroupID: groupID, Description: "Lunch", Total: 2000,
			Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ada, Amount: 2000}},
			Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	if e := add(&g.ID); e.Currency != "EUR" {
		t.Fatalf("group expense currency = %s, want EUR", e.Currency)
	}
	if e := add(nil); e.Currency != "INR" {
		t.Fatalf("expense outside groups = %s, want INR", e.Currency)
	}
	p, err := svc.Expenses.RecordPayment(ctx, ada, expense.PaymentInput{GroupID: &g.ID, From: ben, To: ada, Amount: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if p.Currency != "EUR" {
		t.Fatalf("payment currency = %s, want EUR", p.Currency)
	}
}
