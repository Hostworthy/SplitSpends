package bootstrap_test

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
)

// doomed is the group repository with a hook that runs right before a group
// is deleted.
type doomed struct {
	*repositories.Groups
	before func()
}

func (d doomed) Delete(ctx context.Context, id uuid.UUID) error {
	d.before()
	return d.Groups.Delete(ctx, id)
}

func TestOnlyCreatorDeletesGroup(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	if err := svc.Groups.Delete(ctx, ben, g.ID); !errs.Is(err, errs.KindForbidden) {
		t.Fatalf("only the creator may delete the group, got %v", err)
	}
	if _, err := svc.Groups.View(ctx, ada, g.ID); err != nil {
		t.Fatalf("the refused delete removed the group: %v", err)
	}
	if err := svc.Groups.Delete(ctx, ada, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Groups.View(ctx, ada, g.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("the group should be gone, got %v", err)
	}
	feed, _, err := svc.Activity.List(ctx, activity.Filter{UserID: ben, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range feed {
		if a.Kind == activity.GroupDeleted {
			found = true
			if a.ActorID != ada || a.GroupID != nil || activity.Describe(a, "Ada") != "Ada deleted the group Flat" {
				t.Fatalf("unexpected entry %+v", a)
			}
		}
	}
	if !found {
		t.Fatal("Ben's feed should say who deleted the group")
	}
}

func TestMembersDeleteGroupWhoseCreatorLeft(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	if err := svc.Groups.RemoveMember(ctx, ada, g.ID, ada); err != nil {
		t.Fatal(err)
	}
	if err := svc.Groups.Delete(ctx, ben, g.ID); err != nil {
		t.Fatalf("once the creator has left, members may delete the group: %v", err)
	}
	if !hasKind(feed(t, env, ben), activity.GroupDeleted) {
		t.Fatal("the delete should be in Ben's feed")
	}
}

func feed(t *testing.T, env *bootstraptest.Env, user uuid.UUID) []*activity.Activity {
	t.Helper()
	list, _, err := env.Services.Activity.List(context.Background(), activity.Filter{UserID: user, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestDeleteGroupKeepsConcurrentExpense(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	created := make(chan error, 1)
	finished := make(chan struct{})
	var e *expense.Expense
	// Right before the group is deleted, Ben books an expense in it. The
	// delete goes on once the expense is saved or waiting for the delete.
	repo := doomed{repositories.NewGroups(env.DB), func() {
		go func() {
			var err error
			e, err = env.Services.Expenses.Create(ctx, ben, expense.Input{GroupID: &g.ID, Description: "Pizza", Currency: "EUR",
				Total: 3000, Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ben, Amount: 3000}},
				Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
			created <- err
			close(finished)
		}()
		for {
			select {
			case <-finished:
				return
			default:
			}
			if blocked(t, env) {
				return
			}
		}
	}}
	svc := group.NewService(group.Deps{Repo: repo, Users: env.Services.Users, Balances: env.Services.Balances,
		Activity: env.Services.Activity, Notifier: env.Notifier, Quota: repositories.NewQuotas(env.DB), Tx: database.NewTx(env.DB),
		Clock: env.Clock, BaseURL: "http://test"})
	deleted := svc.Delete(ctx, ada, g.ID)
	if err := <-created; err == nil {
		if !errs.Is(deleted, errs.KindConflict) {
			t.Fatalf("the group was deleted with an expense booked after the settled check: %v", deleted)
		}
		if _, err := env.Services.Expenses.Get(ctx, ben, e.ID); err != nil {
			t.Fatalf("the booked expense is gone: %v", err)
		}
	} else if deleted != nil {
		t.Fatalf("neither the expense nor the delete went through: %v, %v", err, deleted)
	}
}

// TestLastMemberLeavingIsRefused checks that the last member who can sign in
// cannot leave a group, which would leave it where nobody can open or
// delete it, also when the last two leave at the same moment.
func TestLastMemberLeavingIsRefused(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	if _, err := svc.Groups.AddMember(ctx, ada, g.ID, group.Invitee{Name: "Sam"}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, who := range []uuid.UUID{ada, ben} {
		go func() {
			<-start
			results <- svc.Groups.RemoveMember(ctx, who, g.ID, who)
		}()
	}
	close(start)
	left, refused := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			left++
		case errs.Is(err, errs.KindConflict):
			refused++
		default:
			t.Fatal(err)
		}
	}
	if left != 1 || refused != 1 {
		t.Fatalf("one of the last two may leave: %d left, %d refused", left, refused)
	}

	stayed := ada
	if v, err := svc.Groups.View(ctx, ada, g.ID); err != nil || !v.HasMember(ada) {
		stayed = ben
	}
	if err := svc.Groups.RemoveMember(ctx, stayed, g.ID, stayed); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("the last member who can sign in must not leave, got %v", err)
	}
	if err := svc.Groups.Delete(ctx, stayed, g.ID); err != nil {
		t.Fatalf("they can delete the group instead: %v", err)
	}
}

func TestArchivedGroupTakesNoMembers(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	cleo := env.SignIn(t, "cleo@example.com", "Cleo").User.ID
	archived := true
	if _, err := env.Services.Groups.Update(ctx, ada, g.ID, group.Patch{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Services.Groups.Join(ctx, cleo, g.ID); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("joining an archived group must be refused, got %v", err)
	}
	if _, err := env.Services.Groups.Join(ctx, ben, g.ID); err != nil {
		t.Fatalf("a member joining again changes nothing: %v", err)
	}
	if _, err := env.Services.Groups.View(ctx, cleo, g.ID); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("Cleo must not be a member: %v", err)
	}
}

// leaving is the group repository with a hook that runs right before a
// member is removed.
type leaving struct {
	*repositories.Groups
	before func()
}

func (l leaving) RemoveMember(ctx context.Context, groupID, userID uuid.UUID, at time.Time) error {
	l.before()
	return l.Groups.RemoveMember(ctx, groupID, userID, at)
}

// meanwhile runs fn in the background and returns once it has finished or
// some session waits for a lock, so the caller goes on while fn is either
// done or stuck behind it.
func meanwhile(t *testing.T, env *bootstraptest.Env, fn func()) {
	t.Helper()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		fn()
	}()
	for {
		select {
		case <-finished:
			return
		default:
		}
		if blocked(t, env) {
			return
		}
	}
}

func TestRemoveMemberKeepsConcurrentExpense(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	created := make(chan error, 1)
	var e *expense.Expense
	// Right before Ben is removed, Ada books an expense Ben shares.
	repo := leaving{repositories.NewGroups(env.DB), func() {
		meanwhile(t, env, func() {
			var err error
			e, err = env.Services.Expenses.Create(ctx, ada, expense.Input{GroupID: &g.ID, Description: "Pizza", Currency: "EUR",
				Total: 3000, Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ada, Amount: 3000}},
				Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
			created <- err
		})
	}}
	svc := group.NewService(group.Deps{Repo: repo, Users: env.Services.Users, Balances: env.Services.Balances,
		Activity: env.Services.Activity, Notifier: env.Notifier, Quota: repositories.NewQuotas(env.DB), Tx: database.NewTx(env.DB),
		Clock: env.Clock, BaseURL: "http://test"})
	removed := svc.RemoveMember(ctx, ada, g.ID, ben)
	if err := <-created; err == nil {
		if !errs.Is(removed, errs.KindConflict) {
			t.Fatalf("Ben was removed while owing for an expense booked after the settled check: %v", removed)
		}
		if _, err := env.Services.Expenses.Get(ctx, ben, e.ID); err != nil {
			t.Fatalf("the booked expense is gone: %v", err)
		}
	} else if removed != nil {
		t.Fatalf("neither the expense nor the removal went through: %v, %v", err, removed)
	} else if !errs.Is(err, errs.KindValidation) {
		t.Fatalf("an expense for someone just removed should be refused as invalid, got %v", err)
	}
}

// locking is the group repository with a hook that runs right before the
// group row is locked.
type locking struct {
	*repositories.Groups
	before func()
}

func (l locking) Lock(ctx context.Context, id uuid.UUID) (*group.Group, error) {
	l.before()
	return l.Groups.Lock(ctx, id)
}

func TestRemoveMemberChecksBalanceAfterLock(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	// Right before the group is locked for the removal, Ada books an expense
	// Ben shares. Nothing holds the group yet, so it is saved at once.
	repo := locking{repositories.NewGroups(env.DB), func() {
		if _, err := env.Services.Expenses.Create(ctx, ada, expense.Input{GroupID: &g.ID, Description: "Pizza", Currency: "EUR",
			Total: 3000, Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: ada, Amount: 3000}},
			Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}}); err != nil {
			t.Error(err)
		}
	}}
	svc := group.NewService(group.Deps{Repo: repo, Users: env.Services.Users, Balances: env.Services.Balances,
		Activity: env.Services.Activity, Notifier: env.Notifier, Quota: repositories.NewQuotas(env.DB), Tx: database.NewTx(env.DB),
		Clock: env.Clock, BaseURL: "http://test"})
	if err := svc.RemoveMember(ctx, ada, g.ID, ben); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("Ben was removed while owing for an expense booked before the lock: %v", err)
	}
}
