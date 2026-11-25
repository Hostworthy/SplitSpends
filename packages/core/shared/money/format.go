package money

import (
	"strconv"
	"strings"
)

// Format renders minor units for humans, for example "€12.50".
func Format(amount int64, code string) string {
	c := known(code)
	sign, whole, fraction := parts(amount, c.Exponent)
	return sign + c.Symbol + join(group(whole), fraction)
}

// Plain renders minor units as a bare decimal for files and spreadsheets,
// for example "-1234.50": no symbol and no digit grouping. The client's
// Money.toPlain writes the same text for its on-device export.
func Plain(amount int64, code string) string {
	sign, whole, fraction := parts(amount, known(code).Exponent)
	return sign + join(whole, fraction)
}

// known is the currency for code, or a two-decimal stand-in named by its code
// when the catalog does not list it.
func known(code string) Currency {
	if c, ok := Lookup(code); ok {
		return c
	}
	return Currency{Code: code, Symbol: code + " ", Exponent: 2}
}

// parts splits amount into its sign, its whole units and exponent digits of
// fraction.
func parts(amount int64, exponent int) (sign, whole, fraction string) {
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	digits := strconv.FormatInt(amount, 10)
	if exponent <= 0 {
		return sign, digits, ""
	}
	for len(digits) <= exponent {
		digits = "0" + digits
	}
	cut := len(digits) - exponent
	return sign, digits[:cut], digits[cut:]
}

func join(whole, fraction string) string {
	if fraction == "" {
		return whole
	}
	return whole + "." + fraction
}

func group(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
