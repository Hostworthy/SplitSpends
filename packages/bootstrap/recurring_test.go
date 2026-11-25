package bootstrap_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
)

// flat opens an environment on 15 March 2026 with Ada and Ben sharing a
// group.
func flat(t *testing.T) (env *bootstraptest.Env, ada, ben uuid.UUID, g *group.View) {
	t.Helper()
	env = bootstraptest.Open(t, bootstraptest.Options{Now: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)})
	ada = env.SignIn(t, "ada@example.com", "Ada").User.ID
	name := "Flat"
	g, err := env.Services.Groups.Create(context.Background(), ada, group.CreateInput{Patch: group.Patch{Name: &name},
		Members: []group.Invitee{{Email: "ben@example.com", Name: "Ben"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range g.Members {
		if m != ada {
			ben = m
		}
	}
	return env, ada, ben, g
}

// series books a recurring expense Ada paid and shares equally with Ben.
func series(t *testing.T, env *bootstraptest.Env, ada, ben uuid.UUID, groupID *uuid.UUID, description, date string,
	r expense.Recurrence) *expense.Expense {
	t.Helper()
	e, err := env.Services.Expenses.Create(context.Background(), ada, expense.Input{GroupID: groupID, Description: description,
		Currency: "EUR", Total: 100000, Date: date, Method: expense.MethodEqual, Recurrence: r,
		Payers: []expense.Payer{{UserID: ada, Amount: 100000}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// drain runs the job like RunJobsOnce: pass after pass until nothing is due.
func drain(t *testing.T, svc *expense.Service) int {
	t.Helper()
	total := 0
	for pass := 0; pass < 50; pass++ {
		n, err := svc.RunRecurring(context.Background())
		if err != nil {
			t.Error(err)
			return total
		}
		if n == 0 {
			break
		}
		total += n
	}
	return total
}

func groupExpenses(t *testing.T, env *bootstraptest.Env, actor, groupID uuid.UUID) []*expense.Expense {
	t.Helper()
	list, _, err := env.Services.Expenses.List(context.Background(), expense.Filter{UserID: actor, GroupID: &groupID, Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func scheduled(list []*expense.Expense) []*expense.Expense {
	var out []*expense.Expense
	for _, e := range list {
		if e.Recurrence != expense.RecurNone {
			out = append(out, e)
		}
	}
	return out
}

func settle(t *testing.T, env *bootstraptest.Env, ada, ben uuid.UUID, groupID *uuid.UUID) {
	t.Helper()
	_, err := env.Services.Expenses.RecordPayment(context.Background(), ada, expense.PaymentInput{GroupID: groupID,
		From: ben, To: ada, Amount: 50000, Currency: "EUR", Date: "2026-03-15"})
	if err != nil {
		t.Fatal(err)
	}
}

func stopped(t *testing.T, env *bootstraptest.Env, actor uuid.UUID, id uuid.UUID) bool {
	t.Helper()
	feed, _, err := env.Services.Activity.List(context.Background(), activity.Filter{UserID: actor, ExpenseID: &id, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range feed {
		if a.Kind == activity.ExpenseStopped {
			return true
		}
	}
	return false
}

func TestRecurringStopsForLeftMember(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	rent := series(t, env, ada, ben, &g.ID, "Rent", "2026-03-01", expense.RecurMonthly)
	settle(t, env, ada, ben, &g.ID)
	if err := svc.Groups.RemoveMember(ctx, ada, g.ID, ben); err != nil {
		t.Fatal(err)
	}
	env.Clock.Add(30 * 24 * time.Hour)
	if n := drain(t, svc.Expenses); n != 0 {
		t.Fatalf("the job booked %d copies that bill someone who left", n)
	}
	list := groupExpenses(t, env, ada, g.ID)
	if len(list) != 2 {
		t.Fatalf("want the rent and the payment only, got %d expenses", len(list))
	}
	if len(scheduled(list)) != 0 {
		t.Fatal("the series should have ended")
	}
	if !stopped(t, env, ada, rent.ID) {
		t.Fatal("the activity feed should say why the series ended")
	}
	sheet, err := svc.Groups.Balances(ctx, ada, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := sheet.Members[ben]["EUR"]; got != 0 {
		t.Fatalf("Ben left settled and must owe nothing, got %d", got)
	}
}

func TestRecurringStopsForRemovedFriend(t *testing.T) {
	ctx := context.Background()
	env, ada, _, _ := flat(t)
	svc := env.Services
	cleo, err := svc.Friends.Add(ctx, ada, "cleo@example.com", "Cleo")
	if err != nil {
		t.Fatal(err)
	}
	gym := series(t, env, ada, cleo.User.ID, nil, "Gym", "2026-03-01", expense.RecurMonthly)
	settle(t, env, ada, cleo.User.ID, nil)
	if err := svc.Friends.Remove(ctx, ada, cleo.User.ID); err != nil {
		t.Fatal(err)
	}
	env.Clock.Add(30 * 24 * time.Hour)
	if n := drain(t, svc.Expenses); n != 0 {
		t.Fatalf("the job booked %d copies with someone who is no longer a friend", n)
	}
	e, err := svc.Expenses.Get(ctx, ada, gym.ID)
	if err != nil {
		t.Fatal(err)
	}
	if e.Recurrence != expense.RecurNone || e.NextDate != nil {
		t.Fatal("the series should have ended")
	}
	if !stopped(t, env, ada, gym.ID) {
		t.Fatal("the activity feed should say why the series ended")
	}
}

func TestRecurringCatchUpIsBounded(t *testing.T) {
	env, ada, ben, g := flat(t)
	series(t, env, ada, ben, &g.ID, "Cleaning", "2024-03-17", expense.RecurWeekly)
	if n := drain(t, env.Services.Expenses); n != expense.CatchUp {
		t.Fatalf("want the latest %d missed weeks booked, got %d", expense.CatchUp, n)
	}
	list := groupExpenses(t, env, ada, g.ID)
	if len(list) != expense.CatchUp+1 {
		t.Fatalf("want %d expenses, got %d", expense.CatchUp+1, len(list))
	}
	s := scheduled(list)
	if len(s) != 1 {
		t.Fatalf("exactly one expense should carry the schedule, got %d", len(s))
	}
	if s[0].Date.Format(time.DateOnly) != "2026-03-15" || s[0].NextDate.Format(time.DateOnly) != "2026-03-22" {
		t.Fatalf("the series should continue after today: %s next %s", s[0].Date, s[0].NextDate)
	}
}

func TestRecurringConcurrentRuns(t *testing.T) {
	const n = 30
	env, ada, ben, g := flat(t)
	for i := 0; i < n; i++ {
		series(t, env, ada, ben, &g.ID, fmt.Sprintf("Rent %d", i), "2026-02-15", expense.RecurMonthly)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	counts := make([]int, 2)
	for i := range counts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			counts[i] = drain(t, env.Services.Expenses)
		}()
	}
	close(start)
	wg.Wait()
	if counts[0]+counts[1] != n {
		t.Fatalf("runners booked %v copies, want %d in all", counts, n)
	}
	list := groupExpenses(t, env, ada, g.ID)
	if len(list) != 2*n {
		t.Fatalf("want %d expenses, got %d", 2*n, len(list))
	}
	if s := scheduled(list); len(s) != n {
		t.Fatalf("want one schedule per series, got %d", len(s))
	}
}

// paused is the expense repository with a hook that runs after Due has read
// the list, standing in for whatever happens before the job reaches a row.
type paused struct {
	*repositories.Expenses
	after func()
}

func (p paused) Due(ctx context.Context, day time.Time, limit int) ([]*expense.Expense, error) {
	list, err := p.Expenses.Due(ctx, day, limit)
	p.after()
	return list, err
}

func runner(env *bootstraptest.Env, after func()) *expense.Service {
	db := env.DB
	return expense.NewService(expense.Deps{Repo: paused{repositories.NewExpenses(db), after},
		Comments: repositories.NewComments(db), Groups: repositories.NewGroups(db), Connections: env.Services.Friends,
		Receipts: repositories.NewReceipts(db), Catalog: env.Services.Categories, Users: env.Services.Users,
		Activity: env.Services.Activity, Notifier: env.Notifier, Tx: database.NewTx(db), Clock: env.Clock, BaseURL: "http://test"})
}

func TestRecurringClaimsOnce(t *testing.T) {
	env, ada, ben, g := flat(t)
	series(t, env, ada, ben, &g.ID, "Rent", "2026-02-15", expense.RecurMonthly)
	var other int
	n := drain(t, runner(env, func() {
		if other == 0 {
			other = drain(t, env.Services.Expenses)
		}
	}))
	if other != 1 || n != 0 {
		t.Fatalf("the first runner to claim books the copy: got %d and %d", other, n)
	}
	if list := groupExpenses(t, env, ada, g.ID); len(list) != 2 {
		t.Fatalf("want 2 expenses, got %d", len(list))
	}
}

func TestRecurringKeepsConcurrentEdit(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	rent := series(t, env, ada, ben, &g.ID, "Rent", "2026-02-15", expense.RecurMonthly)
	edited := false
	n := drain(t, runner(env, func() {
		if edited {
			return
		}
		edited = true
		_, err := env.Services.Expenses.Update(ctx, ada, rent.ID, expense.Input{Description: "Rent", Total: 120000,
			Date: "2026-02-15", Method: expense.MethodEqual, Recurrence: expense.RecurMonthly,
			Payers: []expense.Payer{{UserID: ada, Amount: 120000}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
		if err != nil {
			t.Error(err)
		}
	}))
	if n != 1 {
		t.Fatalf("want 1 copy, got %d", n)
	}
	for _, e := range groupExpenses(t, env, ada, g.ID) {
		if e.Total != 120000 {
			t.Fatalf("%s on %s lost the edit: total %d", e.Description, e.Date.Format(time.DateOnly), e.Total)
		}
	}
}
