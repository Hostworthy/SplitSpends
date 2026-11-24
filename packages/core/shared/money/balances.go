package money

import "sort"

// Balance is an amount in a single currency.
type Balance struct {
	Currency string
	Amount   int64
}

// Balances accumulates amounts per currency.
type Balances map[string]int64

// Add adds amount in currency.
func (b Balances) Add(currency string, amount int64) {
	b[currency] += amount
}

// Merge adds every entry of other into b.
func (b Balances) Merge(other Balances) {
	for c, a := range other {
		b[c] += a
	}
}

// Negate returns a copy with every amount negated.
func (b Balances) Negate() Balances {
	out := make(Balances, len(b))
	for c, a := range b {
		out[c] = -a
	}
	return out
}

// List returns non-zero balances sorted by currency.
func (b Balances) List() []Balance {
	out := make([]Balance, 0, len(b))
	for c, a := range b {
		if a != 0 {
			out = append(out, Balance{Currency: c, Amount: a})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}

// Settled reports whether every balance is zero.
func (b Balances) Settled() bool {
	for _, a := range b {
		if a != 0 {
			return false
		}
	}
	return true
}
