package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

const instructions = `Split Spends tracks shared expenses between friends and groups, like Splitwise.
Start with get_overview to learn who the user is, their groups, friends and balances.
Ids are UUIDs; always look them up with list_groups or list_friends instead of guessing.
Amounts you send are decimal strings in the currency's normal units ("12.50"). Amounts you read are already formatted.
Positive balances mean the user is owed money; negative means the user owes.
Confirm with the user before deleting anything.
Tools starting with admin_ work only for administrators (managers can list and decide requests); the user's role is in get_overview.`

// NewServer builds an MCP server bound to one API client.
func NewServer(c *Client, version string) *sdk.Server {
	return build(version, slog.Default(), func(*sdk.CallToolRequest) *Client { return c })
}

// build registers every tool on a new server. client picks the API client
// for each call.
func build(version string, log *slog.Logger, client func(*sdk.CallToolRequest) *Client) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "splitspends", Title: "Split Spends", Version: version}, &sdk.ServerOptions{Instructions: instructions})
	ts := &toolset{server: s, client: client, log: log}

	readOnly := &sdk.ToolAnnotations{ReadOnlyHint: true}
	add(ts, "get_overview", "Who the user is, their total balances, groups and friends with balances.", readOnly, (*tools).overview)
	add(ts, "list_groups", "List the user's groups with ids, members and the user's balance in each.", readOnly, (*tools).listGroups)
	add(ts, "get_group", "Group details: members, every member's balance, suggested settle-up payments and recent expenses.", readOnly, (*tools).getGroup)
	add(ts, "list_friends", "List friends with ids, emails and the balance with each.", readOnly, (*tools).listFriends)
	add(ts, "list_expenses", "Search expenses and payments, newest first. Filter by group, friend, text or dates.", readOnly, (*tools).listExpenses)
	add(ts, "get_expense", "Full details of one expense, including who paid and who owes what.", readOnly, (*tools).getExpense)
	add(ts, "list_activity", "Recent activity feed.", readOnly, (*tools).listActivity)
	add(ts, "list_categories", "Expense category ids for add_expense: the built-in ones plus the user's own custom categories.", readOnly, (*tools).listCategories)
	add(ts, "add_expense", "Add a shared expense. Defaults: paid by the user, split equally between the user and the participants given (or, when a group is given and no participants or split are listed, the group's default split, else every group member equally).", nil, (*tools).addExpense)
	add(ts, "record_payment", "Record that one person paid another back (settle up).", nil, (*tools).recordPayment)
	add(ts, "remind_to_settle", "Email someone who owes the user money a friendly reminder to settle up, in one group or across everything. At most one reminder per person and group every 24 hours.", nil, (*tools).remind)
	destructive := true
	add(ts, "delete_expense", "Delete an expense or payment. It can be restored from the app.", &sdk.ToolAnnotations{DestructiveHint: &destructive}, (*tools).deleteExpense)
	admin(ts)
	return s
}

// toolset is what tools are registered with: the server, how a call finds
// its API client, and where failures are logged.
type toolset struct {
	server *sdk.Server
	client func(*sdk.CallToolRequest) *Client
	log    *slog.Logger
}

// add registers a tool whose body runs on tools bound to the caller's
// client. The SDK runs tool calls on goroutines of its own, outside any
// HTTP recovery, so a panic is recovered here and becomes a tool error
// instead of ending the process.
func add[In any](ts *toolset, name, desc string, annotations *sdk.ToolAnnotations, h func(*tools, context.Context, *sdk.CallToolRequest, In) (*sdk.CallToolResult, any, error)) {
	tool := &sdk.Tool{Name: name, Description: desc, Annotations: annotations}
	sdk.AddTool(ts.server, tool, func(ctx context.Context, req *sdk.CallToolRequest, in In) (res *sdk.CallToolResult, out any, err error) {
		defer func() {
			if p := recover(); p != nil {
				ts.log.ErrorContext(ctx, "mcp.tool.panicked", "tool", name, "panic", p, "stack", string(debug.Stack()))
				res, out, err = fail(errors.New("something went wrong in this tool; the server logged the details"))
			}
		}()
		return h(&tools{c: ts.client(req)}, ctx, req, in)
	})
}

// HTTPHandler serves MCP over streamable HTTP. A request without a bearer
// token is answered 401 before any MCP work; the tools of one server, built
// once, call the API at apiBase with each request's own token.
func HTTPHandler(apiBase, version string, hc *http.Client, log *slog.Logger) http.Handler {
	base := NewClient(apiBase, "", hc)
	server := build(version, log, func(req *sdk.CallToolRequest) *Client {
		if req == nil || req.Extra == nil {
			return base
		}
		return base.with(bearer(req.Extra.Header))
	})
	h := stream(func(*http.Request) *sdk.Server { return server }, log)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bearer(r.Header) == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(platform.Error{Error: "Send an API token as a bearer token. Create one in Split Spends under Settings, API tokens.", Code: "UNAUTHORIZED"})
			return
		}
		h.ServeHTTP(w, r)
	})
}

// bearer returns the token of a Bearer authorization header, or "".
func bearer(h http.Header) string {
	scheme, token, ok := strings.Cut(h.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// stream serves the servers server picks as stateless streamable HTTP.
func stream(server func(*http.Request) *sdk.Server, log *slog.Logger) http.Handler {
	return sdk.NewStreamableHTTPHandler(server, &sdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		Logger:       log,
		// Every call needs a bearer token (no cookies), so DNS rebinding cannot
		// borrow credentials; this lets the server sit behind a local proxy.
		DisableLocalhostProtection: true,
	})
}

type tools struct{ c *Client }

func text(s string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: s}}}
}

func fail(err error) (*sdk.CallToolResult, any, error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
		err = errors.New("not signed in: create an API token in Split Spends under Settings, API tokens, and configure it as the bearer token")
	}
	res := text("Error: " + err.Error())
	res.IsError = true
	return res, nil, nil
}

// people indexes names by id from friends and groups.
type people map[string]string

func (p people) name(id string) string {
	if n, ok := p[id]; ok {
		return n
	}
	return id
}

func (t *tools) index(ctx context.Context) (platform.Me, people, error) {
	me, err := get[platform.Me](ctx, t.c, "/me", nil)
	if err != nil {
		return me, nil, err
	}
	idx := people{me.ID: me.Name + " (you)"}
	friends, err := getList[platform.Friend](ctx, t.c, "/friends", nil)
	if err != nil {
		return me, nil, err
	}
	for _, f := range friends.Items {
		idx[f.User.ID] = f.User.Name
	}
	return me, idx, nil
}

func (t *tools) balances(ctx context.Context, bs []platform.Balance) string {
	if len(bs) == 0 {
		return "settled up"
	}
	parts := make([]string, 0, len(bs))
	for _, b := range bs {
		parts = append(parts, t.c.format(ctx, b.Amount, b.Currency))
	}
	return strings.Join(parts, ", ")
}

type empty struct{}

func (t *tools) overview(ctx context.Context, _ *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
	me, _, err := t.index(ctx)
	if err != nil {
		return fail(err)
	}
	totals, err := get[platform.MyBalances](ctx, t.c, "/me/balances", nil)
	if err != nil {
		return fail(err)
	}
	groups, err := getList[platform.Group](ctx, t.c, "/groups", nil)
	if err != nil {
		return fail(err)
	}
	friends, err := getList[platform.Friend](ctx, t.c, "/friends", nil)
	if err != nil {
		return fail(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s <%s> (id %s), default currency %s, role %s.\n", me.Name, email(me.Email), me.ID, me.DefaultCurrency, me.Role)
	fmt.Fprintf(&b, "Net balance: %s. Owed to you: %s. You owe: %s.\n\nGroups:\n", t.balances(ctx, totals.Balances), t.balances(ctx, totals.Owed), t.balances(ctx, totals.Owing))
	for _, g := range groups.Items {
		fmt.Fprintf(&b, "- %s %s (id %s, %d members): %s\n", g.Emoji, g.Name, g.ID, len(g.Members), t.balances(ctx, g.Balances))
	}
	b.WriteString("\nFriends:\n")
	for _, f := range friends.Items {
		fmt.Fprintf(&b, "- %s <%s> (id %s): %s\n", f.User.Name, email(f.User.Email), f.User.ID, t.balances(ctx, f.Balances))
	}
	return text(b.String()), nil, nil
}

func (t *tools) listGroups(ctx context.Context, _ *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
	groups, err := getList[platform.Group](ctx, t.c, "/groups", nil)
	if err != nil {
		return fail(err)
	}
	var b strings.Builder
	for _, g := range groups.Items {
		names := make([]string, 0, len(g.Members))
		for _, m := range g.Members {
			names = append(names, m.Name+" ("+m.ID+")")
		}
		fmt.Fprintf(&b, "%s %s\n  id: %s, currency %s, simplify debts %v\n  members: %s\n  your balance: %s\n",
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
		return fail(err)
	}
	sheet, err := get[platform.GroupBalances](ctx, t.c, path+"/balances", nil)
	if err != nil {
		return fail(err)
	}
	recent, err := getList[platform.Expense](ctx, t.c, "/expenses", url.Values{"groupId": {g.ID}, "limit": {"10"}})
	if err != nil {
		return fail(err)
	}
	idx := people{}
	for _, m := range g.Members {
		idx[m.ID] = m.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s (id %s), default currency %s, simplify debts %v\n\nBalances:\n", g.Emoji, g.Name, g.ID, g.DefaultCurrency, g.SimplifyDebts)
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
		return fail(err)
	}
	var b strings.Builder
	for _, f := range friends.Items {
		fmt.Fprintf(&b, "- %s <%s> (id %s): %s\n", f.User.Name, email(f.User.Email), f.User.ID, t.balances(ctx, f.Balances))
	}
	if b.Len() == 0 {
		return text("No friends yet."), nil, nil
	}
	return text(b.String()), nil, nil
}

func (t *tools) line(ctx context.Context, e platform.Expense, idx people) string {
	payer := ""
	var best int64
	for _, s := range e.Shares {
		if s.Paid > best {
			best, payer = s.Paid, s.UserID
		}
	}
	var mine string
	switch {
	case e.MyBalance > 0:
		mine = "you lent " + t.c.format(ctx, e.MyBalance, e.Currency)
	case e.MyBalance < 0:
		mine = "you borrowed " + t.c.format(ctx, -e.MyBalance, e.Currency)
	default:
		mine = "not involved or even"
	}
	if e.Kind == "payment" {
		to := ""
		for _, s := range e.Shares {
			if s.Owed > 0 {
				to = s.UserID
			}
		}
		return fmt.Sprintf("- %s payment: %s paid %s %s (id %s)\n", e.Date, idx.name(payer), idx.name(to), t.c.format(ctx, e.Total, e.Currency), e.ID)
	}
	return fmt.Sprintf("- %s %s: %s paid by %s, %s [%s] (id %s)\n", e.Date, e.Description, t.c.format(ctx, e.Total, e.Currency), idx.name(payer), mine, e.Category, e.ID)
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
		return fail(err)
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
		return fail(err)
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
		return fail(err)
	}
	e, err := get[platform.Expense](ctx, t.c, "/expenses/"+url.PathEscape(in.ExpenseID), nil)
	if err != nil {
		return fail(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s), %s on %s, category %s, split %s\n", e.Description, e.Kind, t.c.format(ctx, e.Total, e.Currency), e.Date, e.Category, e.SplitMethod)
	if e.GroupID != nil {
		fmt.Fprintf(&b, "Group id: %s\n", *e.GroupID)
	}
	if e.Notes != "" {
		fmt.Fprintf(&b, "Notes: %s\n", e.Notes)
	}
	for _, s := range e.Shares {
		fmt.Fprintf(&b, "- %s paid %s, owes %s\n", idx.name(s.UserID), t.c.format(ctx, s.Paid, e.Currency), t.c.format(ctx, s.Owed, e.Currency))
	}
	for _, it := range e.Items {
		names := make([]string, len(it.UserIDs))
		for i, id := range it.UserIDs {
			names[i] = idx.name(id)
		}
		fmt.Fprintf(&b, "  item %s %s: %s\n", it.Name, t.c.format(ctx, it.Amount, e.Currency), strings.Join(names, ", "))
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
		return fail(err)
	}
	var b strings.Builder
	for _, a := range res.Items {
		fmt.Fprintf(&b, "- %s %s\n", day(a.CreatedAt), a.Summary)
	}
	return text(b.String()), nil, nil
}

func (t *tools) listCategories(ctx context.Context, _ *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
	res, err := getList[platform.CategoryEntry](ctx, t.c, "/categories", nil)
	if err != nil {
		return fail(err)
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
		return fail(err)
	}
	currency := strings.ToUpper(in.Currency)
	var members []string
	var preset *platform.DefaultSplit
	if in.GroupID != "" {
		g, err := get[platform.Group](ctx, t.c, "/groups/"+url.PathEscape(in.GroupID), nil)
		if err != nil {
			return fail(err)
		}
		if currency == "" {
			currency = g.DefaultCurrency
		}
		preset = g.DefaultSplit
		for _, m := range g.Members {
			members = append(members, m.ID)
			idx[m.ID] = m.Name
		}
	}
	if currency == "" {
		currency = me.DefaultCurrency
	}
	total, err := t.c.minor(ctx, in.Amount, currency)
	if err != nil {
		return fail(err)
	}
	payer := in.PaidBy
	if payer == "" {
		payer = me.ID
	}
	body := platform.ExpenseInput{Description: in.Description, Currency: currency, Total: total, Date: in.Date, Category: in.Category,
		Notes: in.Notes, SplitMethod: "equal", Payers: []platform.Payer{{UserID: payer, Amount: total}}}
	if in.GroupID != "" {
		body.GroupID = &in.GroupID
	}
	method := strings.ToLower(in.SplitMethod)
	if method == "" && len(in.Participants) == 0 && len(in.SplitValues) == 0 && preset != nil && preset.Method != "equal" {
		// The group's default split (for example rent 60/40) applies when
		// the assistant gives no split of its own.
		method = preset.Method
		for _, id := range sortedKeys(preset.Values) {
			if v := preset.Values[id]; strings.Trim(v, "0.") != "" {
				in.SplitValues = append(in.SplitValues, splitValue{UserID: id, Value: v})
			}
		}
	}
	switch method {
	case "", "equal":
		ids := in.Participants
		if len(ids) == 0 {
			ids = members
		}
		if in.GroupID == "" {
			ids = append([]string{me.ID}, ids...)
		}
		for _, id := range unique(ids) {
			body.Splits = append(body.Splits, platform.Split{UserID: id})
		}
	case "exact", "percent", "shares":
		body.SplitMethod = method
		for _, v := range in.SplitValues {
			value := strings.TrimSpace(v.Value)
			if !plain(value) {
				return fail(fmt.Errorf("the split value %q for %s is not a decimal number such as 60 or 33.5", v.Value, v.UserID))
			}
			if method == "exact" {
				m, err := t.c.minor(ctx, v.Value, currency)
				if err != nil {
					return fail(err)
				}
				value = strconv.FormatInt(m, 10)
			}
			n := json.Number(value)
			body.Splits = append(body.Splits, platform.Split{UserID: v.UserID, Value: &n})
		}
	default:
		return fail(fmt.Errorf("splitMethod must be equal, exact, percent or shares"))
	}
	e, err := send[platform.Expense](ctx, t.c, http.MethodPost, "/expenses", body)
	if err != nil {
		return fail(err)
	}
	return text("Added.\n" + t.line(ctx, e, idx)), nil, nil
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
		return fail(err)
	}
	currency := strings.ToUpper(in.Currency)
	if currency == "" && in.GroupID != "" {
		g, err := get[platform.Group](ctx, t.c, "/groups/"+url.PathEscape(in.GroupID), nil)
		if err != nil {
			return fail(err)
		}
		currency = g.DefaultCurrency
		for _, m := range g.Members {
			idx[m.ID] = m.Name
		}
	}
	if currency == "" {
		currency = me.DefaultCurrency
	}
	amount, err := t.c.minor(ctx, in.Amount, currency)
	if err != nil {
		return fail(err)
	}
	body := platform.PaymentInput{FromUserID: in.FromUserID, ToUserID: in.ToUserID, Amount: amount, Currency: currency, Notes: in.Notes}
	if in.GroupID != "" {
		body.GroupID = &in.GroupID
	}
	e, err := send[platform.Expense](ctx, t.c, http.MethodPost, "/payments", body)
	if err != nil {
		return fail(err)
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
		return fail(err)
	}
	body := platform.ReminderInput{UserID: in.UserID}
	if in.GroupID != "" {
		body.GroupID = &in.GroupID
	}
	r, err := send[platform.Reminder](ctx, t.c, http.MethodPost, "/reminders", body)
	if err != nil {
		return fail(err)
	}
	out := "Reminded " + idx.name(r.UserID) + " that they owe you " + t.balances(ctx, r.Balances) + "."
	if !r.Emailed {
		out += " They turned reminder emails off, so they will only see it in their activity feed."
	}
	return text(out), nil, nil
}

func (t *tools) deleteExpense(ctx context.Context, _ *sdk.CallToolRequest, in expenseArgs) (*sdk.CallToolResult, any, error) {
	if _, err := t.c.do(ctx, http.MethodDelete, "/expenses/"+url.PathEscape(in.ExpenseID), nil, nil); err != nil {
		return fail(err)
	}
	return text("Deleted expense " + in.ExpenseID + ". It can be restored from the app."), nil, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unique(list []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// day is the date part of an RFC 3339 timestamp, or ts itself when it is
// too short to have one.
func day(ts string) string {
	if len(ts) < len("2006-01-02") {
		return ts
	}
	return ts[:len("2006-01-02")]
}

func email(e *string) string {
	if e == nil {
		return "no email"
	}
	return *e
}
