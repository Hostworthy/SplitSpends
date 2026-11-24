package category

import (
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

func ptr(s string) *string { return &s }

func TestNew(t *testing.T) {
	owner := uuid.New()
	now := time.Now()
	id := "custom." + uuid.NewString()
	cases := []struct {
		name  string
		in    Input
		field string
	}{
		{"name and defaults", Input{Name: ptr("  Board   games ")}, ""},
		{"chosen id", Input{ID: &id, Name: ptr("Climbing"), Icon: ptr("fitness_center"), Group: ptr("Entertainment")}, ""},
		{"empty name", Input{Name: ptr("   ")}, "name"},
		{"long name", Input{Name: ptr(strings.Repeat("a", 41))}, "name"},
		{"forty characters", Input{Name: ptr(strings.Repeat("é", 40))}, ""},
		{"unknown icon", Input{Name: ptr("Bikes"), Icon: ptr("rocket_launch")}, "icon"},
		{"unknown group", Input{Name: ptr("Bikes"), Group: ptr("Hobbies")}, "group"},
		{"built-in id", Input{ID: ptr("food.dining"), Name: ptr("Bikes")}, "id"},
		{"non-canonical id", Input{ID: ptr("custom.not-a-uuid"), Name: ptr("Bikes")}, "id"},
	}
	for _, c := range cases {
		got, err := New(owner, c.in, now)
		if c.field != "" {
			de, ok := errs.As(err)
			if !ok || de.Field != c.field {
				t.Errorf("%s: want error on %q, got %v", c.name, c.field, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !expense.CustomCategory(got.ID) || got.OwnerID != owner || !got.Custom() {
			t.Errorf("%s: bad category %+v", c.name, got)
		}
	}
	c, _ := New(owner, Input{Name: ptr("  Board   games ")}, now)
	if c.Name != "Board games" || c.Icon != DefaultIcon || c.Group != DefaultGroup {
		t.Fatalf("defaults not applied: %+v", c)
	}
}

func TestIconsAndGroups(t *testing.T) {
	for _, b := range expense.Categories() {
		if b.ID == expense.PaymentCategory {
			continue
		}
		found := false
		for _, icon := range Icons {
			found = found || icon == b.Icon
		}
		if !found {
			t.Errorf("built-in icon %q is not offered for custom categories", b.Icon)
		}
	}
	groups := Groups()
	if len(groups) == 0 || groups[len(groups)-1] != DefaultGroup {
		t.Fatalf("groups = %v", groups)
	}
}

func TestCustomCategoryID(t *testing.T) {
	id := uuid.NewString()
	cases := map[string]bool{
		"custom." + id:                  true,
		"custom." + strings.ToUpper(id): false,
		"custom.":                       false,
		id:                              false,
		"general":                       false,
	}
	for in, want := range cases {
		if got := expense.CustomCategory(in); got != want {
			t.Errorf("CustomCategory(%q) = %v, want %v", in, got, want)
		}
	}
}
