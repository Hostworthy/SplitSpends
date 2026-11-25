package transfer

import (
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// TestRemapGroupRejectsCollidingPeople checks that two people from the
// device who turn out to be one person on the server are refused, every
// time, instead of one of their default split values winning at random.
func TestRemapGroupRejectsCollidingPeople(t *testing.T) {
	a, b, x := uuid.New(), uuid.New(), uuid.New()
	people := map[uuid.UUID]uuid.UUID{a: x, b: x}
	names := map[uuid.UUID]string{a: "Ada", b: "Ada (work)"}
	g := Group{ID: uuid.New(), Members: []uuid.UUID{a, b}, Patch: group.Patch{
		DefaultSplit: &group.DefaultSplit{Method: group.SplitShares, Values: map[uuid.UUID]string{a: "1", b: "3"}}}}
	for range 100 {
		_, _, err := remapGroup(people, names, g)
		de, ok := errs.As(err)
		if !ok || de.Kind != errs.KindValidation || de.Field != "defaultSplit" {
			t.Fatalf("want a validation error on defaultSplit, got %v", err)
		}
		if !strings.Contains(de.Message, "“Ada”") || !strings.Contains(de.Message, "“Ada (work)”") {
			t.Fatalf("the message must name both people: %q", de.Message)
		}
	}

	people[b] = uuid.New()
	if _, _, err := remapGroup(people, names, g); err != nil {
		t.Fatalf("distinct people are fine: %v", err)
	}
}

// TestRemapExpenseRejectsCollidingPeople checks the same for an expense's
// payers, splits and items.
func TestRemapExpenseRejectsCollidingPeople(t *testing.T) {
	a, b, x := uuid.New(), uuid.New(), uuid.New()
	people := map[uuid.UUID]uuid.UUID{a: x, b: x}
	names := map[uuid.UUID]string{a: "Ada", b: "Ada (work)"}
	for field, in := range map[string]expense.Input{
		"payers": {Payers: []expense.Payer{{UserID: a}, {UserID: b}}, Splits: []expense.SplitInput{{UserID: a}}},
		"splits": {Payers: []expense.Payer{{UserID: a}}, Splits: []expense.SplitInput{{UserID: a}, {UserID: b}}},
		"items": {Payers: []expense.Payer{{UserID: a}}, Splits: []expense.SplitInput{{UserID: a}},
			Items: []expense.Item{{Name: "Bread", UserIDs: []uuid.UUID{a, b}}}},
	} {
		_, err := remapExpense(people, names, in)
		if de, ok := errs.As(err); !ok || de.Kind != errs.KindValidation || de.Field != field {
			t.Errorf("%s: want a validation error on %s, got %v", field, field, err)
		}
	}
}
