package expense

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type memExpenses map[uuid.UUID]*Expense

func (m memExpenses) Create(_ context.Context, e *Expense) error {
	if _, ok := m[e.ID]; ok {
		return errs.Conflict("taken")
	}
	cp := *e
	m[e.ID] = &cp
	return nil
}
func (m memExpenses) Get(_ context.Context, id uuid.UUID) (*Expense, error) {
	e, ok := m[id]
	if !ok {
		return nil, errs.NotFound("Expense")
	}
	cp := *e
	return &cp, nil
}
func (m memExpenses) Lock(ctx context.Context, id uuid.UUID) (*Expense, error) { return m.Get(ctx, id) }
func (m memExpenses) Update(_ context.Context, e *Expense) error {
	cp := *e
	m[e.ID] = &cp
	return nil
}
func (m memExpenses) List(context.Context, Filter) ([]*Expense, int, error)   { return nil, 0, nil }
func (m memExpenses) Due(context.Context, time.Time, int) ([]*Expense, error) { return nil, nil }
func (m memExpenses) After(context.Context, uuid.UUID, *Expense, int) ([]*Expense, error) {
	return nil, nil
}
func (m memExpenses) Claim(context.Context, uuid.UUID, time.Time) (*Expense, error) { return nil, nil }
func (m memExpenses) EndSeries(context.Context, uuid.UUID, time.Time) error         { return nil }

type memComments map[uuid.UUID]*Comment

func (m memComments) Create(_ context.Context, c *Comment) error { m[c.ID] = c; return nil }
func (m memComments) Get(_ context.Context, id uuid.UUID) (*Comment, error) {
	c, ok := m[id]
	if !ok {
		return nil, errs.NotFound("Comment")
	}
	return c, nil
}
func (m memComments) List(context.Context, uuid.UUID, int, int) ([]*Comment, int, error) {
	return nil, 0, nil
}
func (m memComments) Count(_ context.Context, id uuid.UUID) (int, error) {
	n := 0
	for _, c := range m {
		if c.ExpenseID == id {
			n++
		}
	}
	return n, nil
}
func (m memComments) Delete(_ context.Context, id uuid.UUID) error { delete(m, id); return nil }

// memGroups answers membership the way group.Service.Member does.
type memGroups map[uuid.UUID]*group.Group

func (m memGroups) Member(_ context.Context, actor, id uuid.UUID) (*group.Group, error) {
	g, ok := m[id]
	if !ok || !g.HasMember(actor) {
		return nil, errs.NotFound("Group")
	}
	return g, nil
}
func (m memGroups) Lookup(_ context.Context, id uuid.UUID) (*group.Group, error) {
	g, ok := m[id]
	if !ok {
		return nil, errs.NotFound("Group")
	}
	return g, nil
}
func (m memGroups) Hold(_ context.Context, id uuid.UUID) error {
	if _, ok := m[id]; !ok {
		return errs.NotFound("Group")
	}
	return nil
}

// friends holds connected pairs in both directions.
type friends map[[2]uuid.UUID]bool

func (f friends) Connected(_ context.Context, a, b uuid.UUID) (bool, error) {
	return f[[2]uuid.UUID{a, b}], nil
}
func (f friends) Hold(context.Context, uuid.UUID, uuid.UUID) error { return nil }

// uploads maps a receipt to the person who uploaded it.
type uploads map[uuid.UUID]uuid.UUID

func (u uploads) Owned(_ context.Context, id, userID uuid.UUID) (bool, error) {
	by, ok := u[id]
	return ok && by == userID, nil
}

type noCategories struct{}

func (noCategories) Usable(context.Context, uuid.UUID, string) (bool, error) { return false, nil }

type memUsers map[uuid.UUID]*user.User

// downUsers is a directory whose bulk lookup fails.
type downUsers struct{ memUsers }

func (downUsers) List(context.Context, []uuid.UUID) ([]*user.User, error) {
	return nil, errors.New("connection refused")
}

func (m memUsers) Get(_ context.Context, id uuid.UUID) (*user.User, error) {
	u, ok := m[id]
	if !ok {
		return nil, errs.NotFound("User")
	}
	return u, nil
}
func (m memUsers) List(_ context.Context, ids []uuid.UUID) ([]*user.User, error) {
	var out []*user.User
	for _, id := range ids {
		if u, ok := m[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}
func (m memUsers) Index(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*user.User, error) {
	out := map[uuid.UUID]*user.User{}
	for _, id := range ids {
		if u, ok := m[id]; ok {
			out[id] = u
		}
	}
	return out, nil
}

type feed struct{ kinds []activity.Kind }

func (f *feed) Record(_ context.Context, a activity.Activity) error {
	f.kinds = append(f.kinds, a.Kind)
	return nil
}

type quiet struct{}

func (quiet) LoginCode(context.Context, string, string, time.Duration) error { return nil }
func (quiet) Invite(context.Context, string, string) error                   { return nil }
func (quiet) Notify(context.Context, ports.Notification)                     {}

type direct struct{}

func (direct) Do(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

// world is a group with Ada and Cy as members and Ben, who has left, plus
// Mal, who was never in it. Ada and Zoe are friends outside groups.
type world struct {
	svc                    *Service
	expenses               memExpenses
	feed                   *feed
	logs                   *bytes.Buffer
	group                  *group.Group
	ada, ben, cy, zoe, mal uuid.UUID
	rent, dinner           *Expense
	adaReceipt, malReceipt uuid.UUID
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{expenses: memExpenses{}, feed: &feed{}, logs: &bytes.Buffer{}}
	w.ada, w.ben, w.cy, w.zoe, w.mal = uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	w.adaReceipt, w.malReceipt = uuid.New(), uuid.New()
	people := memUsers{}
	for id, name := range map[uuid.UUID]string{w.ada: "Ada", w.ben: "Ben", w.cy: "Cy", w.zoe: "Zoe", w.mal: "Mal"} {
		people[id] = &user.User{ID: id, Name: name, DefaultCurrency: "EUR"}
	}
	w.group = &group.Group{ID: uuid.New(), Name: "Flat", DefaultCurrency: "EUR", Members: []uuid.UUID{w.ada, w.cy}}
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	w.rent = &Expense{ID: uuid.New(), GroupID: &w.group.ID, Kind: KindExpense, Description: "Rent", Currency: "EUR", Total: 1000,
		Date: now, Method: MethodEqual, Shares: []Share{{UserID: w.ada, Paid: 1000, Owed: 500}, {UserID: w.ben, Owed: 500}},
		CreatedBy: w.ada, CreatedAt: now, UpdatedAt: now}
	w.dinner = &Expense{ID: uuid.New(), Kind: KindExpense, Description: "Dinner", Currency: "EUR", Total: 1000,
		Date: now, Method: MethodEqual, Shares: []Share{{UserID: w.ada, Paid: 1000, Owed: 500}, {UserID: w.zoe, Owed: 500}},
		CreatedBy: w.ada, CreatedAt: now, UpdatedAt: now}
	for _, e := range []*Expense{w.rent, w.dinner} {
		if err := w.expenses.Create(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	w.svc = NewService(Deps{Repo: w.expenses, Comments: memComments{}, Groups: memGroups{w.group.ID: w.group},
		Connections: friends{{w.ada, w.zoe}: true, {w.zoe, w.ada}: true}, Receipts: uploads{w.adaReceipt: w.ada, w.malReceipt: w.mal},
		Catalog: noCategories{}, Users: people, Activity: w.feed, Notifier: quiet{}, Tx: direct{}, Clock: clock.Fixed{At: now},
		Log: slog.New(slog.NewJSONHandler(w.logs, nil)), BaseURL: "http://test"})
	return w
}

func TestLoad(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	for _, tc := range []struct {
		name   string
		actor  uuid.UUID
		e      *Expense
		seen   bool
		member bool
	}{
		{"a member in the group expense", w.ada, w.rent, true, true},
		{"a member outside the group expense", w.cy, w.rent, true, true},
		{"a former member in the group expense", w.ben, w.rent, true, false},
		{"a stranger to the group", w.mal, w.rent, false, false},
		{"someone in an expense outside groups", w.zoe, w.dinner, true, false},
		{"a member of a group, outside the expense", w.cy, w.dinner, false, false},
	} {
		e, g, err := w.svc.load(ctx, tc.actor, tc.e.ID)
		if !tc.seen {
			if !errs.Is(err, errs.KindNotFound) || e != nil {
				t.Errorf("%s: want not found, got %v", tc.name, err)
			}
			continue
		}
		if err != nil || e == nil || e.ID != tc.e.ID {
			t.Errorf("%s: want the expense, got %v", tc.name, err)
			continue
		}
		if (g != nil) != tc.member {
			t.Errorf("%s: group returned %v, want %v", tc.name, g != nil, tc.member)
		}
	}
	if _, _, err := w.svc.load(ctx, w.ada, uuid.New()); !errs.Is(err, errs.KindNotFound) {
		t.Errorf("an unknown id must be not found, got %v", err)
	}
}

func TestAuthorize(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	missing := uuid.New()
	split := func(people ...uuid.UUID) []Share {
		out := make([]Share, 0, len(people))
		for _, p := range people {
			out = append(out, Share{UserID: p, Owed: 100})
		}
		out[0].Paid = int64(100 * len(people))
		return out
	}
	for _, tc := range []struct {
		name    string
		actor   uuid.UUID
		group   *uuid.UUID
		shares  []Share
		receipt *uuid.UUID
		field   string
	}{
		{name: "a member books for others in the group", actor: w.cy, group: &w.group.ID, shares: split(w.ada, w.cy)},
		{name: "a member books between two other members", actor: w.cy, group: &w.group.ID, shares: split(w.ada)},
		{name: "a former member", actor: w.ben, group: &w.group.ID, shares: split(w.ada, w.ben), field: "groupId"},
		{name: "a stranger", actor: w.mal, group: &w.group.ID, shares: split(w.ada), field: "groupId"},
		{name: "a group that does not exist", actor: w.ada, group: &missing, shares: split(w.ada), field: "groupId"},
		{name: "someone who left the group in the split", actor: w.ada, group: &w.group.ID, shares: split(w.ada, w.ben), field: "splits"},
		{name: "friends outside groups", actor: w.ada, shares: split(w.ada, w.zoe)},
		{name: "someone who is not a friend", actor: w.ada, shares: split(w.ada, w.mal), field: "splits"},
		{name: "an expense the actor is not in", actor: w.zoe, shares: split(w.ada), field: "splits"},
		{name: "the actor's own receipt", actor: w.ada, group: &w.group.ID, shares: split(w.ada), receipt: &w.adaReceipt},
		{name: "someone else's receipt", actor: w.ada, group: &w.group.ID, shares: split(w.ada), receipt: &w.malReceipt, field: "receiptId"},
	} {
		_, err := w.svc.authorize(ctx, tc.actor, tc.group, tc.shares, tc.receipt, nil)
		if tc.field == "" {
			if err != nil {
				t.Errorf("%s: %v", tc.name, err)
			}
			continue
		}
		e, ok := errs.As(err)
		if !ok || e.Kind != errs.KindValidation || e.Field != tc.field {
			t.Errorf("%s: want a validation error on %s, got %v", tc.name, tc.field, err)
		}
	}
}

func TestFormerMemberOnlyReads(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	if _, err := w.svc.Get(ctx, w.ben, w.rent.ID); err != nil {
		t.Fatalf("Ben still reads the rent he was part of: %v", err)
	}
	if _, err := w.svc.Comment(ctx, w.ben, w.rent.ID, "Noted"); !errs.Is(err, errs.KindNotFound) {
		t.Errorf("Ben must not comment, got %v", err)
	}
	if err := w.svc.Delete(ctx, w.ben, w.rent.ID); !errs.Is(err, errs.KindNotFound) {
		t.Errorf("Ben must not delete, got %v", err)
	}
	if _, err := w.svc.Restore(ctx, w.ben, w.rent.ID); !errs.Is(err, errs.KindNotFound) {
		t.Errorf("Ben must not restore, got %v", err)
	}
	if len(w.feed.kinds) != 0 || w.expenses[w.rent.ID].DeletedAt != nil {
		t.Fatalf("nothing may change: %v", w.feed.kinds)
	}
	if _, err := w.svc.Comment(ctx, w.cy, w.rent.ID, "Noted"); err != nil {
		t.Fatalf("Cy is a member and comments: %v", err)
	}
	if len(w.feed.kinds) != 1 || w.feed.kinds[0] != activity.CommentCreated {
		t.Fatalf("the comment is in the feed: %v", w.feed.kinds)
	}
}

// TestNotifyReportsLookupFailure checks that an expense is saved when the
// people to notify cannot be looked up, and that the failure is logged.
func TestNotifyReportsLookupFailure(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.svc.users = downUsers{w.svc.users.(memUsers)}
	in := Input{GroupID: &w.group.ID, Description: "Milk", Currency: "EUR", Total: 400, Date: "2026-03-14", Method: MethodEqual,
		Payers: []Payer{{UserID: w.ada, Amount: 400}}, Splits: []SplitInput{{UserID: w.ada}, {UserID: w.cy}}}
	e, err := w.svc.Create(ctx, w.ada, in)
	if err != nil {
		t.Fatalf("the expense is saved anyway: %v", err)
	}
	if got := strings.Count(w.logs.String(), `"msg":"expense.notify.failed"`); got != 1 {
		t.Fatalf("expense.notify.failed logged %d times, want 1: %s", got, w.logs)
	}
	if !strings.Contains(w.logs.String(), e.ID.String()) {
		t.Errorf("the log names the expense: %s", w.logs)
	}
}

// TestCurrencyNeedsMembership checks that an expense left without a currency
// takes the group's only for its members, and that a group the caller cannot
// book into is a mistake on groupId whether or not it exists.
func TestCurrencyNeedsMembership(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	w.group.DefaultCurrency = "JPY"
	missing := uuid.New()
	for _, tc := range []struct {
		name  string
		actor uuid.UUID
		group *uuid.UUID
		want  string
	}{
		{"a member", w.cy, &w.group.ID, "JPY"},
		{"someone outside groups", w.zoe, nil, "EUR"},
		{"a former member", w.ben, &w.group.ID, ""},
		{"a stranger", w.mal, &w.group.ID, ""},
		{"a group that does not exist", w.ada, &missing, ""},
	} {
		got, err := w.svc.currency(ctx, tc.actor, tc.group, "")
		if tc.want != "" {
			if err != nil || got != tc.want {
				t.Errorf("%s: want %s, got %q, %v", tc.name, tc.want, got, err)
			}
			continue
		}
		if e, ok := errs.As(err); !ok || e.Kind != errs.KindValidation || e.Field != "groupId" {
			t.Errorf("%s: want a validation error on groupId, got %q, %v", tc.name, got, err)
		}
	}
	if got, err := w.svc.currency(ctx, w.mal, &w.group.ID, "USD"); err != nil || got != "USD" {
		t.Errorf("a given currency is kept: %q, %v", got, err)
	}
}

func TestSame(t *testing.T) {
	ada, ben := uuid.New(), uuid.New()
	receipt := uuid.New()
	today := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	base := Input{Description: "Rent", Currency: "EUR", Total: 1000, Date: "2026-03-01", Method: MethodExact,
		Payers: []Payer{{UserID: ada, Amount: 1000}}, Splits: []SplitInput{{UserID: ada, Value: "400"}, {UserID: ben, Value: "600"}}}
	n, err := Normalize(base, today)
	if err != nil {
		t.Fatal(err)
	}
	stored := &Expense{Description: n.Description, Category: n.Category, Currency: n.Currency, Total: n.Total, Date: n.Date,
		Method: n.Method, Shares: []Share{n.Shares[1], n.Shares[0]}, Recurrence: n.Recurrence}
	for _, c := range []struct {
		name   string
		change func(in *Input)
		want   bool
	}{
		{"same edit, shares in another order", func(*Input) {}, true},
		{"description", func(in *Input) { in.Description = "Rent, March" }, false},
		{"total", func(in *Input) { in.Total, in.Payers[0].Amount, in.Splits[1].Value = 1100, 1100, "700" }, false},
		{"split", func(in *Input) { in.Splits[0].Value, in.Splits[1].Value = "500", "500" }, false},
		{"date", func(in *Input) { in.Date = "2026-02-28" }, false},
		{"notes", func(in *Input) { in.Notes = "Paid late" }, false},
		{"receipt", func(in *Input) { in.ReceiptID = &receipt }, false},
		{"recurrence", func(in *Input) { in.Recurrence = RecurMonthly }, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := base
			in.Payers = slices.Clone(base.Payers)
			in.Splits = slices.Clone(base.Splits)
			c.change(&in)
			n, err := Normalize(in, today)
			if err != nil {
				t.Fatal(err)
			}
			if got := same(stored, n); got != c.want {
				t.Fatalf("same = %v, want %v", got, c.want)
			}
		})
	}
}
