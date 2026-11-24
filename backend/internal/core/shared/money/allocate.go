package money

import (
	"errors"
	"math/big"
	"sort"
)

// ErrNoWeights is returned when every weight is zero.
var ErrNoWeights = errors.New("money: weights must not all be zero")

// Allocate splits total into len(weights) parts proportional to weights using
// the largest remainder method. The parts always sum exactly to total. Ties in
// remainders go to the earlier index, so results are deterministic.
// Weights must be non-negative; total may be negative.
func Allocate(total int64, weights []*big.Rat) ([]int64, error) {
	sum := new(big.Rat)
	for _, w := range weights {
		if w.Sign() < 0 {
			return nil, errors.New("money: negative weight")
		}
		sum.Add(sum, w)
	}
	if sum.Sign() == 0 {
		return nil, ErrNoWeights
	}

	negative := total < 0
	abs := total
	if negative {
		abs = -total
	}

	parts := make([]int64, len(weights))
	type rem struct {
		index int
		frac  *big.Rat
	}
	rems := make([]rem, 0, len(weights))
	var assigned int64
	for i, w := range weights {
		exact := new(big.Rat).Mul(big.NewRat(abs, 1), w)
		exact.Quo(exact, sum)
		floor := new(big.Int).Quo(exact.Num(), exact.Denom())
		parts[i] = floor.Int64()
		assigned += parts[i]
		frac := new(big.Rat).Sub(exact, new(big.Rat).SetInt(floor))
		rems = append(rems, rem{i, frac})
	}

	sort.SliceStable(rems, func(a, b int) bool { return rems[a].frac.Cmp(rems[b].frac) > 0 })
	for left, k := abs-assigned, 0; left > 0; left, k = left-1, k+1 {
		parts[rems[k%len(rems)].index]++
	}

	if negative {
		for i := range parts {
			parts[i] = -parts[i]
		}
	}
	return parts, nil
}

// Even splits total into n equal parts that sum to total.
func Even(total int64, n int) []int64 {
	weights := make([]*big.Rat, n)
	for i := range weights {
		weights[i] = big.NewRat(1, 1)
	}
	parts, _ := Allocate(total, weights)
	return parts
}
