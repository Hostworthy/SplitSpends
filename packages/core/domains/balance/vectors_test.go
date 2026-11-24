package balance

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	type debt struct {
		From, To uuid.UUID
		Currency string
		Amount   int64
	}
	var doc struct {
		Balances struct {
			Entries    []Entry
			Nets       map[uuid.UUID]map[string]int64
			Pairwise   []debt
			Simplified []debt
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	b := doc.Balances
	for id, want := range b.Nets {
		for cur, amount := range want {
			if got := Nets(b.Entries)[id][cur]; got != amount {
				t.Errorf("net %s %s = %d, want %d", id, cur, got, amount)
			}
		}
	}
	conv := func(ds []Debt) []debt {
		out := []debt{}
		for _, d := range ds {
			out = append(out, debt(d))
		}
		return out
	}
	if got := conv(Pairwise(b.Entries)); !reflect.DeepEqual(got, b.Pairwise) {
		t.Errorf("pairwise:\n got %+v\nwant %+v", got, b.Pairwise)
	}
	if got := conv(Simplify(Nets(b.Entries))); !reflect.DeepEqual(got, b.Simplified) {
		t.Errorf("simplified:\n got %+v\nwant %+v", got, b.Simplified)
	}
}
