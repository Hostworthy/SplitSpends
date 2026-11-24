// Package rates holds exchange-rate tables and exact currency conversion.
//
// Conversion only changes how totals are shown. Expenses, payments and
// balances stay in the currency they were entered in, so a rate change never
// rewrites anyone's history or what they owe.
package rates

import (
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
)

// Table is a snapshot of rates: Rates[code] units of that currency buy one
// unit of Base. The base currency is always present with rate 1.
type Table struct {
	Base      string
	Published time.Time
	Fetched   time.Time
	Source    string
	Rates     map[string]*big.Rat
}

// Amount is a value in minor units of one currency.
type Amount struct {
	Currency string
	Minor    int64
}

// New validates raw decimal rates (for example "0.912") and keeps the
// currencies SplitSpends supports. Invalid or non-positive values are
// dropped rather than failing the whole table.
func New(base string, raw map[string]string) (Table, bool) {
	b, ok := money.Lookup(base)
	if !ok {
		return Table{}, false
	}
	t := Table{Base: b.Code, Rates: map[string]*big.Rat{b.Code: big.NewRat(1, 1)}}
	for code, text := range raw {
		c, ok := money.Lookup(code)
		if !ok || c.Code == b.Code {
			continue
		}
		r, ok := new(big.Rat).SetString(strings.TrimSpace(text))
		if !ok || r.Sign() <= 0 {
			continue
		}
		t.Rates[c.Code] = r
	}
	return t, len(t.Rates) > 1
}

// Rate returns units of code per one Base.
func (t Table) Rate(code string) (*big.Rat, bool) {
	r, ok := t.Rates[strings.ToUpper(code)]
	return r, ok
}

// Has reports whether code has a rate.
func (t Table) Has(code string) bool {
	_, ok := t.Rate(code)
	return ok
}

// Codes lists the currencies in the table, sorted.
func (t Table) Codes() []string {
	out := make([]string, 0, len(t.Rates))
	for c := range t.Rates {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Convert returns a in minor units of to, rounded half away from zero. It
// reports false when either currency is unknown or has no rate.
func (t Table) Convert(a Amount, to string) (int64, bool) {
	v, ok := t.exact(a, to)
	if !ok {
		return 0, false
	}
	return round(v), true
}

// Total converts every amount to to and adds them exactly, rounding once at
// the end so the total does not drift with the number of currencies. Missing
// lists the currencies that could not be converted (and were left out).
func (t Table) Total(amounts []Amount, to string) (total int64, missing []string) {
	sum := new(big.Rat)
	seen := map[string]bool{}
	for _, a := range amounts {
		v, ok := t.exact(a, to)
		if !ok {
			code := strings.ToUpper(a.Currency)
			if !seen[code] {
				seen[code] = true
				missing = append(missing, code)
			}
			continue
		}
		sum.Add(sum, v)
	}
	sort.Strings(missing)
	return round(sum), missing
}

// exact is a converted to minor units of to, before rounding:
// minor × 10^(exp(to) − exp(from)) × rate(to) / rate(from).
func (t Table) exact(a Amount, to string) (*big.Rat, bool) {
	from, ok := money.Lookup(a.Currency)
	if !ok {
		return nil, false
	}
	target, ok := money.Lookup(to)
	if !ok {
		return nil, false
	}
	v := new(big.Rat).SetInt64(a.Minor)
	if from.Code == target.Code {
		return v, true
	}
	rf, ok := t.Rate(from.Code)
	if !ok {
		return nil, false
	}
	rt, ok := t.Rate(target.Code)
	if !ok {
		return nil, false
	}
	v.Mul(v, rt)
	v.Quo(v, rf)
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(target.Exponent-from.Exponent))), nil))
	if target.Exponent >= from.Exponent {
		v.Mul(v, scale)
	} else {
		v.Quo(v, scale)
	}
	return v, true
}

// Decimal renders r as a plain decimal without exponent, exactly when the
// denominator is a product of 2s and 5s (always true for parsed decimals),
// otherwise to 12 places.
func Decimal(r *big.Rat) string {
	places := 12
	d := new(big.Int).Set(r.Denom())
	for k := 0; k <= 40; k++ {
		if new(big.Int).Rem(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil), d).Sign() == 0 {
			places = k
			break
		}
	}
	return r.FloatString(places)
}

func round(v *big.Rat) int64 {
	num := new(big.Int).Abs(v.Num())
	den := v.Denom()
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if new(big.Int).Lsh(rem, 1).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if v.Sign() < 0 {
		q.Neg(q)
	}
	return q.Int64()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
