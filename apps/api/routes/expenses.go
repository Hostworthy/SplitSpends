package routes

import (
	"encoding/csv"
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

func parseID(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errs.Validation(field, "Invalid user id.")
	}
	return id, nil
}

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
	for _, p := range in.Payers {
		id, err := parseID(p.UserID, "payers")
		if err != nil {
			return out, err
		}
		out.Payers = append(out.Payers, expense.Payer{UserID: id, Amount: p.Amount})
	}
	for _, s := range in.Splits {
		id, err := parseID(s.UserID, "splits")
		if err != nil {
			return out, err
		}
		v := ""
		if s.Value != nil {
			v = s.Value.String()
		}
		out.Splits = append(out.Splits, expense.SplitInput{UserID: id, Value: v})
	}
	for _, it := range in.Items {
		item := expense.Item{Name: it.Name, Amount: it.Amount}
		for _, raw := range it.UserIDs {
			id, err := parseID(raw, "items")
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
	body, err := decode[platform.ExpenseInput](r)
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
	body, err := decode[platform.ExpenseInput](r)
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
	comments, err := a.svc.Expenses.Comments(r.Context(), me(r), id)
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
			u = toUser(p)
		}
		out = append(out, platform.Comment{ID: c.ID.String(), ExpenseID: c.ExpenseID.String(), User: u, Body: c.Body, CreatedAt: stamp(c.CreatedAt)})
	}
	return list(w, out, len(out))
}

func (a *API) addComment(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "expenseId", "Expense")
	if err != nil {
		return err
	}
	in, err := decode[platform.CommentInput](r)
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
	return details(w, http.StatusCreated, platform.Comment{ID: c.ID.String(), ExpenseID: c.ExpenseID.String(), User: toUser(u), Body: c.Body, CreatedAt: stamp(c.CreatedAt)})
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
	body, err := decode[platform.PaymentInput](r)
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
	if in.From, err = parseID(body.FromUserID, "fromUserId"); err != nil {
		return err
	}
	if in.To, err = parseID(body.ToUserID, "toUserId"); err != nil {
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
	var all []*expense.Expense
	for offset := 0; ; offset += 200 {
		items, total, err := a.svc.Expenses.List(r.Context(), expense.Filter{UserID: me(r), GroupID: &id, Limit: 200, Offset: offset})
		if err != nil {
			return err
		}
		all = append(all, items...)
		if offset+200 >= total {
			break
		}
	}
	names := map[uuid.UUID]string{}
	ids := []uuid.UUID{}
	for _, e := range all {
		ids = append(ids, e.Participants()...)
	}
	people, err := a.svc.Users.Index(r.Context(), ids)
	if err != nil {
		return err
	}
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
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		c, _ := money.Lookup(e.Currency)
		row := []string{e.Date.Format(time.DateOnly), safeCell(e.Description), safeCell(e.Category), string(e.Kind), e.Currency, decimalString(e.Total, c.Exponent)}
		for _, id := range order {
			row = append(row, decimalString(e.Net(id), c.Exponent))
		}
		_ = cw.Write(row)
	}
	cw.Flush()
	return cw.Error()
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
		out = append(out, toActivity(it, people[it.ActorID]))
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
