package balance

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

var flat = uuid.MustParse("00000000-0000-0000-0000-0000000000f1")

// paid is ledger after c pays a and b back in full: everyone is settled in
// USD, while b still owes a in EUR.
var paid = append(append([]Entry{}, ledger...),
	Entry{Currency: "USD", Shares: []Share{{c, 2000, 0}, {a, 0, 1000}, {b, 0, 1000}}},
	Entry{Currency: "EUR", Shares: []Share{{a, 500, 0}, {b, 0, 500}}},
)

func TestSettled(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		entries []Entry
		user    uuid.UUID
		want    bool
	}{
		{"someone who is owed", ledger, a, false},
		{"someone who owes", ledger, c, false},
		{"someone with no entries", ledger, d, true},
		{"someone paid back in full", paid, c, true},
		{"someone square in one currency but owed in another", paid, a, false},
		{"an empty group", nil, a, true},
	} {
		s := NewService(vectorLedger{entries: map[uuid.UUID][]Entry{flat: tc.entries}})
		got, err := s.Settled(ctx, flat, tc.user)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: settled is %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGroupSettled(t *testing.T) {
	ctx := context.Background()
	square := append(append([]Entry{}, paid...), Entry{Currency: "EUR", Shares: []Share{{b, 500, 0}, {a, 0, 500}}})
	for _, tc := range []struct {
		name    string
		entries []Entry
		want    bool
	}{
		{"open debts", ledger, false},
		{"one currency still open", paid, false},
		{"every currency square", square, true},
		{"no entries", nil, true},
	} {
		s := NewService(vectorLedger{entries: map[uuid.UUID][]Entry{flat: tc.entries}})
		got, err := s.GroupSettled(ctx, flat)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: group settled is %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGroupView(t *testing.T) {
	s := NewService(vectorLedger{entries: map[uuid.UUID][]Entry{flat: ledger}})
	for _, simplify := range []bool{false, true} {
		v, err := s.Group(context.Background(), flat, simplify)
		if err != nil {
			t.Fatal(err)
		}
		if want := Nets(ledger); !reflect.DeepEqual(v.Members, want) {
			t.Errorf("simplify %v: members %v, want %v", simplify, v.Members, want)
		}
		if want := Debts(ledger, simplify); !reflect.DeepEqual(v.Debts, want) {
			t.Errorf("simplify %v: debts %+v, want %+v", simplify, v.Debts, want)
		}
	}
}

// broken fails the read named by fail and serves one group otherwise.
type broken struct {
	vectorLedger
	fail string
}

var errLedger = errors.New("ledger unavailable")

func (l broken) Groups(ctx context.Context, id uuid.UUID) ([]GroupRef, error) {
	if l.fail == "groups" {
		return nil, errLedger
	}
	return l.vectorLedger.Groups(ctx, id)
}

func (l broken) GroupsEntries(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]Entry, error) {
	if l.fail == "entries" {
		return nil, errLedger
	}
	return l.vectorLedger.GroupsEntries(ctx, ids)
}

func (l broken) NonGroup(ctx context.Context, id uuid.UUID) ([]Entry, error) {
	if l.fail == "nonGroup" {
		return nil, errLedger
	}
	return l.vectorLedger.NonGroup(ctx, id)
}

func TestForReturnsLedgerErrors(t *testing.T) {
	healthy := vectorLedger{groups: []GroupRef{{ID: flat, Name: "Flat"}}, entries: map[uuid.UUID][]Entry{flat: ledger}}
	for _, fail := range []string{"groups", "entries", "nonGroup"} {
		sheet, err := NewService(broken{healthy, fail}).For(context.Background(), a)
		if !errors.Is(err, errLedger) || sheet != nil {
			t.Errorf("a failing %s read must fail the sheet, got %+v and %v", fail, sheet, err)
		}
	}
	sheet, err := NewService(broken{vectorLedger: healthy}).For(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if got := sheet.Groups[flat]; !reflect.DeepEqual(got, money.Balances{"USD": 1000}) {
		t.Errorf("a's net in the flat is %v, want 1000 USD", got)
	}
}
