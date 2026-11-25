package bootstrap_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/category"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
)

func TestRetryMustMatch(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services.Expenses
	e := series(t, env, ada, ben, &g.ID, "Rent", "2026-03-01", expense.RecurNone)
	p, err := svc.RecordPayment(ctx, ada, expense.PaymentInput{GroupID: &g.ID, From: ben, To: ada, Amount: 500, Currency: "EUR"})
	if err != nil {
		t.Fatal(err)
	}
	same := expense.Input{ID: &e.ID, GroupID: &g.ID, Description: "Rent", Currency: "EUR", Total: 100000, Method: expense.MethodEqual,
		Payers: []expense.Payer{{UserID: ada, Amount: 100000}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}}
	if got, err := svc.Create(ctx, ada, same); err != nil || got.ID != e.ID {
		t.Fatalf("a retried create must return the stored expense, got %v, %v", got, err)
	}
	if got, err := svc.RecordPayment(ctx, ada, expense.PaymentInput{ID: &p.ID, GroupID: &g.ID, From: ben, To: ada, Amount: 500}); err != nil || got.ID != p.ID {
		t.Fatalf("a retried payment must return the stored payment, got %v, %v", got, err)
	}

	outside := same
	outside.GroupID = nil
	payment := same
	payment.ID = &p.ID
	for name, call := range map[string]func() (*expense.Expense, error){
		"payment with an expense id": func() (*expense.Expense, error) {
			return svc.RecordPayment(ctx, ada, expense.PaymentInput{ID: &e.ID, GroupID: &g.ID, From: ben, To: ada, Amount: 500})
		},
		"expense with a payment id":     func() (*expense.Expense, error) { return svc.Create(ctx, ada, payment) },
		"expense id from another group": func() (*expense.Expense, error) { return svc.Create(ctx, ada, outside) },
	} {
		if _, err := call(); !errs.Is(err, errs.KindConflict) {
			t.Errorf("%s: want a conflict, got %v", name, err)
		}
	}
}

func TestRetryRace(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	id := uuid.New()
	in := expense.Input{ID: &id, GroupID: &g.ID, Description: "Rent", Currency: "EUR", Total: 100000, Method: expense.MethodEqual,
		Payers: []expense.Payer{{UserID: ada, Amount: 100000}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}}
	// The first request saves the expense after the retry has looked for it
	// and found nothing.
	svc := wired(env, held{repositories.NewExpenses(env.DB), &sync.Once{}, func() {
		if _, err := env.Services.Expenses.Create(ctx, ada, in); err != nil {
			t.Error(err)
		}
	}})
	got, err := svc.Create(ctx, ada, in)
	if err != nil {
		t.Fatalf("the retry must return the stored expense, got %v", err)
	}
	if got.ID != id {
		t.Fatalf("want expense %s, got %s", id, got.ID)
	}
	if list := groupExpenses(t, env, ada, g.ID); len(list) != 1 {
		t.Fatalf("want 1 expense, got %d", len(list))
	}
}

// late is the group repository with a hook that runs once, right after the
// first read of a group.
type late struct {
	*repositories.Groups
	once  *sync.Once
	after func()
}

func (l late) Get(ctx context.Context, id uuid.UUID) (*group.Group, error) {
	g, err := l.Groups.Get(ctx, id)
	l.once.Do(l.after)
	return g, err
}

func TestGroupRetryRace(t *testing.T) {
	ctx := context.Background()
	env, ada, _, _ := flat(t)
	s := env.Services
	id := uuid.New()
	name := "Trip"
	in := group.CreateInput{ID: &id, Patch: group.Patch{Name: &name}}
	repo := late{repositories.NewGroups(env.DB), &sync.Once{}, func() {
		if _, err := s.Groups.Create(ctx, ada, in); err != nil {
			t.Error(err)
		}
	}}
	svc := group.NewService(group.Deps{Repo: repo, Users: s.Users, Balances: s.Balances, Activity: s.Activity, Notifier: env.Notifier,
		Quota: repositories.NewQuotas(env.DB), Tx: database.NewTx(env.DB), Clock: env.Clock, BaseURL: "http://test"})
	got, err := svc.Create(ctx, ada, in)
	if err != nil {
		t.Fatalf("the retry must return the stored group, got %v", err)
	}
	if got.ID != id {
		t.Fatalf("want group %s, got %s", id, got.ID)
	}
}

// looked is the category repository with a hook that runs once, right after
// the first read of a category.
type looked struct {
	*repositories.Categories
	once  *sync.Once
	after func()
}

func (l looked) Get(ctx context.Context, id string) (*category.Category, error) {
	c, err := l.Categories.Get(ctx, id)
	l.once.Do(l.after)
	return c, err
}

func TestCategoryRetryRace(t *testing.T) {
	ctx := context.Background()
	env, ada, _, _ := flat(t)
	id := category.NewID()
	in := category.Input{ID: &id, Name: str("Plants")}
	// The first request saves the category after the retry has looked for it
	// and found nothing.
	svc := category.NewService(looked{repositories.NewCategories(env.DB), &sync.Once{}, func() {
		if _, err := env.Services.Categories.Create(ctx, ada, in); err != nil {
			t.Error(err)
		}
	}}, env.Clock)
	got, err := svc.Create(ctx, ada, in)
	if err != nil {
		t.Fatalf("the retry must return the stored category, got %v", err)
	}
	if got.ID != id || got.Name != "Plants" {
		t.Fatalf("want category %s, got %+v", id, got)
	}
	if _, err := svc.Create(ctx, uuid.New(), in); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("someone else's id must stay a conflict, got %v", err)
	}
}
