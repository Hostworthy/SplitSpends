package platform

import (
	"encoding/json"
	"testing"
)

func TestGroupPatchDefaultSplit(t *testing.T) {
	cases := []struct {
		body       string
		set, value bool
	}{
		{`{"name":"Flat"}`, false, false},
		{`{"defaultSplit":null}`, true, false},
		{`{"defaultSplit":{"method":"percent","values":{"a":"60","b":"40"}}}`, true, true},
	}
	for _, c := range cases {
		var p GroupPatch
		if err := json.Unmarshal([]byte(c.body), &p); err != nil {
			t.Fatalf("%s: %v", c.body, err)
		}
		if p.DefaultSplit.Set != c.set || (p.DefaultSplit.Value != nil) != c.value {
			t.Errorf("%s: got set=%v value=%v", c.body, p.DefaultSplit.Set, p.DefaultSplit.Value)
		}
	}

	var p GroupPatch
	_ = json.Unmarshal([]byte(cases[2].body), &p)
	if p.DefaultSplit.Value.Values["a"] != "60" {
		t.Fatalf("values not decoded: %+v", p.DefaultSplit.Value)
	}
	out, err := json.Marshal(GroupPatch{})
	if err != nil || string(out) != `{}` {
		t.Fatalf("unset field must be omitted, got %s %v", out, err)
	}
	out, _ = json.Marshal(GroupPatch{DefaultSplit: Nullable[DefaultSplit]{Set: true}})
	if string(out) != `{"defaultSplit":null}` {
		t.Fatalf("cleared field must be null, got %s", out)
	}
}
