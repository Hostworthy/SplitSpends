package group

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// TestDefaultSplitVectors runs the defaultSplits cases of spec/vectors.json,
// which the Dart ledger also runs, so both modes agree on default splits.
func TestDefaultSplitVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		DefaultSplits []struct {
			Name  string
			Split struct {
				Method SplitMethod
				Values map[uuid.UUID]string
			}
			Members   []uuid.UUID
			Error     string
			Message   string
			Current   []uuid.UUID
			Effective *bool
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.DefaultSplits) == 0 {
		t.Fatal("no defaultSplits vectors")
	}
	for _, c := range doc.DefaultSplits {
		d, err := DefaultSplit{Method: c.Split.Method, Values: c.Split.Values}.Validate(c.Members)
		if c.Error != "" {
			de, ok := errs.As(err)
			if !ok || de.Field != c.Error || de.Message != c.Message {
				t.Errorf("%s: want error on %q saying %q, got %v", c.Name, c.Error, c.Message, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		if c.Effective == nil {
			continue
		}
		g := &Group{DefaultSplit: d, Members: c.Current}
		if got := g.Effective() != nil; got != *c.Effective {
			t.Errorf("%s: effective %v, want %v", c.Name, got, *c.Effective)
		}
	}
}

func TestApplyDefaultSplit(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	now := time.Now()
	g, err := New("Flat", a, "EUR", now)
	if err != nil {
		t.Fatal(err)
	}
	g.Members = []uuid.UUID{a, b}

	rent := &DefaultSplit{Method: SplitPercent, Values: map[uuid.UUID]string{a: " 60 ", b: "40"}}
	if err := g.Apply(Patch{DefaultSplit: rent}, now); err != nil {
		t.Fatal(err)
	}
	if got := g.Effective(); got == nil || got.Values[a] != "60" || got.Values[b] != "40" {
		t.Fatalf("effective split = %+v", got)
	}

	bad := &DefaultSplit{Method: SplitPercent, Values: map[uuid.UUID]string{a: "60", b: "60"}}
	if err := g.Apply(Patch{DefaultSplit: bad}, now); err == nil {
		t.Fatal("want an error for 120%")
	}
	if g.DefaultSplit.Values[b] != "40" {
		t.Fatal("a rejected patch must keep the old default split")
	}

	g.Members = append(g.Members, uuid.New())
	if g.Effective() != nil {
		t.Fatal("a new member makes a percent default absent")
	}

	if err := g.Apply(Patch{DefaultSplit: &DefaultSplit{}}, now); err != nil {
		t.Fatal(err)
	}
	if g.DefaultSplit != nil {
		t.Fatal("an empty method clears the default split")
	}
}
