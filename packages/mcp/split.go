package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// currency is the currency of a new entry: the one the assistant gave, else
// the group's, else the user's.
func currency(given string, g *platform.Group, me platform.Me) string {
	if c := strings.ToUpper(given); c != "" {
		return c
	}
	if g != nil && g.DefaultCurrency != "" {
		return g.DefaultCurrency
	}
	return me.DefaultCurrency
}

// applyDefault returns the method and values a group's default split gives
// an expense the assistant did not split itself. An equal default returns
// no method, like no default at all: the expense is then split equally
// between every member. A member whose value is zero takes no part, so
// gets no value.
func applyDefault(preset *platform.DefaultSplit) (string, []splitValue) {
	if preset == nil || preset.Method == "equal" {
		return "", nil
	}
	var values []splitValue
	for _, id := range sortedKeys(preset.Values) {
		v := preset.Values[id]
		// The API validated these as plain decimals; anything else is passed
		// on so that the API reports it.
		if r, ok := new(big.Rat).SetString(strings.TrimSpace(v)); ok && r.Sign() == 0 {
			continue
		}
		values = append(values, splitValue{UserID: id, Value: v})
	}
	return preset.Method, values
}

// splits returns the split method and the splits of a new expense. me is
// the user, members are the group's members (none outside a group), preset
// is the group's default split and cur the expense's currency.
func splits(in addArgs, me string, members []string, preset *platform.DefaultSplit, cur unit) (string, []platform.Split, error) {
	method := strings.ToLower(in.SplitMethod)
	values := in.SplitValues
	if method == "" && len(in.Participants) == 0 && len(values) == 0 {
		// The group's default split (for example rent 60/40) applies when
		// the assistant gives no split of its own.
		if m, v := applyDefault(preset); m != "" {
			method, values = m, v
		}
	}
	var out []platform.Split
	switch method {
	case "", "equal":
		ids := in.Participants
		if len(ids) == 0 {
			ids = members
		}
		if in.GroupID == "" {
			ids = append([]string{me}, ids...)
		}
		for _, id := range unique(ids) {
			out = append(out, platform.Split{UserID: id})
		}
		return "equal", out, nil
	case "exact", "percent", "shares":
		for _, v := range values {
			value := strings.TrimSpace(v.Value)
			if !plain(value) {
				return "", nil, fmt.Errorf("the split value %q for %s is not a decimal number such as 60 or 33.5", v.Value, v.UserID)
			}
			if method == "exact" {
				m, err := cur.minor(v.Value)
				if err != nil {
					return "", nil, err
				}
				value = strconv.FormatInt(m, 10)
			}
			n := json.Number(value)
			out = append(out, platform.Split{UserID: v.UserID, Value: &n})
		}
		return method, out, nil
	default:
		return "", nil, errors.New("splitMethod must be equal, exact, percent or shares")
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unique(list []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
