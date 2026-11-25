package expense

import (
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
)

func TestPaymentCategoryIsForPayments(t *testing.T) {
	in := base(MethodEqual, SplitInput{UserID: ada}, SplitInput{UserID: ben})
	in.Category = PaymentCategory
	_, err := Normalize(in, day)
	if de, ok := errs.As(err); !ok || de.Field != "category" {
		t.Fatalf("want an error on category, got %v", err)
	}
	n, err := payment(PaymentInput{From: ben, To: ada, Amount: 2500, Currency: "EUR"}, day)
	if err != nil {
		t.Fatal(err)
	}
	if n.Category != PaymentCategory || n.Description != "Payment" || len(n.Shares) != 2 {
		t.Fatalf("payment: %+v", n)
	}
}

func TestRepayment(t *testing.T) {
	two := func(in *Input) { in.Payers = append(in.Payers, Payer{ben, 0}) }
	cases := []struct {
		name  string
		edit  func(in *Input)
		field string
	}{
		{"one to one", func(*Input) {}, ""},
		{"two payers", two, "splits"},
		{"two recipients", func(in *Input) { in.Splits = append(in.Splits, SplitInput{UserID: cleo}) }, "splits"},
		{"part paid", func(in *Input) { in.Payers[0].Amount = 100 }, "payers"},
		{"paying oneself", func(in *Input) { in.Splits[0].UserID = ada }, "toUserId"},
	}
	for _, c := range cases {
		in := base(MethodExact, SplitInput{UserID: ben, Value: "10000"})
		in.Description, in.Category, in.Recurrence = "Rent", "rent", RecurMonthly
		c.edit(&in)
		p, err := repayment(in)
		var n *Normalized
		if err == nil {
			n, err = payment(p, day)
		}
		if c.field == "" {
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if n.Category != PaymentCategory || n.Description != "Payment" || n.Recurrence != RecurNone {
				t.Fatalf("%s: an edited payment must stay a payment: %+v", c.name, n.Input)
			}
			continue
		}
		if de, ok := errs.As(err); !ok || de.Field != c.field {
			t.Errorf("%s: want an error on %s, got %v", c.name, c.field, err)
		}
	}
}
