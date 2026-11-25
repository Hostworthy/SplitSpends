package money

import (
	"math/big"
	"strings"
	"testing"
)

func TestDecimal(t *testing.T) {
	ok := map[string]*big.Rat{
		"60":      big.NewRat(60, 1),
		"-3.5":    big.NewRat(-7, 2),
		"+5":      big.NewRat(5, 1),
		"1.":      big.NewRat(1, 1),
		".25":     big.NewRat(1, 4),
		"33.3333": big.NewRat(333333, 10000),
		strings.Repeat("9", DecimalLength): new(big.Rat).SetFrac(
			new(big.Int).Sub(new(big.Int).Exp(big.NewInt(10), big.NewInt(DecimalLength), nil), big.NewInt(1)), big.NewInt(1)),
	}
	for in, want := range ok {
		got, valid := Decimal(in)
		if !valid || got.Cmp(want) != 0 {
			t.Errorf("Decimal(%q) = %v, %v; want %v", in, got, valid, want)
		}
	}
	for _, in := range []string{
		"", " 5", "0x10", "0X1F", "0b11", "0o17", "1_000", "1.5_0", "0x1p4", "0x.8",
		"1e3", "1E3", "1/2", ".", "+", "1.23456", "\ufeff5", strings.Repeat("9", DecimalLength+1),
	} {
		if r, valid := Decimal(in); valid {
			t.Errorf("Decimal(%q) = %v, want it rejected", in, r)
		}
	}
}
