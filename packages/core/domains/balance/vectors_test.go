package balance

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
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
		Balances []struct {
			Name       string
			Entries    []Entry
			Nets       map[uuid.UUID]money.Balances
			Pairwise   []debt
			Simplified []debt
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	conv := func(ds []Debt) []debt {
		out := []debt{}
		for _, d := range ds {
			out = append(out, debt(d))
		}
		return out
	}
	if len(doc.Balances) == 0 {
		t.Fatal("no balance vectors")
	}
	for _, b := range doc.Balances {
		if got, want := prune(Nets(b.Entries)), prune(b.Nets); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: nets:\n got %v\nwant %v", b.Name, got, want)
		}
		if got := conv(Pairwise(b.Entries)); !reflect.DeepEqual(got, b.Pairwise) {
			t.Errorf("%s: pairwise:\n got %+v\nwant %+v", b.Name, got, b.Pairwise)
		}
		if got := conv(Simplify(Nets(b.Entries))); !reflect.DeepEqual(got, b.Simplified) {
			t.Errorf("%s: simplified:\n got %+v\nwant %+v", b.Name, got, b.Simplified)
		}
	}
}

// friend is the part of a FriendView a sheet vector gives.
type friend struct {
	Total    money.Balances
	ByGroup  map[uuid.UUID]money.Balances
	NonGroup money.Balances
}

// vectorLedger serves one sheet vector to Service.For.
type vectorLedger struct {
	groups   []GroupRef
	entries  map[uuid.UUID][]Entry
	nonGroup []Entry
}

func (l vectorLedger) Group(_ context.Context, id uuid.UUID) ([]Entry, error) {
	return l.entries[id], nil
}

func (l vectorLedger) Groups(context.Context, uuid.UUID) ([]GroupRef, error) {
	return l.groups, nil
}

func (l vectorLedger) GroupsEntries(context.Context, []uuid.UUID) (map[uuid.UUID][]Entry, error) {
	return l.entries, nil
}

func (l vectorLedger) NonGroup(context.Context, uuid.UUID) ([]Entry, error) {
	return l.nonGroup, nil
}

func TestSheetVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sheets []struct {
			Name   string
			Me     uuid.UUID
			Groups []struct {
				ID       uuid.UUID
				Name     string
				Simplify bool
				Entries  []Entry
			}
			NonGroup  []Entry
			Friends   map[uuid.UUID]friend
			GroupNets map[uuid.UUID]money.Balances
			Net       money.Balances
			Owed      money.Balances
			Owing     money.Balances
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Sheets) == 0 {
		t.Fatal("no sheet vectors")
	}
	for _, c := range doc.Sheets {
		l := vectorLedger{entries: map[uuid.UUID][]Entry{}, nonGroup: c.NonGroup}
		for _, g := range c.Groups {
			l.groups = append(l.groups, GroupRef{ID: g.ID, Name: g.Name, Simplify: g.Simplify})
			l.entries[g.ID] = g.Entries
		}
		sheet, err := NewService(l).For(context.Background(), c.Me)
		if err != nil {
			t.Fatal(err)
		}
		friends := map[uuid.UUID]friend{}
		for id, f := range sheet.Friends {
			friends[id] = friend{Total: f.Total, ByGroup: f.ByGroup, NonGroup: f.NonGroup}
		}
		if got, want := pruneFriends(friends), pruneFriends(c.Friends); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: friends:\n got %+v\nwant %+v", c.Name, got, want)
		}
		if got, want := prune(sheet.Groups), prune(c.GroupNets); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: group nets:\n got %+v\nwant %+v", c.Name, got, want)
		}
		net, owed, owing := sheet.Totals()
		for _, x := range []struct {
			name      string
			got, want money.Balances
		}{{"net", net, c.Net}, {"owed", owed, c.Owed}, {"owing", owing, c.Owing}} {
			if !reflect.DeepEqual(nonZero(x.got), nonZero(x.want)) {
				t.Errorf("%s: %s %v, want %v", c.Name, x.name, x.got, x.want)
			}
		}
	}
}

// nonZero drops settled currencies, which the two ports may or may not keep.
func nonZero(b money.Balances) money.Balances {
	out := money.Balances{}
	for c, a := range b {
		if a != 0 {
			out[c] = a
		}
	}
	return out
}

func prune(groups map[uuid.UUID]money.Balances) map[uuid.UUID]money.Balances {
	out := map[uuid.UUID]money.Balances{}
	for id, b := range groups {
		if b := nonZero(b); len(b) > 0 {
			out[id] = b
		}
	}
	return out
}

func pruneFriends(friends map[uuid.UUID]friend) map[uuid.UUID]friend {
	out := map[uuid.UUID]friend{}
	for id, f := range friends {
		out[id] = friend{Total: nonZero(f.Total), ByGroup: prune(f.ByGroup), NonGroup: nonZero(f.NonGroup)}
	}
	return out
}
