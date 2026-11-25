package bootstrap_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// house is the flat with everyone the expense rules tell apart: Ada and Ben
// are members, Cy shared the gas bill, settled it and left, Zoe is Ada's
// friend outside the group and Mal knows nobody.
type house struct {
	env                        *bootstraptest.Env
	ada, ben, cy, zoe, mal     uuid.UUID
	group                      *group.View
	rent, gas, dinner, payback *expense.Expense
}

func household(t *testing.T) *house {
	t.Helper()
	ctx := context.Background()
	h := &house{}
	h.env, h.ada, h.ben, h.group = flat(t)
	svc := h.env.Services
	h.mal = h.env.SignIn(t, "mal@example.com", "Mal").User.ID
	h.zoe = h.env.SignIn(t, "zoe@example.com", "Zoe").User.ID
	if _, err := svc.Friends.Add(ctx, h.ada, "zoe@example.com", ""); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Groups.AddMember(ctx, h.ada, h.group.ID, group.Invitee{Email: "cy@example.com", Name: "Cy"})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range v.Members {
		if m != h.ada && m != h.ben {
			h.cy = m
		}
	}
	h.rent = h.book(t, h.ada, bill(&h.group.ID, "Rent", h.ada, h.ada, h.ben))
	h.gas = h.book(t, h.ada, bill(&h.group.ID, "Gas", h.ada, h.ada, h.cy))
	h.dinner = h.book(t, h.ada, bill(nil, "Dinner", h.ada, h.ada, h.zoe))
	if h.payback, err = svc.Expenses.RecordPayment(ctx, h.ada, expense.PaymentInput{GroupID: &h.group.ID, From: h.cy, To: h.ada,
		Amount: 1500, Currency: "EUR", Date: "2026-03-15"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Groups.RemoveMember(ctx, h.ada, h.group.ID, h.cy); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *house) book(t *testing.T, actor uuid.UUID, in expense.Input) *expense.Expense {
	t.Helper()
	e, err := h.env.Services.Expenses.Create(context.Background(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// bill is 30.00 that payer paid, split equally between people.
func bill(groupID *uuid.UUID, description string, payer uuid.UUID, people ...uuid.UUID) expense.Input {
	in := expense.Input{GroupID: groupID, Description: description, Currency: "EUR", Total: 3000, Date: "2026-03-10",
		Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: payer, Amount: 3000}}}
	for _, p := range people {
		in.Splits = append(in.Splits, expense.SplitInput{UserID: p})
	}
	return in
}

// refused fails unless err is a validation error on field.
func refused(t *testing.T, name string, err error, field string) {
	t.Helper()
	if de, ok := errs.As(err); !ok || de.Kind != errs.KindValidation || de.Field != field {
		t.Errorf("%s: want a validation error on %q, got %v", name, field, err)
	}
}

// stored counts the expenses saved with these ids.
func stored(t *testing.T, env *bootstraptest.Env, ids ...uuid.UUID) int {
	t.Helper()
	var n int
	if err := env.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM expenses WHERE id = ANY($1)`, ids).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateAuthorization(t *testing.T) {
	ctx := context.Background()
	h := household(t)
	svc := h.env.Services
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	mine, err := svc.Receipts.Upload(ctx, h.ada, png)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := svc.Receipts.Upload(ctx, h.ben, png)
	if err != nil {
		t.Fatal(err)
	}
	unknown := uuid.New()
	with := func(in expense.Input, receipt uuid.UUID) expense.Input {
		in.ReceiptID = &receipt
		return in
	}
	g := &h.group.ID
	for _, tc := range []struct {
		name  string
		actor uuid.UUID
		in    expense.Input
		field string
	}{
		{"a member books a group expense", h.ada, bill(g, "Bread", h.ben, h.ada, h.ben), ""},
		{"an outsider books in the group", h.mal, bill(g, "Bread", h.mal, h.mal), "groupId"},
		{"a former member books in the group", h.cy, bill(g, "Bread", h.cy, h.cy), "groupId"},
		{"an outsider puts members in the group's expense", h.mal, bill(g, "Bread", h.ada, h.ada, h.ben), "groupId"},
		{"a group that does not exist", h.ada, bill(&unknown, "Bread", h.ada, h.ada), "groupId"},
		{"a friend outside the group takes part", h.ada, bill(g, "Bread", h.ada, h.ada, h.zoe), "splits"},
		{"a friend outside the group pays", h.ada, bill(g, "Bread", h.zoe, h.ada, h.ben), "splits"},
		{"a former member takes part", h.ada, bill(g, "Bread", h.ada, h.ada, h.cy), "splits"},
		{"a friend outside groups", h.ada, bill(nil, "Lunch", h.zoe, h.ada, h.zoe), ""},
		{"a group-mate outside groups", h.ben, bill(nil, "Lunch", h.ben, h.ada, h.ben), ""},
		{"a stranger outside groups", h.ada, bill(nil, "Lunch", h.ada, h.ada, h.mal), "splits"},
		{"a stranger pays outside groups", h.ada, bill(nil, "Lunch", h.mal, h.ada), "splits"},
		{"a friend's debt to someone they do not know", h.ada, bill(nil, "Lunch", h.zoe, h.ada, h.ben), "splits"},
		{"booking for others without taking part", h.ada, bill(nil, "Lunch", h.zoe, h.zoe), "splits"},
		{"a receipt of one's own", h.ada, with(bill(g, "Shop", h.ada, h.ada, h.ben), mine.ID), ""},
		{"a receipt someone else uploaded", h.ada, with(bill(g, "Shop", h.ada, h.ada, h.ben), theirs.ID), "receiptId"},
		{"a receipt that does not exist", h.ada, with(bill(g, "Shop", h.ada, h.ada, h.ben), unknown), "receiptId"},
	} {
		id := uuid.New()
		tc.in.ID = &id
		_, err := svc.Expenses.Create(ctx, tc.actor, tc.in)
		if tc.field == "" {
			if err != nil {
				t.Errorf("%s: %v", tc.name, err)
			}
			continue
		}
		refused(t, tc.name, err, tc.field)
		if n := stored(t, h.env, id); n != 0 {
			t.Errorf("%s: the refused expense was saved", tc.name)
		}
	}
}

// TestExpenseAccess checks who may read, comment on, edit, delete and restore
// each kind of expense. Everyone else gets the answer an unknown id gets, and
// the expense stays as it was.
func TestExpenseAccess(t *testing.T) {
	ctx := context.Background()
	h := household(t)
	svc := h.env.Services.Expenses
	people := map[string]uuid.UUID{"Ada": h.ada, "Ben": h.ben, "Cy": h.cy, "Zoe": h.zoe, "Mal": h.mal}
	for _, tc := range []struct {
		name    string
		e       *expense.Expense
		readers []string
		editors []string
		// edit and remove are what editors get from Update and Delete: gas
		// still bills Cy, who has left, so nobody may change or delete it.
		edit, remove errs.Kind
	}{
		{name: "rent, shared by the members", e: h.rent, readers: []string{"Ada", "Ben"}, editors: []string{"Ada", "Ben"}},
		{name: "gas, shared with Cy before Cy left", e: h.gas, readers: []string{"Ada", "Ben", "Cy"}, editors: []string{"Ada", "Ben"},
			edit: errs.KindValidation, remove: errs.KindConflict},
		{name: "dinner, outside groups", e: h.dinner, readers: []string{"Ada", "Zoe"}, editors: []string{"Ada", "Zoe"}},
	} {
		in := bill(tc.e.GroupID, tc.e.Description, h.ada, tc.e.Participants()...)
		for who, actor := range people {
			label := tc.name + ": " + who
			expect := func(op string, err error, kind errs.Kind) {
				t.Helper()
				if kind == "" && err != nil {
					t.Errorf("%s %s: %v", label, op, err)
				} else if kind != "" && !errs.Is(err, kind) {
					t.Errorf("%s %s: want %s, got %v", label, op, kind, err)
				}
			}
			read, edit, remove, other := errs.Kind(""), tc.edit, tc.remove, errs.Kind("")
			if !slices.Contains(tc.readers, who) {
				read = errs.KindNotFound
			}
			if !slices.Contains(tc.editors, who) {
				edit, remove, other = errs.KindNotFound, errs.KindNotFound, errs.KindNotFound
			}
			_, err := svc.Get(ctx, actor, tc.e.ID)
			expect("reads", err, read)
			_, _, err = svc.Comments(ctx, actor, tc.e.ID, 0, 0)
			expect("reads comments", err, read)
			_, err = svc.Comment(ctx, actor, tc.e.ID, "Noted")
			expect("comments", err, other)
			_, err = svc.Update(ctx, actor, tc.e.ID, in)
			expect("edits", err, edit)
			expect("deletes", svc.Delete(ctx, actor, tc.e.ID), remove)
			_, err = svc.Restore(ctx, actor, tc.e.ID)
			expect("restores", err, other)
		}
		got, err := svc.Get(ctx, h.ada, tc.e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.DeletedAt != nil || got.Total != tc.e.Total || len(got.Shares) != len(tc.e.Shares) {
			t.Errorf("%s: the expense changed: %+v", tc.name, got)
		}
		if _, n, err := svc.Comments(ctx, h.ada, tc.e.ID, 0, 0); err != nil || n != len(tc.editors) {
			t.Errorf("%s: %d comments, want %d (%v)", tc.name, n, len(tc.editors), err)
		}
	}
	unknown := uuid.New()
	if _, err := svc.Get(ctx, h.ada, unknown); !errs.Is(err, errs.KindNotFound) {
		t.Errorf("reading an expense that does not exist: want not found, got %v", err)
	}
	if err := svc.Delete(ctx, h.ada, unknown); !errs.Is(err, errs.KindNotFound) {
		t.Errorf("deleting an expense that does not exist: want not found, got %v", err)
	}
}

func TestPaymentRules(t *testing.T) {
	ctx := context.Background()
	h := household(t)
	svc := h.env.Services.Expenses
	g := &h.group.ID
	pay := func(groupID *uuid.UUID, from, to uuid.UUID, amount int64) expense.PaymentInput {
		return expense.PaymentInput{GroupID: groupID, From: from, To: to, Amount: amount, Currency: "EUR", Date: "2026-03-15"}
	}
	for _, tc := range []struct {
		name  string
		actor uuid.UUID
		in    expense.PaymentInput
		field string
	}{
		{"a member records a payment in the group", h.ben, pay(g, h.ben, h.ada, 500), ""},
		{"a member records another member's payment", h.ada, pay(g, h.ben, h.ada, 500), ""},
		{"paying oneself", h.ada, pay(nil, h.ada, h.ada, 500), "toUserId"},
		{"paying nobody", h.ada, pay(nil, h.ada, uuid.Nil, 500), "toUserId"},
		{"paying nothing", h.ada, pay(nil, h.ada, h.zoe, 0), "amount"},
		{"paying a negative amount", h.ada, pay(nil, h.ada, h.zoe, -500), "amount"},
		{"an outsider records in the group", h.mal, pay(g, h.ada, h.ben, 500), "groupId"},
		{"a former member records in the group", h.cy, pay(g, h.cy, h.ada, 500), "groupId"},
		{"a payment in the group to someone outside it", h.ada, pay(g, h.ada, h.zoe, 500), "splits"},
		{"a payment in the group from a former member", h.ada, pay(g, h.cy, h.ada, 500), "splits"},
		{"a payment to a friend outside groups", h.zoe, pay(nil, h.zoe, h.ada, 500), ""},
		{"a payment between two others outside groups", h.ada, pay(nil, h.ben, h.zoe, 500), "fromUserId"},
		{"a payment to a stranger outside groups", h.ada, pay(nil, h.ada, h.mal, 500), "splits"},
		{"a payment to someone who does not exist", h.ada, pay(nil, h.ada, uuid.New(), 500), "splits"},
	} {
		id := uuid.New()
		tc.in.ID = &id
		p, err := svc.RecordPayment(ctx, tc.actor, tc.in)
		if tc.field == "" {
			if err != nil {
				t.Errorf("%s: %v", tc.name, err)
				continue
			}
			if p.Kind != expense.KindPayment || p.Net(tc.in.From) != tc.in.Amount || p.Net(tc.in.To) != -tc.in.Amount {
				t.Errorf("%s: booked %+v", tc.name, p)
			}
			continue
		}
		refused(t, tc.name, err, tc.field)
		if n := stored(t, h.env, id); n != 0 {
			t.Errorf("%s: the refused payment was saved", tc.name)
		}
	}
}

// TestRetriedCreate checks that a create sent again with the same id returns
// the first expense, and that an id cannot be taken over for anything else.
func TestRetriedCreate(t *testing.T) {
	ctx := context.Background()
	h := household(t)
	svc := h.env.Services.Expenses
	id := uuid.New()
	first := bill(&h.group.ID, "Bread", h.ada, h.ada, h.ben)
	first.ID = &id
	e, err := svc.Create(ctx, h.ada, first)
	if err != nil {
		t.Fatal(err)
	}
	again := first
	again.Total, again.Payers = 6000, []expense.Payer{{UserID: h.ada, Amount: 6000}}
	for name, actor := range map[string]uuid.UUID{"the same person": h.ada, "another member": h.ben} {
		got, err := svc.Create(ctx, actor, again)
		if err != nil {
			t.Fatalf("%s retrying: %v", name, err)
		}
		if got.ID != e.ID || got.Total != e.Total {
			t.Errorf("%s retrying got %+v, want the first expense", name, got)
		}
	}

	outside := first
	outside.GroupID, outside.Splits = nil, []expense.SplitInput{{UserID: h.ada}, {UserID: h.zoe}}
	other := bill(&h.group.ID, "Bread", h.mal, h.mal)
	other.ID = &id
	payment := expense.PaymentInput{ID: &id, GroupID: &h.group.ID, From: h.ben, To: h.ada, Amount: 3000,
		Currency: "EUR", Date: "2026-03-15"}
	for name, err := range map[string]error{
		"someone who cannot see it": func() error { _, err := svc.Create(ctx, h.mal, other); return err }(),
		"a former member":           func() error { _, err := svc.Create(ctx, h.cy, first); return err }(),
		"outside the group":         func() error { _, err := svc.Create(ctx, h.ada, outside); return err }(),
		"a payment":                 func() error { _, err := svc.RecordPayment(ctx, h.ada, payment); return err }(),
	} {
		if !errs.Is(err, errs.KindConflict) {
			t.Errorf("reusing the id for %s: want a conflict, got %v", name, err)
		}
	}
	got, err := svc.Get(ctx, h.ada, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != expense.KindExpense || got.GroupID == nil || got.Total != 3000 || got.CreatedBy != h.ada {
		t.Errorf("the expense changed: %+v", got)
	}

	pid := uuid.New()
	payment.ID = &pid
	p, err := svc.RecordPayment(ctx, h.ada, payment)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.RecordPayment(ctx, h.ada, payment); err != nil || got.ID != p.ID {
		t.Errorf("retrying a payment got %+v, %v", got, err)
	}
	reuse := bill(&h.group.ID, "Bread", h.ada, h.ada, h.ben)
	reuse.ID = &pid
	if _, err := svc.Create(ctx, h.ada, reuse); !errs.Is(err, errs.KindConflict) {
		t.Errorf("reusing a payment's id for an expense: want a conflict, got %v", err)
	}
}

func TestListVisibility(t *testing.T) {
	ctx := context.Background()
	h := household(t)
	svc := h.env.Services.Expenses
	g := &h.group.ID
	juice := h.book(t, h.ben, bill(g, "Juice 100% orange", h.ben, h.ben)).ID
	litre := h.book(t, h.ben, bill(g, "Juice 1000 ml", h.ben, h.ben)).ID
	box := h.book(t, h.ada, bill(g, "Box a_b", h.ada, h.ada)).ID
	boxes := h.book(t, h.ada, bill(g, "Box axb", h.ada, h.ada)).ID
	if err := svc.Delete(ctx, h.ada, h.rent.ID); err != nil {
		t.Fatal(err)
	}
	zoe, ben := h.zoe, h.ben
	live := []uuid.UUID{h.gas.ID, h.payback.ID, juice, litre, box, boxes}
	for _, tc := range []struct {
		name string
		f    expense.Filter
		want []uuid.UUID
	}{
		{"an outsider", expense.Filter{UserID: h.mal}, nil},
		{"a former member sees what they took part in", expense.Filter{UserID: h.cy}, []uuid.UUID{h.gas.ID, h.payback.ID}},
		{"a friend outside the group", expense.Filter{UserID: h.zoe}, []uuid.UUID{h.dinner.ID}},
		{"a member sees the group's live expenses", expense.Filter{UserID: h.ben, GroupID: g}, live},
		{"with deleted expenses", expense.Filter{UserID: h.ben, GroupID: g, IncludeDeleted: true}, append([]uuid.UUID{h.rent.ID}, live...)},
		{"with a friend", expense.Filter{UserID: h.ada, FriendID: &zoe}, []uuid.UUID{h.dinner.ID}},
		{"with a group-mate", expense.Filter{UserID: h.ada, FriendID: &ben, IncludeDeleted: true}, []uuid.UUID{h.rent.ID}},
		{"with someone one shares nothing with", expense.Filter{UserID: h.zoe, FriendID: &ben}, nil},
		{"a percent sign is matched literally", expense.Filter{UserID: h.ada, Query: "100%"}, []uuid.UUID{juice}},
		{"an underscore is matched literally", expense.Filter{UserID: h.ada, Query: "a_b"}, []uuid.UUID{box}},
		{"a backslash is matched literally", expense.Filter{UserID: h.ada, Query: `\`}, nil},
		{"payments only", expense.Filter{UserID: h.ada, Kind: expense.KindPayment}, []uuid.UUID{h.payback.ID}},
	} {
		tc.f.Limit = 100
		list, total, err := svc.List(ctx, tc.f)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		got := make([]uuid.UUID, len(list))
		for i, e := range list {
			got[i] = e.ID
		}
		slices.SortFunc(got, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		want := slices.Clone(tc.want)
		slices.SortFunc(want, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		if !slices.Equal(got, want) || total != len(want) {
			t.Errorf("%s: got %v (total %d), want %v", tc.name, got, total, want)
		}
	}
	for name, actor := range map[string]uuid.UUID{"an outsider": h.mal, "a former member": h.cy} {
		if _, _, err := svc.List(ctx, expense.Filter{UserID: actor, GroupID: g, Limit: 100}); !errs.Is(err, errs.KindNotFound) {
			t.Errorf("%s listing the group: want not found, got %v", name, err)
		}
	}
}
