package mcp

import (
	"encoding/json"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func TestApplyDefault(t *testing.T) {
	cases := []struct {
		name       string
		preset     *platform.DefaultSplit
		wantMethod string
		want       []splitValue
	}{
		{"no default", nil, "", nil},
		{"equal falls back to every member", &platform.DefaultSplit{Method: "equal"}, "", nil},
		{"percent leaves out members at zero", &platform.DefaultSplit{Method: "percent", Values: map[string]string{"u3": "0.0000", "u2": "40.0", "u1": "60"}},
			"percent", []splitValue{{"u1", "60"}, {"u2", "40.0"}}},
		{"shares keep every value above zero", &platform.DefaultSplit{Method: "shares", Values: map[string]string{"u1": "1", "u2": "2.5", "u3": "0"}},
			"shares", []splitValue{{"u1", "1"}, {"u2", "2.5"}}},
		{"a value above zero that trims like zero", &platform.DefaultSplit{Method: "shares", Values: map[string]string{"u1": "0.0001", "u2": "10"}},
			"shares", []splitValue{{"u1", "0.0001"}, {"u2", "10"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method, values := applyDefault(tc.preset)
			if method != tc.wantMethod || !reflect.DeepEqual(values, tc.want) {
				t.Fatalf("got %q %v, want %q %v", method, values, tc.wantMethod, tc.want)
			}
		})
	}
}

// TestApplyDefaultVectors applies every valid default split of
// spec/vectors.json, which core and the Dart ledger also run: each member
// whose value is above zero takes part, and nobody else.
func TestApplyDefaultVectors(t *testing.T) {
	raw, err := os.ReadFile("../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		DefaultSplits []struct {
			Name  string
			Split platform.DefaultSplit
			Error string
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	applied := 0
	for _, c := range doc.DefaultSplits {
		if c.Error != "" {
			continue
		}
		applied++
		method, values := applyDefault(&c.Split)
		if c.Split.Method == "equal" {
			if method != "" || values != nil {
				t.Errorf("%s: an equal default must split between every member, got %q %v", c.Name, method, values)
			}
			continue
		}
		if method != c.Split.Method {
			t.Errorf("%s: method %q, want %q", c.Name, method, c.Split.Method)
		}
		got := map[string]string{}
		for _, v := range values {
			got[v.UserID] = v.Value
		}
		for id, v := range c.Split.Values {
			r, ok := new(big.Rat).SetString(v)
			if !ok {
				t.Fatalf("%s: %q is not a decimal", c.Name, v)
			}
			if _, in := got[id]; in != (r.Sign() != 0) {
				t.Errorf("%s: member %s with %q: taking part %v", c.Name, id, v, in)
			}
		}
	}
	if applied == 0 {
		t.Fatal("no valid defaultSplits vectors")
	}
}

func TestSplits(t *testing.T) {
	usd := unit{code: "USD", exponent: 2}
	members := []string{"u1", "u2", "u3"}
	number := func(s string) *json.Number {
		n := json.Number(s)
		return &n
	}
	rent := &platform.DefaultSplit{Method: "percent", Values: map[string]string{"u1": "60", "u2": "40", "u3": "0"}}
	cases := []struct {
		name       string
		in         addArgs
		members    []string
		preset     *platform.DefaultSplit
		wantMethod string
		want       []platform.Split
	}{
		{"equal outside a group adds the user once", addArgs{Participants: []string{"u2", "u1", "u2"}}, nil, nil,
			"equal", []platform.Split{{UserID: "u1"}, {UserID: "u2"}}},
		{"equal in a group uses every member", addArgs{GroupID: "g1"}, members, nil,
			"equal", []platform.Split{{UserID: "u1"}, {UserID: "u2"}, {UserID: "u3"}}},
		{"equal in a group with participants uses only them", addArgs{GroupID: "g1", SplitMethod: "Equal", Participants: []string{"u2", "u3"}}, members, rent,
			"equal", []platform.Split{{UserID: "u2"}, {UserID: "u3"}}},
		{"the group default applies when nothing is given", addArgs{GroupID: "g1"}, members, rent,
			"percent", []platform.Split{{UserID: "u1", Value: number("60")}, {UserID: "u2", Value: number("40")}}},
		{"exact values become minor units", addArgs{SplitMethod: "exact", SplitValues: []splitValue{{"u1", "10.25"}, {"u2", " 2.25 "}}}, nil, nil,
			"exact", []platform.Split{{UserID: "u1", Value: number("1025")}, {UserID: "u2", Value: number("225")}}},
		{"percent values stay as given", addArgs{SplitMethod: "percent", SplitValues: []splitValue{{"u1", " 33.5 "}, {"u2", "66.5"}}}, nil, nil,
			"percent", []platform.Split{{UserID: "u1", Value: number("33.5")}, {UserID: "u2", Value: number("66.5")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method, got, err := splits(tc.in, "u1", tc.members, tc.preset, usd)
			if err != nil {
				t.Fatal(err)
			}
			if method != tc.wantMethod || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q %+v, want %q %+v", method, got, tc.wantMethod, tc.want)
			}
		})
	}
}

func TestSplitsRefuses(t *testing.T) {
	usd := unit{code: "USD", exponent: 2}
	cases := []struct {
		name string
		in   addArgs
		want string
	}{
		{"unknown method", addArgs{SplitMethod: "ratio"}, "splitMethod must be"},
		{"value that is not a decimal", addArgs{SplitMethod: "shares", SplitValues: []splitValue{{"u1", "1/2"}}}, "not a decimal number"},
		{"exact value with too many decimals", addArgs{SplitMethod: "exact", SplitValues: []splitValue{{"u1", "1.005"}}}, "more decimals than USD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := splits(tc.in, "u1", nil, nil, usd)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error with %q, got %v", tc.want, err)
			}
		})
	}
}

func TestCurrency(t *testing.T) {
	me := platform.Me{User: platform.User{DefaultCurrency: "USD"}}
	flat := &platform.Group{DefaultCurrency: "JPY"}
	cases := []struct {
		given string
		group *platform.Group
		want  string
	}{
		{"kwd", flat, "KWD"},
		{"", flat, "JPY"},
		{"", &platform.Group{}, "USD"},
		{"", nil, "USD"},
	}
	for _, tc := range cases {
		if got := currency(tc.given, tc.group, me); got != tc.want {
			t.Errorf("currency(%q, %+v) = %q, want %q", tc.given, tc.group, got, tc.want)
		}
	}
}
