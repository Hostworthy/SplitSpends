package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/category"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/stats"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/transfer"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

func login(t *testing.T, svc *Services, notes *fakeNotifier, email, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if err := svc.Auth.RequestCode(ctx, email, ""); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Auth.Verify(ctx, email, notes.codes[email], name, "test")
	if err != nil {
		t.Fatal(err)
	}
	return res.User.ID
}

func str(s string) *string { return &s }

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	de, ok := errs.As(err)
	if !ok || de.Field != field {
		t.Fatalf("want a validation error on %q, got %v", field, err)
	}
}

func TestDefaultSplit(t *testing.T) {
	ctx := context.Background()
	svc, notes := setup(t, time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC))
	ada := login(t, svc, notes, "ada@example.com", "Ada")
	ben := login(t, svc, notes, "ben@example.com", "Ben")
	v, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: str("Flat")}, People: []uuid.UUID{ben}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Effective() != nil {
		t.Fatal("a new group has no default split")
	}

	// Rent is split 60/40, so every new expense in the flat defaults to that.
	rent := &group.DefaultSplit{Method: group.SplitPercent, Values: map[uuid.UUID]string{ada: "60", ben: "40"}}
	if v, err = svc.Groups.Update(ctx, ben, v.ID, group.Patch{DefaultSplit: rent}); err != nil {
		t.Fatal(err)
	}
	if d := v.Effective(); d == nil || d.Values[ada] != "60" || d.Values[ben] != "40" {
		t.Fatalf("default split = %+v", d)
	}
	reloaded, err := svc.Groups.View(ctx, ada, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d := reloaded.Effective(); d == nil || d.Method != group.SplitPercent || d.Values[ada] != "60" {
		t.Fatalf("stored default split = %+v", d)
	}

	_, err = svc.Groups.Update(ctx, ada, v.ID, group.Patch{DefaultSplit: &group.DefaultSplit{Method: group.SplitPercent,
		Values: map[uuid.UUID]string{ada: "70", ben: "40"}}})
	wantField(t, err, "defaultSplit")
	stranger := uuid.New()
	_, err = svc.Groups.Update(ctx, ada, v.ID, group.Patch{DefaultSplit: &group.DefaultSplit{Method: group.SplitShares,
		Values: map[uuid.UUID]string{ada: "1", stranger: "1"}}})
	wantField(t, err, "defaultSplit")

	// Someone joins: the 60/40 default no longer matches and is absent until
	// the members match again.
	if v, err = svc.Groups.AddMember(ctx, ada, v.ID, group.Invitee{Name: "Cy"}); err != nil {
		t.Fatal(err)
	}
	if v.Effective() != nil {
		t.Fatal("a percent default must be absent once someone joins")
	}
	var cy uuid.UUID
	for _, m := range v.Members {
		if m != ada && m != ben {
			cy = m
		}
	}
	if err := svc.Groups.RemoveMember(ctx, ada, v.ID, cy); err != nil {
		t.Fatal(err)
	}
	if v, err = svc.Groups.View(ctx, ada, v.ID); err != nil {
		t.Fatal(err)
	}
	if v.Effective() == nil {
		t.Fatal("the default applies again once the members match")
	}

	if v, err = svc.Groups.Update(ctx, ada, v.ID, group.Patch{DefaultSplit: &group.DefaultSplit{}}); err != nil {
		t.Fatal(err)
	}
	if v.DefaultSplit != nil {
		t.Fatal("an empty method clears the default split")
	}
}

func TestCustomCategories(t *testing.T) {
	ctx := context.Background()
	svc, notes := setup(t, time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC))
	ada := login(t, svc, notes, "ada@example.com", "Ada")
	ben := login(t, svc, notes, "ben@example.com", "Ben")
	g, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: str("Crag")}, People: []uuid.UUID{ben}})
	if err != nil {
		t.Fatal(err)
	}

	climbing, err := svc.Categories.Create(ctx, ada, category.Input{Name: str("Climbing"), Icon: str("fitness_center"), Group: str("Entertainment")})
	if err != nil {
		t.Fatal(err)
	}
	if !expense.CustomCategory(climbing.ID) {
		t.Fatalf("custom id %q", climbing.ID)
	}
	again, err := svc.Categories.Create(ctx, ada, category.Input{ID: &climbing.ID, Name: str("Climbing")})
	if err != nil || again.ID != climbing.ID {
		t.Fatalf("repeating a create with the same id must return it: %v", err)
	}
	if _, err := svc.Categories.Create(ctx, ben, category.Input{ID: &climbing.ID, Name: str("Mine")}); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("someone else's id must conflict, got %v", err)
	}
	if _, err := svc.Categories.Update(ctx, ben, climbing.ID, category.Input{Name: str("Hijack")}); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("only the owner may rename, got %v", err)
	}

	ids := func(user uuid.UUID) map[string]bool {
		list, err := svc.Categories.List(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, c := range list {
			out[c.ID] = c.Custom() && c.OwnerID == user
		}
		return out
	}
	if mine, ok := ids(ada)[climbing.ID]; !ok || !mine {
		t.Fatal("the owner sees their category as their own")
	}
	if _, ok := ids(ben)[climbing.ID]; ok {
		t.Fatal("others do not see an unused custom category")
	}
	if mine, ok := ids(ben)["food.dining"]; !ok || mine {
		t.Fatal("built-in categories are listed and belong to nobody")
	}

	split := []expense.SplitInput{{UserID: ada}, {UserID: ben}}
	shoes, err := svc.Expenses.Create(ctx, ada, expense.Input{GroupID: &g.ID, Description: "Shoes", Currency: "EUR", Total: 9000,
		Date: "2026-04-20", Category: climbing.ID, Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ada, Amount: 9000}}, Splits: split})
	if err != nil {
		t.Fatal(err)
	}
	if mine, ok := ids(ben)[climbing.ID]; !ok || mine {
		t.Fatal("members see custom categories used by expenses they can see, not as their own")
	}
	_, err = svc.Expenses.Create(ctx, ben, expense.Input{GroupID: &g.ID, Description: "Rope", Currency: "EUR", Total: 5000,
		Category: climbing.ID, Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ben, Amount: 5000}}, Splits: split})
	wantField(t, err, "category")
	edit := expense.Input{Description: "Climbing shoes", Currency: "EUR", Total: 9000, Date: "2026-04-20", Category: climbing.ID,
		Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ada, Amount: 9000}}, Splits: split}
	if _, err := svc.Expenses.Update(ctx, ben, shoes.ID, edit); err != nil {
		t.Fatalf("editing keeps the expense's own category: %v", err)
	}

	month := stats.Query{UserID: ada, GroupID: &g.ID, Currency: "EUR", From: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)}
	sum, err := svc.Stats.Summary(ctx, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.ByCategory) != 1 || sum.ByCategory[0].Key != climbing.ID || sum.ByCategory[0].Name != "Climbing" {
		t.Fatalf("stats by category = %+v", sum.ByCategory)
	}

	// Deleting hides the category but keeps history: the expense keeps its
	// id, shows as General in the stats and can still be edited.
	if err := svc.Categories.Delete(ctx, ada, climbing.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := ids(ada)[climbing.ID]; ok {
		t.Fatal("deleted categories are not listed")
	}
	kept, err := svc.Expenses.Get(ctx, ben, shoes.ID)
	if err != nil || kept.Category != climbing.ID {
		t.Fatalf("history must not be rewritten: %v %v", kept, err)
	}
	if _, err := svc.Expenses.Update(ctx, ada, shoes.ID, edit); err != nil {
		t.Fatalf("editing an expense of a deleted category: %v", err)
	}
	edit.Description = "New shoes"
	_, err = svc.Expenses.Create(ctx, ada, edit)
	wantField(t, err, "category")
	if sum, err = svc.Stats.Summary(ctx, month); err != nil {
		t.Fatal(err)
	}
	if len(sum.ByCategory) != 1 || sum.ByCategory[0].Key != expense.DefaultCategory || sum.ByCategory[0].Name != "General" {
		t.Fatalf("deleted categories count as General: %+v", sum.ByCategory)
	}
}

func TestImportDefaultSplitAndCategories(t *testing.T) {
	ctx := context.Background()
	svc, notes := setup(t, time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC))
	cara := login(t, svc, notes, "cara@example.com", "Cara")

	me, dan, flat := uuid.New(), uuid.New(), uuid.New()
	live, gone := "custom."+uuid.NewString(), "custom."+uuid.NewString()
	split := []expense.SplitInput{{UserID: me, Value: "60"}, {UserID: dan, Value: "40"}}
	bundle := transfer.Bundle{
		Me:     me,
		People: []transfer.Person{{ID: dan, Name: "Dan"}},
		Categories: []transfer.Category{
			{Input: category.Input{ID: &live, Name: str("Plants"), Icon: str("park"), Group: str("Home")}},
			{Input: category.Input{ID: &gone, Name: str("Old"), Icon: str("label")}, Deleted: true},
		},
		Groups: []transfer.Group{{ID: flat, Members: []uuid.UUID{me, dan}, Patch: group.Patch{Name: str("Flat"),
			DefaultSplit: &group.DefaultSplit{Method: group.SplitPercent, Values: map[uuid.UUID]string{me: "60", dan: "40"}}}}},
	}
	for i, cat := range []string{live, gone} {
		id := uuid.New()
		bundle.Expenses = append(bundle.Expenses, transfer.Expense{Kind: expense.KindExpense, Input: expense.Input{ID: &id, GroupID: &flat,
			Description: "Thing", Currency: "EUR", Total: int64(1000 * (i + 1)), Date: "2026-04-01", Category: cat, Method: expense.MethodPercent,
			Payers: []expense.Payer{{UserID: me, Amount: int64(1000 * (i + 1))}}, Splits: split}})
	}
	for attempt := 0; attempt < 2; attempt++ {
		res, err := svc.Transfer.Import(ctx, cara, bundle)
		if err != nil {
			t.Fatalf("import %d: %v", attempt, err)
		}
		if res.Categories != 2 || res.Expenses != 2 || res.People[me] != cara {
			t.Fatalf("import %d result = %+v", attempt, res)
		}
	}

	v, err := svc.Groups.View(ctx, cara, flat)
	if err != nil {
		t.Fatal(err)
	}
	d := v.Effective()
	if d == nil || d.Values[cara] != "60" || len(d.Values) != 2 || d.Values[dan] != "40" {
		t.Fatalf("imported default split must use server ids: %+v", d)
	}
	list, err := svc.Categories.List(ctx, cara)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range list {
		seen[c.ID] = true
	}
	if !seen[live] || seen[gone] {
		t.Fatalf("live custom categories are listed, deleted ones are not: %v", seen)
	}
	items, total, err := svc.Expenses.List(ctx, expense.Filter{UserID: cara, GroupID: &flat, Limit: 10})
	if err != nil || total != 2 {
		t.Fatalf("imported expenses: %d %v", total, err)
	}
	cats := map[string]bool{}
	for _, e := range items {
		cats[e.Category] = true
	}
	if !cats[live] || !cats[gone] {
		t.Fatalf("expenses keep their categories: %v", cats)
	}
}
