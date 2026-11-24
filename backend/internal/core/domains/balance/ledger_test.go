package balance

import (
	"testing"

	"github.com/Hostworthy/SplitSpends/backend/internal/core/shared/money"
	"github.com/google/uuid"
)

var (
	a = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	b = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	c = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
)

// a pays 30 split by three; b pays 30 split by three; c pays nothing.
var ledger = []Entry{
	{Currency: "USD", Shares: []Share{{a, 3000, 1000}, {b, 0, 1000}, {c, 0, 1000}}},
	{Currency: "USD", Shares: []Share{{a, 0, 1000}, {b, 3000, 1000}, {c, 0, 1000}}},
}

func TestNets(t *testing.T) {
	n := Nets(ledger)
	if n[a]["USD"] != 1000 || n[b]["USD"] != 1000 || n[c]["USD"] != -2000 {
		t.Fatalf("got %v", n)
	}
}

func TestPairwiseCancelsOppositeDebts(t *testing.T) {
	debts := Pairwise(ledger)
	// a and b owe each other 10 in opposite directions, which cancels.
	want := map[[2]uuid.UUID]int64{{c, a}: 1000, {c, b}: 1000}
	if len(debts) != len(want) {
		t.Fatalf("got %+v", debts)
	}
	for _, d := range debts {
		if want[[2]uuid.UUID{d.From, d.To}] != d.Amount {
			t.Fatalf("unexpected debt %+v", d)
		}
	}
}

func TestSimplifyMinimizesTransfers(t *testing.T) {
	// a owes b 10, b owes c 10: simplified to a owes c 10.
	entries := []Entry{
		{Currency: "EUR", Shares: []Share{{b, 1000, 0}, {a, 0, 1000}}},
		{Currency: "EUR", Shares: []Share{{c, 1000, 0}, {b, 0, 1000}}},
	}
	if got := Pairwise(entries); len(got) != 2 {
		t.Fatalf("pairwise should keep two debts, got %+v", got)
	}
	got := Simplify(Nets(entries))
	if len(got) != 1 || got[0].From != a || got[0].To != c || got[0].Amount != 1000 {
		t.Fatalf("got %+v", got)
	}
}

func TestSimplifyKeepsCurrenciesApart(t *testing.T) {
	nets := map[uuid.UUID]money.Balances{a: {"USD": 500, "EUR": -300}, b: {"USD": -500, "EUR": 300}}
	got := Simplify(nets)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestWith(t *testing.T) {
	got := With(Pairwise(ledger), a, c)
	if got["USD"] != 1000 {
		t.Fatalf("got %v", got)
	}
}
