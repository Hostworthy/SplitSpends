package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func (t *tools) index(ctx context.Context) (platform.Me, people, error) {
	me, err := get[platform.Me](ctx, t.c, "/me", nil)
	if err != nil {
		return me, nil, err
	}
	idx := people{me.ID: strconv.Quote(me.Name) + " (you)"}
	friends, err := getList[platform.Friend](ctx, t.c, "/friends", nil)
	if err != nil {
		return me, nil, err
	}
	for _, f := range friends.Items {
		idx[f.User.ID] = strconv.Quote(f.User.Name)
	}
	return me, idx, nil
}

type empty struct{}

func (t *tools) overview(ctx context.Context, _ *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
	me, err := get[platform.Me](ctx, t.c, "/me", nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	totals, err := get[platform.MyBalances](ctx, t.c, "/me/balances", nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	groups, err := getList[platform.Group](ctx, t.c, "/groups", nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	friends, err := getList[platform.Friend](ctx, t.c, "/friends", nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are %q <%s> (id %s), default currency %s, role %s.\n", me.Name, email(me.Email), me.ID, me.DefaultCurrency, me.Role)
	fmt.Fprintf(&b, "Net balance: %s. Owed to you: %s. You owe: %s.\n\nGroups:\n", t.balances(ctx, totals.Balances), t.balances(ctx, totals.Owed), t.balances(ctx, totals.Owing))
	for _, g := range groups.Items {
		fmt.Fprintf(&b, "- %s %q (id %s, %d members): %s\n", g.Emoji, g.Name, g.ID, len(g.Members), t.balances(ctx, g.Balances))
	}
	b.WriteString("\nFriends:\n")
	for _, f := range friends.Items {
		fmt.Fprintf(&b, "- %q <%s> (id %s): %s\n", f.User.Name, email(f.User.Email), f.User.ID, t.balances(ctx, f.Balances))
	}
	return text(b.String()), nil, nil
}

func (t *tools) listGroups(ctx context.Context, _ *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
	groups, err := getList[platform.Group](ctx, t.c, "/groups", nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	var b strings.Builder
	for _, g := range groups.Items {
		names := make([]string, 0, len(g.Members))
		for _, m := range g.Members {
			names = append(names, strconv.Quote(m.Name)+" ("+m.ID+")")
		}
		fmt.Fprintf(&b, "%s %q\n  id: %s, currency %s, simplify debts %v\n  members: %s\n  your balance: %s\n",
			g.Emoji, g.Name, g.ID, g.DefaultCurrency, g.SimplifyDebts, strings.Join(names, ", "), t.balances(ctx, g.Balances))
	}
	if b.Len() == 0 {
		return text("No groups yet."), nil, nil
	}
	return text(b.String()), nil, nil
}

type groupArgs struct {
	GroupID string `json:"groupId" jsonschema:"the group id"`
}

func (t *tools) getGroup(ctx context.Context, _ *sdk.CallToolRequest, in groupArgs) (*sdk.CallToolResult, any, error) {
	path := "/groups/" + url.PathEscape(in.GroupID)
	g, err := get[platform.Group](ctx, t.c, path, nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	sheet, err := get[platform.GroupBalances](ctx, t.c, path+"/balances", nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	recent, err := getList[platform.Expense](ctx, t.c, "/expenses", url.Values{"groupId": {g.ID}, "limit": {"10"}})
	if err != nil {
		return t.fail(ctx, err)
	}
	idx := people{}
	for _, m := range g.Members {
		idx[m.ID] = strconv.Quote(m.Name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %q (id %s), default currency %s, simplify debts %v\n\nBalances:\n", g.Emoji, g.Name, g.ID, g.DefaultCurrency, g.SimplifyDebts)
	for _, m := range sheet.Members {
		fmt.Fprintf(&b, "- %s (%s): %s\n", idx.name(m.UserID), m.UserID, t.balances(ctx, m.Balances))
	}
	b.WriteString("\nTo settle up:\n")
	if len(sheet.Debts) == 0 {
		b.WriteString("- everyone is settled up\n")
	}
	for _, d := range sheet.Debts {
		fmt.Fprintf(&b, "- %s pays %s %s\n", idx.name(d.From), idx.name(d.To), t.c.format(ctx, d.Amount, d.Currency))
	}
	b.WriteString("\nRecent expenses:\n")
	for _, e := range recent.Items {
		b.WriteString(t.line(ctx, e, idx))
	}
	return text(b.String()), nil, nil
}

func (t *tools) listFriends(ctx context.Context, _ *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
	friends, err := getList[platform.Friend](ctx, t.c, "/friends", nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	var b strings.Builder
	for _, f := range friends.Items {
		fmt.Fprintf(&b, "- %q <%s> (id %s): %s\n", f.User.Name, email(f.User.Email), f.User.ID, t.balances(ctx, f.Balances))
	}
	if b.Len() == 0 {
		return text("No friends yet."), nil, nil
	}
	return text(b.String()), nil, nil
}

type listArgs struct {
	GroupID  string `json:"groupId,omitempty" jsonschema:"only expenses in this group"`
	FriendID string `json:"friendId,omitempty" jsonschema:"only expenses shared with this friend"`
	Query    string `json:"query,omitempty" jsonschema:"text to search in descriptions and notes"`
	From     string `json:"from,omitempty" jsonschema:"start date YYYY-MM-DD"`
	To       string `json:"to,omitempty" jsonschema:"end date YYYY-MM-DD"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum results, default 25, at most 200"`
}

func (t *tools) listExpenses(ctx context.Context, _ *sdk.CallToolRequest, in listArgs) (*sdk.CallToolResult, any, error) {
	_, idx, err := t.index(ctx)
	if err != nil {
		return t.fail(ctx, err)
	}
	q := url.Values{}
	for k, v := range map[string]string{"groupId": in.GroupID, "friendId": in.FriendID, "q": in.Query, "from": in.From, "to": in.To} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if in.Limit <= 0 {
		in.Limit = 25
	}
	q.Set("limit", strconv.Itoa(min(in.Limit, 200)))
	res, err := getList[platform.Expense](ctx, t.c, "/expenses", q)
	if err != nil {
		return t.fail(ctx, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d of %d matching expenses:\n", len(res.Items), res.Total)
	for _, e := range res.Items {
		b.WriteString(t.line(ctx, e, idx))
	}
	return text(b.String()), nil, nil
}

type expenseArgs struct {
	ExpenseID string `json:"expenseId" jsonschema:"the expense id"`
}

func (t *tools) getExpense(ctx context.Context, _ *sdk.CallToolRequest, in expenseArgs) (*sdk.CallToolResult, any, error) {
	_, idx, err := t.index(ctx)
	if err != nil {
		return t.fail(ctx, err)
	}
	e, err := get[platform.Expense](ctx, t.c, "/expenses/"+url.PathEscape(in.ExpenseID), nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q (%s), %s on %s, category %s, split %s\n", e.Description, e.Kind, t.c.format(ctx, e.Total, e.Currency), e.Date, e.Category, e.SplitMethod)
	if e.GroupID != nil {
		fmt.Fprintf(&b, "Group id: %s\n", *e.GroupID)
	}
	if e.Notes != "" {
		fmt.Fprintf(&b, "Notes: %q\n", e.Notes)
	}
	for _, s := range e.Shares {
		fmt.Fprintf(&b, "- %s paid %s, owes %s\n", idx.name(s.UserID), t.c.format(ctx, s.Paid, e.Currency), t.c.format(ctx, s.Owed, e.Currency))
	}
	for _, it := range e.Items {
		names := make([]string, len(it.UserIDs))
		for i, id := range it.UserIDs {
			names[i] = idx.name(id)
		}
		fmt.Fprintf(&b, "  item %q %s: %s\n", it.Name, t.c.format(ctx, it.Amount, e.Currency), strings.Join(names, ", "))
	}
	if e.Deleted {
		b.WriteString("This expense is deleted.\n")
	}
	return text(b.String()), nil, nil
}

type activityArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum events, default 20"`
}

func (t *tools) listActivity(ctx context.Context, _ *sdk.CallToolRequest, in activityArgs) (*sdk.CallToolResult, any, error) {
	if in.Limit <= 0 {
		in.Limit = 20
	}
	res, err := getList[platform.Activity](ctx, t.c, "/activity", url.Values{"limit": {strconv.Itoa(min(in.Limit, 200))}})
	if err != nil {
		return t.fail(ctx, err)
	}
	var b strings.Builder
	for _, a := range res.Items {
		fmt.Fprintf(&b, "- %s %q\n", day(a.CreatedAt), a.Summary)
	}
	return text(b.String()), nil, nil
}

func (t *tools) listCategories(ctx context.Context, _ *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
	res, err := getList[platform.CategoryEntry](ctx, t.c, "/categories", nil)
	if err != nil {
		return t.fail(ctx, err)
	}
	groups := map[string][]string{}
	for _, c := range res.Items {
		if c.ID == "payment" || (c.Custom && !c.Mine) {
			continue
		}
		label := c.ID + " (" + c.Name + ")"
		if c.Custom {
			label = c.ID + " (" + c.Name + ", custom)"
		}
		groups[c.Group] = append(groups[c.Group], label)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s: %s\n", k, strings.Join(groups[k], ", "))
	}
	return text(b.String()), nil, nil
}

type splitValue struct {
	UserID string `json:"userId"`
	Value  string `json:"value" jsonschema:"for exact: the amount as a decimal string; for percent: the percentage; for shares: the number of shares"`
}

type addArgs struct {
	Description  string       `json:"description" jsonschema:"short title, for example Dinner at Lua"`
	Amount       string       `json:"amount" jsonschema:"total as a decimal string, for example 60 or 12.50"`
	Currency     string       `json:"currency,omitempty" jsonschema:"ISO code; defaults to the group's or the user's currency"`
	GroupID      string       `json:"groupId,omitempty" jsonschema:"group id; omit for an expense between friends"`
	PaidBy       string       `json:"paidBy,omitempty" jsonschema:"user id of the payer; defaults to the user"`
	Participants []string     `json:"participants,omitempty" jsonschema:"user ids sharing the cost for an equal split; the user is included automatically outside groups"`
	SplitMethod  string       `json:"splitMethod,omitempty" jsonschema:"equal (default), exact, percent or shares"`
	SplitValues  []splitValue `json:"splitValues,omitempty" jsonschema:"per-person values for exact, percent or shares splits"`
	Date         string       `json:"date,omitempty" jsonschema:"YYYY-MM-DD, defaults to today"`
	Category     string       `json:"category,omitempty" jsonschema:"category id from list_categories"`
	Notes        string       `json:"notes,omitempty"`
}

func (t *tools) addExpense(ctx context.Context, _ *sdk.CallToolRequest, in addArgs) (*sdk.CallToolResult, any, error) {
	me, idx, err := t.index(ctx)
	if err != nil {
		return t.fail(ctx, err)
	}
	g, err := t.group(ctx, in.GroupID, idx)
	if err != nil {
		return t.fail(ctx, err)
	}
	var members []string
	var preset *platform.DefaultSplit
	if g != nil {
		preset = g.DefaultSplit
		for _, m := range g.Members {
			members = append(members, m.ID)
		}
	}
	cur, err := t.c.unit(ctx, currency(in.Currency, g, me))
	if err != nil {
		return t.fail(ctx, err)
	}
	total, err := cur.minor(in.Amount)
	if err != nil {
		return t.fail(ctx, err)
	}
	method, shares, err := splits(in, me.ID, members, preset, cur)
	if err != nil {
		return t.fail(ctx, err)
	}
	payer := in.PaidBy
	if payer == "" {
		payer = me.ID
	}
	body := platform.ExpenseInput{Description: in.Description, Currency: cur.code, Total: total, Date: in.Date, Category: in.Category,
		Notes: in.Notes, SplitMethod: method, Payers: []platform.Payer{{UserID: payer, Amount: total}}, Splits: shares}
	if in.GroupID != "" {
		body.GroupID = &in.GroupID
	}
	e, err := send[platform.Expense](ctx, t.c, http.MethodPost, "/expenses", body)
	if err != nil {
		return t.fail(ctx, err)
	}
	return text("Added.\n" + t.line(ctx, e, idx)), nil, nil
}

// group fetches the group with id, if one is given, and adds its members to
// idx so that entries in it name everyone.
func (t *tools) group(ctx context.Context, id string, idx people) (*platform.Group, error) {
	if id == "" {
		return nil, nil
	}
	g, err := get[platform.Group](ctx, t.c, "/groups/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	for _, m := range g.Members {
		idx[m.ID] = strconv.Quote(m.Name)
	}
	return &g, nil
}

type paymentArgs struct {
	FromUserID string `json:"fromUserId" jsonschema:"who paid the money"`
	ToUserID   string `json:"toUserId" jsonschema:"who received the money"`
	Amount     string `json:"amount" jsonschema:"decimal string, for example 25.00"`
	Currency   string `json:"currency,omitempty" jsonschema:"ISO code; defaults to the group's or the user's currency"`
	GroupID    string `json:"groupId,omitempty" jsonschema:"group the payment settles, if any"`
	Notes      string `json:"notes,omitempty"`
}

func (t *tools) recordPayment(ctx context.Context, _ *sdk.CallToolRequest, in paymentArgs) (*sdk.CallToolResult, any, error) {
	me, idx, err := t.index(ctx)
	if err != nil {
		return t.fail(ctx, err)
	}
	g, err := t.group(ctx, in.GroupID, idx)
	if err != nil {
		return t.fail(ctx, err)
	}
	code := currency(in.Currency, g, me)
	amount, err := t.c.minor(ctx, in.Amount, code)
	if err != nil {
		return t.fail(ctx, err)
	}
	body := platform.PaymentInput{FromUserID: in.FromUserID, ToUserID: in.ToUserID, Amount: amount, Currency: code, Notes: in.Notes}
	if in.GroupID != "" {
		body.GroupID = &in.GroupID
	}
	e, err := send[platform.Expense](ctx, t.c, http.MethodPost, "/payments", body)
	if err != nil {
		return t.fail(ctx, err)
	}
	return text("Recorded.\n" + t.line(ctx, e, idx)), nil, nil
}

type remindArgs struct {
	UserID  string `json:"userId" jsonschema:"the person who owes the user money"`
	GroupID string `json:"groupId,omitempty" jsonschema:"remind about this group only; omit for everything they owe"`
}

func (t *tools) remind(ctx context.Context, _ *sdk.CallToolRequest, in remindArgs) (*sdk.CallToolResult, any, error) {
	_, idx, err := t.index(ctx)
	if err != nil {
		return t.fail(ctx, err)
	}
	body := platform.ReminderInput{UserID: in.UserID}
	if in.GroupID != "" {
		body.GroupID = &in.GroupID
	}
	r, err := send[platform.Reminder](ctx, t.c, http.MethodPost, "/reminders", body)
	if err != nil {
		return t.fail(ctx, err)
	}
	out := "Reminded " + idx.name(r.UserID) + " that they owe you " + t.balances(ctx, r.Balances) + "."
	if !r.Emailed {
		out += " They turned reminder emails off, so they will only see it in their activity feed."
	}
	return text(out), nil, nil
}

func (t *tools) deleteExpense(ctx context.Context, _ *sdk.CallToolRequest, in expenseArgs) (*sdk.CallToolResult, any, error) {
	if _, err := t.c.do(ctx, http.MethodDelete, "/expenses/"+url.PathEscape(in.ExpenseID), nil, nil); err != nil {
		return t.fail(ctx, err)
	}
	return text("Deleted expense " + in.ExpenseID + ". It can be restored from the app."), nil, nil
}
