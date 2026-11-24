package bootstrap_test

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/google/uuid"
)

func TestRecurringAndNotifications(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	env := bootstraptest.Open(t, bootstraptest.Options{Now: now})
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
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
	for _, n := range env.Notifier.Sent() {
		if n.To == "ben@example.com" {
			t.Fatalf("invited user got a notification: %+v", n)
		}
	}
}
