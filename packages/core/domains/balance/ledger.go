// Package balance turns the expense ledger into balances and settle-up debts.
// The calculations are pure functions; Service adds the reads they need.
package balance

import (
	"bytes"
	"math/big"
	"slices"
	"sort"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Share is one participant's position in a ledger entry.
type Share struct {
	UserID uuid.UUID
	Paid   int64
	Owed   int64
}

// Entry is a non-deleted expense or payment reduced to what balances need.
type Entry struct {
	GroupID  *uuid.UUID
	Currency string
	Shares   []Share
}

// Debt says From owes To Amount in Currency.
type Debt struct {
	From     uuid.UUID
	To       uuid.UUID
	Currency string
	Amount   int64
}

// Nets returns each user's paid minus owed per currency. Positive means the
// user is owed money.
func Nets(entries []Entry) map[uuid.UUID]money.Balances {
	out := map[uuid.UUID]money.Balances{}
	for _, e := range entries {
		for _, s := range e.Shares {
			b, ok := out[s.UserID]
			if !ok {
				b = money.Balances{}
				out[s.UserID] = b
			}
			b.Add(e.Currency, s.Paid-s.Owed)
		}
	}
	return out
}

type pairKey struct {
	a, b     uuid.UUID
	currency string
}

// Pairwise returns who owes whom without simplification: every expense's
// debtors owe its creditors in proportion to what each creditor fronted, and
// opposite debts between the same two people cancel out. Within an expense
// each debtor's debts add up to what they owe and each creditor is owed
// exactly what they fronted, so the debts always agree with Nets.
func Pairwise(entries []Entry) []Debt {
	pairs := map[pairKey]int64{}
	for _, e := range entries {
		shares := slices.Clone(e.Shares)
		slices.SortFunc(shares, func(x, y Share) int { return bytes.Compare(x.UserID[:], y.UserID[:]) })
		var creditors, debtors []uuid.UUID
		var credits, debts []int64
		for _, s := range shares {
			switch net := s.Paid - s.Owed; {
			case net > 0:
				creditors = append(creditors, s.UserID)
				credits = append(credits, net)
			case net < 0:
				debtors = append(debtors, s.UserID)
				debts = append(debts, -net)
			}
		}
		if len(creditors) == 0 {
			continue
		}
		for i, row := range spread(debts, credits) {
			for j, amount := range row {
				addPair(pairs, debtors[i], creditors[j], e.Currency, amount)
			}
		}
	}
	return pairDebts(pairs)
}

// spread splits debts among creditors in proportion to credits so that row i
// adds up to debts[i] and column j to credits[j]; an expense's debts and
// credits are equal in total. Every cell starts at the floor of its exact
// share. The units left over then number fewer than the creditors in each
// row and fewer than the debtors in each column, and they are handed out
// from the first row and column onwards, which fills both totals exactly.
// Callers pass debtors and creditors in id order so the result is the same
// whatever order the shares were read in.
func spread(debts, credits []int64) [][]int64 {
	total := new(big.Int)
	for _, c := range credits {
		total.Add(total, big.NewInt(c))
	}
	cells := make([][]int64, len(debts))
	rows := slices.Clone(debts)
	cols := slices.Clone(credits)
	for i, d := range debts {
		cells[i] = make([]int64, len(credits))
		for j, c := range credits {
			exact := new(big.Int).Mul(big.NewInt(d), big.NewInt(c))
			cells[i][j] = exact.Quo(exact, total).Int64()
			rows[i] -= cells[i][j]
			cols[j] -= cells[i][j]
		}
	}
	for i, j := 0, 0; i < len(rows) && j < len(cols); {
		n := min(rows[i], cols[j])
		cells[i][j] += n
		rows[i] -= n
		cols[j] -= n
		if rows[i] == 0 {
			i++
		}
		if cols[j] == 0 {
			j++
		}
	}
	return cells
}

// addPair records that from owes to amount.
func addPair(pairs map[pairKey]int64, from, to uuid.UUID, currency string, amount int64) {
	if amount == 0 || from == to {
		return
	}
	if before(from, to) {
		pairs[pairKey{from, to, currency}] += amount
	} else {
		pairs[pairKey{to, from, currency}] -= amount
	}
}

func pairDebts(pairs map[pairKey]int64) []Debt {
	out := make([]Debt, 0, len(pairs))
	for k, amount := range pairs {
		switch {
		case amount > 0:
			out = append(out, Debt{From: k.a, To: k.b, Currency: k.currency, Amount: amount})
		case amount < 0:
			out = append(out, Debt{From: k.b, To: k.a, Currency: k.currency, Amount: -amount})
		}
	}
	sortDebts(out)
	return out
}

// Simplify returns the fewest practical transfers that settle nets: per
// currency, the largest debtor repeatedly pays the largest creditor. Each
// transfer zeroes at least one person, so there are at most n-1 transfers.
func Simplify(nets map[uuid.UUID]money.Balances) []Debt {
	type position struct {
		id     uuid.UUID
		amount int64
	}
	byCurrency := map[string][]position{}
	for id, b := range nets {
		for c, a := range b {
			if a != 0 {
				byCurrency[c] = append(byCurrency[c], position{id, a})
			}
		}
	}
	var out []Debt
	for currency, list := range byCurrency {
		var creditors, debtors []position
		for _, p := range list {
			if p.amount > 0 {
				creditors = append(creditors, p)
			} else {
				debtors = append(debtors, position{p.id, -p.amount})
			}
		}
		order := func(ps []position) {
			sort.Slice(ps, func(i, j int) bool {
				if ps[i].amount != ps[j].amount {
					return ps[i].amount > ps[j].amount
				}
				return before(ps[i].id, ps[j].id)
			})
		}
		for len(creditors) > 0 && len(debtors) > 0 {
			order(creditors)
			order(debtors)
			c, d := &creditors[0], &debtors[0]
			amount := min(c.amount, d.amount)
			out = append(out, Debt{From: d.id, To: c.id, Currency: currency, Amount: amount})
			c.amount -= amount
			d.amount -= amount
			if c.amount == 0 {
				creditors = creditors[1:]
			}
			if d.amount == 0 {
				debtors = debtors[1:]
			}
		}
	}
	sortDebts(out)
	return out
}

// Debts returns simplified or pairwise debts for entries.
func Debts(entries []Entry, simplify bool) []Debt {
	if simplify {
		return Simplify(Nets(entries))
	}
	return Pairwise(entries)
}

// With returns, from the point of view of me, the balance with other implied
// by debts: positive means other owes me.
func With(debts []Debt, me, other uuid.UUID) money.Balances {
	out := money.Balances{}
	for _, d := range debts {
		switch {
		case d.From == other && d.To == me:
			out.Add(d.Currency, d.Amount)
		case d.From == me && d.To == other:
			out.Add(d.Currency, -d.Amount)
		}
	}
	return out
}

func sortDebts(ds []Debt) {
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].Currency != ds[j].Currency {
			return ds[i].Currency < ds[j].Currency
		}
		if ds[i].Amount != ds[j].Amount {
			return ds[i].Amount > ds[j].Amount
		}
		if ds[i].From != ds[j].From {
			return before(ds[i].From, ds[j].From)
		}
		return before(ds[i].To, ds[j].To)
	})
}

// before orders ids without formatting them; byte order equals the order of
// the lowercase hex strings, which is how the Dart ledger sorts.
func before(a, b uuid.UUID) bool {
	return bytes.Compare(a[:], b[:]) < 0
}
