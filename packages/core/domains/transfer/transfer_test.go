package transfer

import (
	"errors"
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

// TestWrapNamesTheItem checks that a domain error from one item of an
// import names that item in its field and message, and that other errors
// pass through unchanged.
func TestWrapNamesTheItem(t *testing.T) {
	cases := []struct {
		name, path, label, field, message string
		err                               error
	}{
		{"field and label", "expenses[3]", "Rent", "expenses[3].total", "“Rent”: Must be positive.", errs.Validation("total", "Must be positive.")},
		{"no field", "groups[0]", "Flat", "groups[0]", "“Flat”: Taken.", errs.Conflict("Taken.")},
		{"no label", "people[1]", "", "people[1].email", "Invalid email.", errs.Validation("email", "Invalid email.")},
	}
	for _, c := range cases {
		de, ok := errs.As(wrap(c.err, c.path, c.label))
		if !ok || de.Field != c.field || de.Message != c.message {
			t.Errorf("%s: want %s %q, got %+v", c.name, c.field, c.message, de)
		}
		if orig, _ := errs.As(c.err); de.Kind != orig.Kind {
			t.Errorf("%s: the kind must stay %s, got %s", c.name, orig.Kind, de.Kind)
		}
	}
	plain := errors.New("database.down")
	if got := wrap(plain, "people[0]", "Sam"); !errors.Is(got, plain) || got.Error() != plain.Error() {
		t.Errorf("other errors must pass through, got %v", got)
	}
}
