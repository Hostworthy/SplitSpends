package money

import (
	"math/big"
	"regexp"
)

// DecimalLength is the longest split value accepted: room for a sign, the
// 13 digits of the largest expense total, a point and four decimals. It keeps
// parsing cheap and stored values human-sized.
const DecimalLength = 20

// DecimalPattern matches a plain decimal number: digits with an optional
// sign and point, and no exponent, fraction, base prefix or separator.
var DecimalPattern = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)$`)

// Decimal parses a split value: a plain decimal such as "60", "-3.5", "1."
// or ".25" with at most DecimalLength characters and four decimal places.
// Exponents, fractions, base prefixes and digit separators are rejected, as
// they are by the Dart ledger. The caller trims s first.
func Decimal(s string) (*big.Rat, bool) {
	if len(s) > DecimalLength || !DecimalPattern.MatchString(s) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, false
	}
	if !new(big.Rat).Mul(r, big.NewRat(10000, 1)).IsInt() {
		return nil, false
	}
	return r, true
}
