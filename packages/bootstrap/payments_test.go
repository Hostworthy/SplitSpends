package bootstrap_test

import (
	"context"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/transfer"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

func TestPaymentStaysPayment(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services.Expenses
	p, err := svc.RecordPayment(ctx, ada, expense.PaymentInput{GroupID: &g.ID, From: ben, To: ada, Amount: 500, Currency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	edit := func(payers []expense.Payer, splits []expense.SplitInput) expense.Input {
		return expense.Input{Description: "Gift", Currency: "EUR", Total: 800, Method: expense.MethodEqual, Category: "food",
			Recurrence: expense.RecurMonthly, Payers: payers, Splits: splits}
	}
	two := edit([]expense.Payer{{UserID: ben, Amount: 400}, {UserID: ada, Amount: 400}}, []expense.SplitInput{{UserID: ada, Value: "800"}})
	if _, err := svc.Update(ctx, ada, p.ID, two); !errs.Is(err, errs.KindValidation) {
		t.Fatalf("a payment with two payers must be refused, got %v", err)
	}
	got, err := svc.Update(ctx, ada, p.ID, edit([]expense.Payer{{UserID: ben, Amount: 800}}, []expense.SplitInput{{UserID: ada, Value: "800"}}))
	if err != nil {
		t.Fatalf("editing a payment's amount must work: %v", err)
	}
	if got.Kind != expense.KindPayment || got.Total != 800 || got.Description != "Payment" || got.Category != expense.PaymentCategory ||
		got.Recurrence != expense.RecurNone || got.Payer() != ben || got.Recipient() != ada || len(got.Shares) != 2 {
		t.Fatalf("an edited payment must stay one person paying another: %+v", got)
	}
}

func TestPaymentCategoryIsForPayments(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	in := expense.Input{GroupID: &g.ID, Description: "Dinner", Currency: "EUR", Total: 3000, Method: expense.MethodEqual,
		Category: expense.PaymentCategory, Payers: []expense.Payer{{UserID: ada, Amount: 3000}},
		Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}}
	_, err := env.Services.Expenses.Create(ctx, ada, in)
	if de, ok := errs.As(err); !ok || de.Kind != errs.KindValidation || de.Field != "category" {
		t.Fatalf("want a validation error on category, got %v", err)
	}

	me := uuid.New()
	local := in
	local.GroupID = nil
	local.Payers, local.Splits = []expense.Payer{{UserID: me, Amount: 3000}}, []expense.SplitInput{{UserID: me}}
	if _, err := env.Services.Transfer.Import(ctx, ada, transfer.Bundle{Me: me,
		Expenses: []transfer.Expense{{Kind: expense.KindExpense, Input: local}}}); err != nil {
		t.Fatalf("an import of an expense filed under payments must still work: %v", err)
	}
	list, _, err := env.Services.Expenses.List(ctx, expense.Filter{UserID: ada, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Category != expense.DefaultCategory {
		t.Fatalf("the imported expense must use the default category: %+v", list)
	}
}
