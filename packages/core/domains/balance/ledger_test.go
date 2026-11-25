package balance

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

var (
	a = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	b = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	c = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
	d = uuid.MustParse("00000000-0000-0000-0000-00000000000d")
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

// cuts splits total into n random non-negative parts.
func cuts(r *rand.Rand, total int64, n int) []int64 {
	parts := make([]int64, n)
	for range total {
		parts[r.IntN(n)]++
	}
	return parts
}

// randomLedger builds balanced entries with several payers each, so most of
// them leave rounding remainders between creditors.
func randomLedger(r *rand.Rand) []Entry {
	users := []uuid.UUID{a, b, c, d, uuid.MustParse("00000000-0000-0000-0000-00000000000e"), uuid.MustParse("00000000-0000-0000-0000-00000000000f")}
	var entries []Entry
	for range 200 {
		r.Shuffle(len(users), func(i, j int) { users[i], users[j] = users[j], users[i] })
		people := users[:2+r.IntN(len(users)-1)]
		total := 1 + r.Int64N(500)
		paid := cuts(r, total, 1+r.IntN(len(people)))
		owed := cuts(r, total, len(people))
		shares := make([]Share, len(people))
		for i, id := range people {
			shares[i] = Share{UserID: id, Owed: owed[i]}
			if i < len(paid) {
				shares[i].Paid = paid[i]
			}
		}
		entries = append(entries, Entry{Currency: []string{"USD", "EUR"}[r.IntN(2)], Shares: shares})
	}
	return entries
}

func TestPairwiseMatchesNets(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for round := range 50 {
		entries := randomLedger(r)
		debts := Pairwise(entries)
		nets := Nets(entries)
		for me, want := range nets {
			got := money.Balances{}
			for other := range nets {
				for cur, amount := range With(debts, me, other) {
					got.Add(cur, amount)
				}
			}
			for _, cur := range []string{"USD", "EUR"} {
				if got[cur] != want[cur] {
					t.Fatalf("round %d: %s %s from debts = %d, net = %d", round, me, cur, got[cur], want[cur])
				}
			}
		}
	}
}

func TestPairwiseKeepsEveryCreditorWhole(t *testing.T) {
	// a and b each fronted 2 and owe 1; c and d owe 1 each.
	entries := []Entry{{Currency: "USD", Shares: []Share{{a, 2, 1}, {b, 2, 1}, {c, 0, 1}, {d, 0, 1}}}}
	want := []Debt{{From: c, To: a, Currency: "USD", Amount: 1}, {From: d, To: b, Currency: "USD", Amount: 1}}
	if got := Pairwise(entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPairwiseIgnoresShareOrder(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	entries := randomLedger(r)
	want := Pairwise(entries)
	for range 20 {
		for _, e := range entries {
			r.Shuffle(len(e.Shares), func(i, j int) { e.Shares[i], e.Shares[j] = e.Shares[j], e.Shares[i] })
		}
		if got := Pairwise(entries); !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffled shares changed the debts:\n got %+v\nwant %+v", got, want)
		}
	}
}
