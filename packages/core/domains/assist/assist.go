// Package assist turns the user's instructions, and optionally a receipt
// photo, into an expense draft using the AI provider the host configured.
// The model runs outside SplitSpends; this package builds its context and
// checks everything it returns. Drafts are never saved: the user reviews them.
package assist

import (
	"context"
	"io"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/friend"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/rates"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/receipt"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// MaxText bounds the instructions sent to the provider.
const MaxText = 1000

// Request is what the user asked for.
type Request struct {
	Text      string
	GroupID   *uuid.UUID
	ReceiptID *uuid.UUID
}

// Draft is a reviewable expense proposal.
type Draft struct {
	GroupID     *uuid.UUID
	ReceiptID   *uuid.UUID
	Description string
	Currency    string
	Total       int64
	Date        string
	Category    string
	Method      expense.Method
	Payers      []expense.Payer
	Splits      []expense.SplitInput
	Items       []expense.Item
	Tax         int64
	Tip         int64
	Warnings    []string
}

// Service builds drafts.
type Service struct {
	reader   ports.ReceiptReader
	users    *user.Service
	groups   *group.Service
	friends  friend.Repository
	receipts *receipt.Service
	rates    *rates.Service
	clock    clock.Clock
}

// NewService builds a Service. reader may be nil when AI is disabled.
func NewService(reader ports.ReceiptReader, users *user.Service, groups *group.Service, friends friend.Repository, receipts *receipt.Service, r *rates.Service, c clock.Clock) *Service {
	return &Service{reader: reader, users: users, groups: groups, friends: friends, receipts: receipts, rates: r, clock: c}
}

// known is everything about the user that the model may refer to.
type known struct {
	me      *user.User
	people  map[uuid.UUID]*user.User
	groups  map[uuid.UUID]*group.Group
	friends map[uuid.UUID]bool
}

func (s *Service) gather(ctx context.Context, actor uuid.UUID, only *uuid.UUID) (*known, error) {
	me, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	k := &known{me: me, people: map[uuid.UUID]*user.User{}, groups: map[uuid.UUID]*group.Group{}, friends: map[uuid.UUID]bool{}}
	var list []*group.Group
	if only != nil {
		g, err := s.groups.Member(ctx, actor, *only)
		if err != nil {
			return nil, err
		}
		list = []*group.Group{g}
	} else if list, err = s.groups.Mine(ctx, actor); err != nil {
		return nil, err
	}
	ids := []uuid.UUID{}
	for _, g := range list {
		k.groups[g.ID] = g
		ids = append(ids, g.Members...)
	}
	if only == nil {
		friends, err := s.friends.List(ctx, actor)
		if err != nil {
			return nil, err
		}
		for _, id := range friends {
			k.friends[id] = true
		}
		ids = append(ids, friends...)
	}
	people, err := s.users.List(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, u := range people {
		if u.ID != actor {
			k.people[u.ID] = u
		}
	}
	return k, nil
}

func (k *known) input(text, today, currency string) ports.DraftInput {
	in := ports.DraftInput{Text: text, Today: today, Currency: currency,
		Me: ports.Person{ID: k.me.ID.String(), Name: k.me.Name, Email: k.me.Email}, Categories: expense.CategoryIDs()}
	for _, u := range k.people {
		in.People = append(in.People, ports.Person{ID: u.ID.String(), Name: u.Name, Email: u.Email})
	}
	for _, g := range k.groups {
		opt := ports.GroupOption{ID: g.ID.String(), Name: g.Name, Currency: g.DefaultCurrency}
		for _, m := range g.Members {
			opt.MemberIDs = append(opt.MemberIDs, m.String())
		}
		in.Groups = append(in.Groups, opt)
	}
	return in
}

// Draft asks the provider to read the instructions (and receipt, if any).
func (s *Service) Draft(ctx context.Context, actor uuid.UUID, req Request) (*Draft, error) {
	if s.reader == nil {
		return nil, errs.Unavailable("AI features are not enabled on this server.")
	}
	req.Text = strings.TrimSpace(req.Text)
	if utf8.RuneCountInString(req.Text) > MaxText {
		return nil, errs.Validation("text", "Keep instructions under 1000 characters.")
	}
	if req.Text == "" && req.ReceiptID == nil {
		return nil, errs.Validation("text", "Describe the expense or add a receipt.")
	}
	k, err := s.gather(ctx, actor, req.GroupID)
	if err != nil {
		return nil, err
	}

	tz, err := time.LoadLocation(k.me.Timezone)
	if err != nil {
		tz = time.UTC
	}
	today := s.clock.Now().In(tz).Format(time.DateOnly)
	currency := k.me.DefaultCurrency
	if req.GroupID != nil {
		currency = k.groups[*req.GroupID].DefaultCurrency
	}
	in := k.input(req.Text, today, currency)
	if req.GroupID != nil {
		in.GroupID = req.GroupID.String()
	}
	if req.ReceiptID != nil {
		r, body, err := s.receipts.Open(ctx, actor, *req.ReceiptID)
		if err != nil {
			return nil, err
		}
		in.Image, err = io.ReadAll(io.LimitReader(body, receipt.MaxSize+1))
		body.Close()
		if err != nil {
			return nil, err
		}
		in.ContentType = r.ContentType
	}

	ext, err := s.reader.DraftExpense(ctx, in)
	if err != nil {
		return nil, err
	}
	table := func() *rates.Table {
		if s.rates == nil {
			return nil
		}
		t, err := s.rates.Latest(ctx)
		if err != nil {
			return nil
		}
		return t
	}
	return build(k, req, ext, today, currency, table)
}

// build turns the model's answer into a valid draft, dropping anything it
// made up and explaining what changed in Warnings.
//
// Expenses always use the group's currency (or the user's own outside
// groups). When the model reads amounts in another currency, typically from
// a receipt abroad, they are converted here with today's rates from table,
// exactly and line by line, rather than trusting the model's arithmetic.
func build(k *known, req Request, ext ports.DraftExtract, today, currency string, table func() *rates.Table) (*Draft, error) {
	d := &Draft{ReceiptID: req.ReceiptID, Description: strings.TrimSpace(ext.Description), Date: today,
		Category: expense.DefaultCategory, Method: expense.MethodEqual}
	warn := func(msg string) { d.Warnings = append(d.Warnings, msg) }

	// Group: what the user picked wins, then a group the model chose.
	var g *group.Group
	if req.GroupID != nil {
		g = k.groups[*req.GroupID]
	} else if id, err := uuid.Parse(ext.GroupID); err == nil && k.groups[id] != nil {
		g = k.groups[id]
	}
	if g != nil {
		d.GroupID = &g.ID
		currency = g.DefaultCurrency
	}
	d.Currency = currency
	source := currency
	if c, ok := money.Lookup(ext.Currency); ok {
		source = c.Code
	}
	from, _ := money.Lookup(source)
	var rate *rates.Table
	if source != currency {
		rate = table()
		if rate == nil || !rate.Has(source) || !rate.Has(currency) {
			return nil, errs.NoRates("These amounts are in " + source + ", and exchange rates are not available right now to convert them to " +
				currency + ". Try again later, or enter the expense yourself.")
		}
	}
	amount := func(s string) int64 {
		v, ok := expense.ParseDecimal(s, from.Exponent)
		if !ok || v < 0 {
			return 0
		}
		if rate != nil {
			v, _ = rate.Convert(rates.Amount{Currency: source, Minor: v}, currency)
		}
		return v
	}
	if _, err := time.Parse(time.DateOnly, ext.Date); err == nil {
		d.Date = ext.Date
	}
	if expense.KnownCategory(ext.Category) && ext.Category != expense.PaymentCategory {
		d.Category = ext.Category
	}
	if d.Description == "" {
		d.Description = "Expense"
	}

	// Who may take part: group members, or you and your friends.
	allowed := func(id uuid.UUID) bool {
		if id == k.me.ID {
			return true
		}
		if g != nil {
			return g.HasMember(id)
		}
		return k.friends[id]
	}
	dropped := map[uuid.UUID]bool{}
	resolve := func(raw []string) []uuid.UUID {
		out := []uuid.UUID{}
		seen := map[uuid.UUID]bool{}
		for _, r := range raw {
			id, err := uuid.Parse(strings.TrimSpace(r))
			if err != nil || seen[id] {
				continue
			}
			if !allowed(id) {
				if u := k.people[id]; u != nil && !dropped[id] && g != nil {
					warn(u.Name + " is not in " + g.Name + ", so they were left out.")
				}
				dropped[id] = true
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
		return out
	}
	participants := resolve(ext.ParticipantIDs)
	if len(participants) == 0 {
		if g != nil {
			participants = append(participants, g.Members...)
		} else {
			participants = []uuid.UUID{k.me.ID}
		}
	}

	if len(ext.Items) > 0 {
		d.Method = expense.MethodItemized
		var sum int64
		for _, it := range ext.Items {
			a := amount(it.Amount)
			if a == 0 {
				continue
			}
			ids := resolve(it.ParticipantIDs)
			if len(ids) == 0 {
				ids = participants
			}
			name := strings.TrimSpace(it.Name)
			if name == "" {
				name = "Item"
			}
			d.Items = append(d.Items, expense.Item{Name: name, Amount: a, UserIDs: ids})
			sum += a
		}
		d.Tax, d.Tip = amount(ext.Tax), amount(ext.Tip)
		d.Total = sum + d.Tax + d.Tip
		if want := amount(ext.Amount); want > 0 && want != d.Total && len(d.Items) > 0 {
			reconcile(d, want)
		}
		if len(d.Items) == 0 {
			d.Method = expense.MethodEqual
			warn("Could not read any items, so the total is split equally.")
		}
	}
	if d.Method == expense.MethodEqual {
		d.Total = amount(ext.Amount)
		for _, id := range participants {
			d.Splits = append(d.Splits, expense.SplitInput{UserID: id})
		}
	}
	if d.Total == 0 {
		warn("Could not find an amount.")
	}

	payer := k.me.ID
	if id, err := uuid.Parse(ext.PayerID); err == nil && allowed(id) {
		payer = id
	}
	d.Payers = []expense.Payer{{UserID: payer, Amount: d.Total}}

	for _, name := range ext.Unmatched {
		if name = strings.TrimSpace(name); name != "" {
			warn("Could not match “" + name + "” to anyone you know.")
		}
	}
	if rate != nil {
		original, _ := expense.ParseDecimal(ext.Amount, from.Exponent)
		note := "Converted from " + source + " to " + currency + " at 1 " + source + " = " + unitRate(rate, source, currency) + " " + currency
		if !rate.Published.IsZero() {
			note += " (rates of " + rate.Published.Format("2 Jan 2006") + ")"
		}
		if original > 0 {
			note += "; the receipt total was " + money.Format(original, source)
		}
		warn(note + ".")
	}
	return d, nil
}

// unitRate is how much one unit of from buys in to, to four decimals.
func unitRate(t *rates.Table, from, to string) string {
	rf, _ := t.Rate(from)
	rt, _ := t.Rate(to)
	return new(big.Rat).Quo(rt, rf).FloatString(4)
}

// reconcile makes items + tax + tip equal the receipt total: a gap goes to
// tax, and a negative remainder (an order discount) is spread over items.
func reconcile(d *Draft, total int64) {
	var items int64
	for _, it := range d.Items {
		items += it.Amount
	}
	d.Tax = total - items - d.Tip
	if d.Tax < 0 {
		weights := make([]*big.Rat, len(d.Items))
		for i, it := range d.Items {
			weights[i] = big.NewRat(it.Amount, 1)
		}
		if parts, err := money.Allocate(d.Tax, weights); err == nil {
			for i := range d.Items {
				d.Items[i].Amount = max(d.Items[i].Amount+parts[i], 1)
			}
		}
		d.Tax = 0
	}
	d.Total = 0
	for _, it := range d.Items {
		d.Total += it.Amount
	}
	d.Total += d.Tax + d.Tip
}
