package money

import (
	"strconv"
	"strings"
)

// Format renders minor units for humans, for example "€12.50".
func Format(amount int64, code string) string {
	c, ok := Lookup(code)
	if !ok {
		c = Currency{Code: code, Symbol: code + " ", Exponent: 2}
	}
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	digits := strconv.FormatInt(amount, 10)
	if c.Exponent > 0 {
		for len(digits) <= c.Exponent {
			digits = "0" + digits
		}
		cut := len(digits) - c.Exponent
		digits = group(digits[:cut]) + "." + digits[cut:]
	} else {
		digits = group(digits)
	}
	return sign + c.Symbol + digits
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
