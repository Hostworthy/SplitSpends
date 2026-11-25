package group

import (
	"math/big"
	"strings"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// SplitMethod is how a default split divides new expenses. Only methods that
// do not depend on the expense total can be defaults.
type SplitMethod string

const (
	SplitEqual   SplitMethod = "equal"
	SplitPercent SplitMethod = "percent"
	SplitShares  SplitMethod = "shares"
)

// DefaultSplit is the split new expenses in a group start with. Clients apply
// it when someone adds an expense; it never changes saved expenses.
//
// Rules (mirrored in packages/ledger/lib/default_split.dart and run from
// spec/vectors.json by both):
//
//   - equal: Values is empty and everyone splits equally.
//   - percent and shares: Values has an entry for every current member and
//     for nobody else. Values are plain decimals with at most four decimal
//     places, never negative; zero means that member takes no part. Percent
//     values add up to exactly 100; shares add up to more than zero.
//   - A percent or shares default applies only while its members are exactly
//     the group's members. When someone joins or leaves, it is treated as
//     absent (Effective returns nil) until it is saved again, so a stale
//     default never silently produces a wrong split.
type DefaultSplit struct {
	Method SplitMethod
	Values map[uuid.UUID]string
}

const splitField = "defaultSplit"

// Validate checks d against the group's current members and returns a copy
// with trimmed values.
func (d DefaultSplit) Validate(members []uuid.UUID) (*DefaultSplit, error) {
	out := &DefaultSplit{Method: d.Method, Values: map[uuid.UUID]string{}}
	switch d.Method {
	case SplitEqual:
		if len(d.Values) > 0 {
			return nil, errs.Validation(splitField, "An equal default split takes no values.")
		}
		return out, nil
	case SplitPercent, SplitShares:
	default:
		return nil, errs.Validation(splitField, "A default split is equal, percent or shares.")
	}
	current := make(map[uuid.UUID]bool, len(members))
	for _, m := range members {
		current[m] = true
	}
	sum := new(big.Rat)
	for id, raw := range d.Values {
		if !current[id] {
			return nil, errs.Validation(splitField, "Only current members can be part of the default split.")
		}
		v := strings.TrimSpace(raw)
		r, ok := money.Decimal(v)
		if !ok {
			return nil, errs.Validation(splitField, "Use plain decimal numbers with at most four decimal places.")
		}
		if r.Sign() < 0 {
			return nil, errs.Validation(splitField, "Values cannot be negative.")
		}
		sum.Add(sum, r)
		out.Values[id] = v
	}
	if len(out.Values) != len(current) {
		return nil, errs.Validation(splitField, "Give every member a value (0 leaves them out).")
	}
	if d.Method == SplitPercent && sum.Cmp(big.NewRat(100, 1)) != 0 {
		return nil, errs.Validation(splitField, "Percentages must add up to 100.")
	}
	if sum.Sign() == 0 {
		return nil, errs.Validation(splitField, "At least one share must be above zero.")
	}
	return out, nil
}

// Matches reports whether d still fits the members: always for equal, and
// for percent and shares only when its members are exactly members.
func (d DefaultSplit) Matches(members []uuid.UUID) bool {
	if d.Method == SplitEqual {
		return true
	}
	if len(d.Values) != len(members) {
		return false
	}
	for _, m := range members {
		if _, ok := d.Values[m]; !ok {
			return false
		}
	}
	return true
}

// Effective returns the default split that applies to the current members,
// or nil when there is none or it no longer matches them.
func (g *Group) Effective() *DefaultSplit {
	if g.DefaultSplit == nil || !g.DefaultSplit.Matches(g.Members) {
		return nil
	}
	return g.DefaultSplit
}
