package routes

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/stats"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

func toInput(in platform.ExpenseInput) (expense.Input, error) {
	out := expense.Input{Description: in.Description, Currency: in.Currency, Total: in.Total, Date: in.Date, Category: in.Category,
		Notes: in.Notes, Method: expense.Method(in.SplitMethod), Tax: in.Tax, Tip: in.Tip, Recurrence: expense.Recurrence(in.Recurrence)}
	var err error
	if out.ID, err = optionalID(in.ID, "id"); err != nil {
		return out, err
	}
	if out.GroupID, err = optionalID(in.GroupID, "groupId"); err != nil {
		return out, err
	}
	if out.ReceiptID, err = optionalID(in.ReceiptID, "receiptId"); err != nil {
		return out, err
	}
	if in.UpdatedAt != nil && *in.UpdatedAt != "" {
		at, err := time.Parse(time.RFC3339, *in.UpdatedAt)
		if err != nil {
			return out, errs.Validation("updatedAt", "Use the updatedAt the expense came with.")
		}
		out.UpdatedAt = &at
	}
	for i, p := range in.Payers {
		id, err := requiredID(p.UserID, fmt.Sprintf("payers[%d].userId", i))
		if err != nil {
			return out, err
		}
		out.Payers = append(out.Payers, expense.Payer{UserID: id, Amount: p.Amount})
	}
	for i, s := range in.Splits {
		id, err := requiredID(s.UserID, fmt.Sprintf("splits[%d].userId", i))
		if err != nil {
			return out, err
		}
		v := ""
		if s.Value != nil {
			v = s.Value.String()
		}
		out.Splits = append(out.Splits, expense.SplitInput{UserID: id, Value: v})
	}
	for i, it := range in.Items {
		item := expense.Item{Name: it.Name, Amount: it.Amount}
		for j, raw := range it.UserIDs {
			id, err := requiredID(raw, fmt.Sprintf("items[%d].userIds[%d]", i, j))
			if err != nil {
				return out, err
			}
			item.UserIDs = append(item.UserIDs, id)
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (a *API) listExpenses(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	f := expense.Filter{UserID: me(r), Query: q.Get("q"), Category: q.Get("category"), Kind: expense.Kind(q.Get("kind"))}
	f.IncludeDeleted, _ = strconv.ParseBool(q.Get("includeDeleted"))
	f.Limit, f.Offset = page(r)
	var err error
	if f.GroupID, err = queryID(r, "groupId"); err != nil {
		return err
	}
	if f.FriendID, err = queryID(r, "friendId"); err != nil {
		return err
	}
	if f.From, err = queryDate(r, "from"); err != nil {
		return err
	}
	if f.To, err = queryDate(r, "to"); err != nil {
		return err
	}
	items, total, err := a.svc.Expenses.List(r.Context(), f)
	if err != nil {
		return err
	}
	out := make([]platform.Expense, 0, len(items))
	for _, e := range items {
		out = append(out, toExpense(e, f.UserID))
	}
	return list(w, out, total)
}

func (a *API) createExpense(w http.ResponseWriter, r *http.Request) error {
	body, err := decode[platform.ExpenseInput](w, r)
	if err != nil {
		return err
	}
	in, err := toInput(body)
	if err != nil {
		return err
	}
	e, err := a.svc.Expenses.Create(r.Context(), me(r), in)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, toExpense(e, me(r)))
}

func (a *API) getExpense(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "expenseId", "Expense")
	if err != nil {
		return err
	}
	e, err := a.svc.Expenses.Get(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toExpense(e, me(r)))
}

func (a *API) updateExpense(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "expenseId", "Expense")
	if err != nil {
		return err
	}
	body, err := decode[platform.ExpenseInput](w, r)
	if err != nil {
		return err
	}
	in, err := toInput(body)
	if err != nil {
		return err
	}
	e, err := a.svc.Expenses.Update(r.Context(), me(r), id, in)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toExpense(e, me(r)))
}

func (a *API) deleteExpense(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "expenseId", "Expense")
	if err != nil {
		return err
	}
	if err := a.svc.Expenses.Delete(r.Context(), me(r), id); err != nil {
		return err
	}
	return noContent(w)
}

func (a *API) restoreExpense(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "expenseId", "Expense")
	if err != nil {
		return err
	}
	e, err := a.svc.Expenses.Restore(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, toExpense(e, me(r)))
}

func (a *API) listComments(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "expenseId", "Expense")
	if err != nil {
		return err
	}
	limit, offset := window(r)
	comments, total, err := a.svc.Expenses.Comments(r.Context(), me(r), id, limit, offset)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.UserID)
	}
	people, err := a.svc.Users.Index(r.Context(), ids)
	if err != nil {
		return err
	}
	out := make([]platform.Comment, 0, len(comments))
	for _, c := range comments {
		u := platform.User{ID: c.UserID.String(), Name: "Someone"}
		if p := people[c.UserID]; p != nil {
			u = a.toUser(p)
		}
		out = append(out, platform.Comment{ID: c.ID.String(), ExpenseID: c.ExpenseID.String(), User: u, Body: c.Body, CreatedAt: stamp(c.CreatedAt)})
	}
	return list(w, out, total)
}

func (a *API) addComment(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "expenseId", "Expense")
	if err != nil {
		return err
	}
	in, err := decode[platform.CommentInput](w, r)
	if err != nil {
		return err
	}
	c, err := a.svc.Expenses.Comment(r.Context(), me(r), id, in.Body)
	if err != nil {
		return err
	}
	u, err := a.svc.Users.Get(r.Context(), c.UserID)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, platform.Comment{ID: c.ID.String(), ExpenseID: c.ExpenseID.String(), User: a.toUser(u), Body: c.Body, CreatedAt: stamp(c.CreatedAt)})
}

func (a *API) deleteComment(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "commentId", "Comment")
	if err != nil {
		return err
	}
	if err := a.svc.Expenses.DeleteComment(r.Context(), me(r), id); err != nil {
		return err
	}
	return noContent(w)
}

func (a *API) createPayment(w http.ResponseWriter, r *http.Request) error {
	body, err := decode[platform.PaymentInput](w, r)
	if err != nil {
		return err
	}
	in := expense.PaymentInput{Amount: body.Amount, Currency: body.Currency, Date: body.Date, Notes: body.Notes}
	if in.ID, err = optionalID(body.ID, "id"); err != nil {
		return err
	}
	if in.GroupID, err = optionalID(body.GroupID, "groupId"); err != nil {
		return err
	}
	if in.From, err = requiredID(body.FromUserID, "fromUserId"); err != nil {
		return err
	}
	if in.To, err = requiredID(body.ToUserID, "toUserId"); err != nil {
		return err
	}
	e, err := a.svc.Expenses.RecordPayment(r.Context(), me(r), in)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, toExpense(e, me(r)))
}

func (a *API) exportGroup(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "groupId", "Group")
	if err != nil {
		return err
	}
	view, err := a.svc.Groups.View(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	// The expenses are read twice, a page at a time, so a large group is
	// never held in memory: once for the people who need a column, once to
	// write the rows.
	seen := map[uuid.UUID]bool{}
	ids := []uuid.UUID{}
	err = a.pages(r, id, func(e *expense.Expense) error {
		for _, p := range e.Participants() {
			if !seen[p] {
				seen[p] = true
				ids = append(ids, p)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	people, err := a.svc.Users.Index(r.Context(), ids)
	if err != nil {
		return err
	}
	names := map[uuid.UUID]string{}
	for id, u := range people {
		names[id] = u.Name
	}
	order := []uuid.UUID{}
	for _, u := range view.People {
		order = append(order, u.ID)
	}
	for id := range names {
		if !view.HasMember(id) {
			order = append(order, id)
		}
	}

	filename := strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 32 {
			return '_'
		}
		return r
	}, view.Name) + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	cw := csv.NewWriter(w)
	header := []string{"Date", "Description", "Category", "Kind", "Currency", "Cost"}
	for _, id := range order {
		header = append(header, safeCell(names[id]))
	}
	_ = cw.Write(header)
	err = a.pages(r, id, func(e *expense.Expense) error {
		c, _ := money.Lookup(e.Currency)
		row := []string{e.Date.Format(time.DateOnly), safeCell(e.Description), safeCell(e.Category), string(e.Kind), e.Currency, decimalString(e.Total, c.Exponent)}
		for _, id := range order {
			row = append(row, decimalString(e.Net(id), c.Exponent))
		}
		return cw.Write(row)
	})
	cw.Flush()
	if err != nil {
		return err
	}
	return cw.Error()
}

// exportPage is how many expenses an export reads at a time.
const exportPage = 200

// pages calls fn with each of the group's expenses, oldest first, reading
// them a page at a time from the end of the newest-first list.
func (a *API) pages(r *http.Request, groupID uuid.UUID, fn func(*expense.Expense) error) error {
	f := expense.Filter{UserID: me(r), GroupID: &groupID, Limit: 1}
	_, total, err := a.svc.Expenses.List(r.Context(), f)
	if err != nil {
		return err
	}
	for end := total; end > 0; end -= exportPage {
		f.Offset = max(0, end-exportPage)
		f.Limit = end - f.Offset
		items, _, err := a.svc.Expenses.List(r.Context(), f)
		if err != nil {
			return err
		}
		for i := len(items) - 1; i >= 0; i-- {
			if err := fn(items[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// safeCell neutralises spreadsheet formula injection in text a person
// typed: names, descriptions and categories. Amount cells never pass through
// it, since a negative amount must stay a number. The on-device export
// (LocalGroupsRepository.export in packages/client) follows the same rule.
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func decimalString(amount int64, exponent int) string {
	neg := amount < 0
	if neg {
		amount = -amount
	}
	digits := strconv.FormatInt(amount, 10)
	if exponent > 0 {
		for len(digits) <= exponent {
			digits = "0" + digits
		}
		digits = digits[:len(digits)-exponent] + "." + digits[len(digits)-exponent:]
	}
	if neg {
		digits = "-" + digits
	}
	return digits
}

func (a *API) listActivity(w http.ResponseWriter, r *http.Request) error {
	f := activity.Filter{UserID: me(r)}
	f.Limit, f.Offset = page(r)
	var err error
	if f.GroupID, err = queryID(r, "groupId"); err != nil {
		return err
	}
	if f.ExpenseID, err = queryID(r, "expenseId"); err != nil {
		return err
	}
	items, total, err := a.svc.Activity.List(r.Context(), f)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ActorID)
	}
	people, err := a.svc.Users.Index(r.Context(), ids)
	if err != nil {
		return err
	}
	out := make([]platform.Activity, 0, len(items))
	for _, it := range items {
		out = append(out, a.toActivity(it, people[it.ActorID]))
	}
	return list(w, out, total)
}

func (a *API) stats(w http.ResponseWriter, r *http.Request) error {
	q := stats.Query{UserID: me(r), Currency: r.URL.Query().Get("currency")}
	var err error
	if q.GroupID, err = queryID(r, "groupId"); err != nil {
		return err
	}
	from, err := queryDate(r, "from")
	if err != nil {
		return err
	}
	to, err := queryDate(r, "to")
	if err != nil {
		return err
	}
	if from != nil {
		q.From = *from
	}
	if to != nil {
		q.To = *to
	}
	s, err := a.svc.Stats.Summary(r.Context(), q)
	if err != nil {
		return err
	}
	out := platform.Stats{Currency: s.Currency, TotalSpent: s.Total, ByCategory: []platform.CategoryAmount{}, ByMonth: []platform.MonthAmount{}, ByMember: []platform.MemberSpend{}}
	for _, c := range s.ByCategory {
		out.ByCategory = append(out.ByCategory, platform.CategoryAmount{Category: c.Key, Name: c.Name, Amount: c.Amount})
	}
	for _, m := range s.ByMonth {
		out.ByMonth = append(out.ByMonth, platform.MonthAmount{Month: m.Key, Amount: m.Amount})
	}
	for _, m := range s.ByMember {
		out.ByMember = append(out.ByMember, platform.MemberSpend{UserID: m.UserID.String(), Paid: m.Paid, Owed: m.Owed})
	}
	return details(w, http.StatusOK, out)
}
