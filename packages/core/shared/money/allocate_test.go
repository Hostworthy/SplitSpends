package money

import (
	"errors"
	"math/big"
	"testing"
)

func sum(parts []int64) int64 {
	var s int64
	for _, p := range parts {
		s += p
	}
	return s
}

func TestEvenSumsToTotal(t *testing.T) {
	for _, tc := range []struct {
		total int64
		n     int
	}{{100, 3}, {1, 3}, {9999, 7}, {-100, 3}, {0, 4}} {
		parts := Even(tc.total, tc.n)
		if sum(parts) != tc.total {
			t.Fatalf("Even(%d,%d) = %v, sum %d", tc.total, tc.n, parts, sum(parts))
		}
	}
	if got := Even(100, 3); got[0] != 34 || got[1] != 33 || got[2] != 33 {
		t.Fatalf("Even(100,3) = %v, want [34 33 33]", got)
	}
}

func TestAllocateProportional(t *testing.T) {
	parts, err := Allocate(1000, []*big.Rat{big.NewRat(1, 1), big.NewRat(2, 1), big.NewRat(2, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if parts[0] != 200 || parts[1] != 400 || parts[2] != 400 {
		t.Fatalf("got %v", parts)
	}
}

func TestAllocateLargestRemainder(t *testing.T) {
	// 33.33% / 33.33% / 33.34% of 1000 = 333.3, 333.3, 333.4
	parts, _ := Allocate(1000, []*big.Rat{big.NewRat(3333, 1), big.NewRat(3333, 1), big.NewRat(3334, 1)})
	if sum(parts) != 1000 || parts[2] != 334 {
		t.Fatalf("got %v", parts)
	}
}

func TestAllocateZeroWeights(t *testing.T) {
	if _, err := Allocate(10, []*big.Rat{new(big.Rat)}); !errors.Is(err, ErrNoWeights) {
		t.Fatalf("want ErrNoWeights, got %v", err)
	}
}
