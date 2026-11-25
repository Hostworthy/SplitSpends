// Package assist turns the user's instructions, and optionally a receipt
// photo, into an expense draft using the AI provider the host configured.
// The model runs outside SplitSpends; this package builds its context and
// checks everything it returns. Drafts are never saved: the user reviews them.
package assist

import (
	"context"
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
	reader   ports.Assistant
	users    *user.Service
	groups   *group.Service
	friends  friend.Repository
	receipts *receipt.Service
	rates    *rates.Service
	clock    clock.Clock
}

// NewService builds a Service. reader may be nil when AI is disabled.
func NewService(reader ports.Assistant, users *user.Service, groups *group.Service, friends friend.Repository, receipts *receipt.Service, r *rates.Service, c clock.Clock) *Service {
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
		Me: ports.Person{ID: k.me.ID.String(), Name: k.me.Name}, Categories: expense.CategoryIDs()}
	for _, u := range k.people {
		in.People = append(in.People, ports.Person{ID: u.ID.String(), Name: u.Name})
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
		return nil, errs.AIDisabled("AI features are not enabled on this server.")
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
		in.Image, in.ContentType, err = s.receipts.Image(ctx, actor, *req.ReceiptID)
		if err != nil {
			return nil, err
		}
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
	g := chooseGroup(k, req, ext)
	if g != nil {
		d.GroupID = &g.ID
		currency = g.DefaultCurrency
	}
	d.Currency = currency
	conv, err := converter(ext.Currency, currency, table)
	if err != nil {
		return nil, err
	}
	d.details(ext)

	r := &roster{k: k, g: g, dropped: map[uuid.UUID]bool{}, warn: d.warn}
	participants, preset := r.participants(ext.ParticipantIDs)
	if len(ext.Items) > 0 {
		if err := d.itemize(ext, conv, r, participants); err != nil {
			return nil, err
		}
		if len(d.Items) == 0 {
			d.Method = expense.MethodEqual
			if preset != nil {
				d.warn("Could not read any items, so the group's default split is used.")
			} else {
				d.warn("Could not read any items, so the total is split equally.")
			}
		}
	}
	if d.Method == expense.MethodEqual {
		if d.Total, err = conv.amount(ext.Amount); err != nil {
			return nil, err
		}
		d.share(participants, preset, g)
	}
	if d.Total == 0 {
		d.warn("Could not find an amount.")
	}

	payer := k.me.ID
	if id, err := uuid.Parse(ext.PayerID); err == nil && r.allowed(id) {
		payer = id
	}
	d.Payers = []expense.Payer{{UserID: payer, Amount: d.Total}}
	for _, name := range ext.Unmatched {
		if name = strings.TrimSpace(name); name != "" {
			d.warn("Could not match “" + name + "” to anyone you know.")
		}
	}
	if note := conv.note(ext.Amount); note != "" {
		d.warn(note)
	}
	return d, nil
}

func (d *Draft) warn(msg string) { d.Warnings = append(d.Warnings, msg) }

// details keeps the date and category the model read when they are valid,
// and names an expense it found no description for.
func (d *Draft) details(ext ports.DraftExtract) {
	if _, err := time.Parse(time.DateOnly, ext.Date); err == nil {
		d.Date = ext.Date
	}
	if expense.KnownCategory(ext.Category) && ext.Category != expense.PaymentCategory {
		d.Category = ext.Category
	}
	if d.Description == "" {
		d.Description = "Expense"
	}
}

// chooseGroup is the group the draft belongs to: the one the user picked,
// else one of theirs the model chose, else none.
func chooseGroup(k *known, req Request, ext ports.DraftExtract) *group.Group {
	if req.GroupID != nil {
		return k.groups[*req.GroupID]
	}
	if id, err := uuid.Parse(ext.GroupID); err == nil {
		return k.groups[id]
	}
	return nil
}

// conversion reads the model's amounts, written in source, as minor units of
// target, converting them exactly when the two differ.
type conversion struct {
	source, target string
	exponent       int
	// rate is nil when source is target.
	rate *rates.Table
}

// converter reads amounts in the currency the model named, or in target when
// it named none it knows, and fetches today's rates when that differs from
// target.
func converter(read, target string, table func() *rates.Table) (*conversion, error) {
	c := &conversion{source: target, target: target}
	if cur, ok := money.Lookup(read); ok {
		c.source = cur.Code
	}
	from, _ := money.Lookup(c.source)
	c.exponent = from.Exponent
	if c.source == target {
		return c, nil
	}
	c.rate = table()
	if c.rate == nil || !c.rate.Has(c.source) || !c.rate.Has(target) {
		return nil, errs.NoRates("These amounts are in " + c.source + ", and exchange rates are not available right now to convert them to " +
			target + ". Try again later, or enter the expense yourself.")
	}
	return c, nil
}

// amount is s in minor units of the target currency. Text that is not an
// amount, or is negative, reads as zero so the caller can leave it out. An
// amount too large for an expense once converted is an error, not a zero.
func (c *conversion) amount(s string) (int64, error) {
	v, ok := expense.ParseDecimal(s, c.exponent)
	if !ok || v < 0 {
		return 0, nil
	}
	if c.rate == nil {
		return v, nil
	}
	if v, ok = c.rate.Convert(rates.Amount{Currency: c.source, Minor: v}, c.target); !ok || v > expense.MaxTotal {
		return 0, errs.Validation("", "These amounts are too large for one expense once converted to "+c.target+".")
	}
	return v, nil
}

// note explains a conversion, quoting total as the receipt showed it, and is
// empty when nothing was converted.
func (c *conversion) note(total string) string {
	if c.rate == nil {
		return ""
	}
	note := "Converted from " + c.source + " to " + c.target + " at 1 " + c.source + " = " + unitRate(c.rate, c.source, c.target) + " " + c.target
	if !c.rate.Published.IsZero() {
		note += " (rates of " + c.rate.Published.Format("2 Jan 2006") + ")"
	}
	if original, _ := expense.ParseDecimal(total, c.exponent); original > 0 {
		note += "; the receipt total was " + money.Format(original, c.source)
	}
	return note + "."
}

// roster decides who may take part in the draft: the group's members, or
// the user and their friends outside groups.
type roster struct {
	k       *known
	g       *group.Group
	dropped map[uuid.UUID]bool
	warn    func(string)
}

func (r *roster) allowed(id uuid.UUID) bool {
	if id == r.k.me.ID {
		return true
	}
	if r.g != nil {
		return r.g.HasMember(id)
	}
	return r.k.friends[id]
}

// resolve keeps the ids in raw that may take part, once each, and warns
// once about each known person left out of the group.
func (r *roster) resolve(raw []string) []uuid.UUID {
	out := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, s := range raw {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil || seen[id] {
			continue
		}
		if !r.allowed(id) {
			if u := r.k.people[id]; u != nil && !r.dropped[id] && r.g != nil {
				r.warn(u.Name + " is not in " + r.g.Name + ", so they were left out.")
			}
			r.dropped[id] = true
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// participants resolves the people the instructions name. When they name
// nobody, everyone in the group takes part (or the user alone outside
// groups), and the group's default split, say rent 60/40, applies as it
// does in the editor and in MCP.
func (r *roster) participants(raw []string) ([]uuid.UUID, *group.DefaultSplit) {
	if ids := r.resolve(raw); len(ids) > 0 {
		return ids, nil
	}
	if r.g == nil {
		return []uuid.UUID{r.k.me.ID}, nil
	}
	ids := append([]uuid.UUID{}, r.g.Members...)
	if p := r.g.Effective(); p != nil && p.Method != group.SplitEqual {
		return ids, p
	}
	return ids, nil
}

// itemize fills the draft from the receipt lines the model read. Lines
// without an amount are left out, and lines that name nobody are shared by
// participants.
func (d *Draft) itemize(ext ports.DraftExtract, c *conversion, r *roster, participants []uuid.UUID) error {
	d.Method = expense.MethodItemized
	var sum int64
	for _, it := range ext.Items {
		a, err := c.amount(it.Amount)
		if err != nil {
			return err
		}
		if a == 0 {
			continue
		}
		ids := r.resolve(it.ParticipantIDs)
		if len(ids) == 0 {
			ids = participants
		}
		name := strings.TrimSpace(it.Name)
		if name == "" {
			name = "Item"
		}
		d.Items = append(d.Items, expense.Item{Name: name, Amount: a, UserIDs: ids})
		// Each amount is at most MaxTotal, so stopping here keeps the sum
		// far from overflowing.
		if sum += a; sum > expense.MaxTotal {
			return tooLarge()
		}
	}
	var err error
	if d.Tax, err = c.amount(ext.Tax); err != nil {
		return err
	}
	if d.Tip, err = c.amount(ext.Tip); err != nil {
		return err
	}
	want, err := c.amount(ext.Amount)
	if err != nil {
		return err
	}
	if d.Total = sum + d.Tax + d.Tip; d.Total > expense.MaxTotal {
		return tooLarge()
	}
	if want > 0 && want != d.Total && len(d.Items) > 0 && !reconcile(d, want) {
		d.warn("The receipt total did not match its lines; check the amounts.")
	}
	return nil
}

// share splits the total between participants, by the group's default split
// when one applies and equally otherwise.
func (d *Draft) share(participants []uuid.UUID, preset *group.DefaultSplit, g *group.Group) {
	if preset != nil {
		d.Method, d.Splits = expense.Method(preset.Method), defaults(preset, g.Members)
		return
	}
	for _, id := range participants {
		d.Splits = append(d.Splits, expense.SplitInput{UserID: id})
	}
}

func tooLarge() error {
	return errs.Validation("", "These amounts are too large for one expense.")
}

// defaults returns the splits a percent or shares default gives the members,
// in member order. A member whose value is zero takes no part.
func defaults(preset *group.DefaultSplit, members []uuid.UUID) []expense.SplitInput {
	splits := []expense.SplitInput{}
	for _, id := range members {
		v := preset.Values[id]
		if r, ok := money.Decimal(v); ok && r.Sign() == 0 {
			continue
		}
		splits = append(splits, expense.SplitInput{UserID: id, Value: v})
	}
	return splits
}

// unitRate is how much one unit of from buys in to, to four decimals.
func unitRate(t *rates.Table, from, to string) string {
	rf, _ := t.Rate(from)
	rt, _ := t.Rate(to)
	return new(big.Rat).Quo(rt, rf).FloatString(4)
}

// reconcile makes items + tax + tip equal the receipt total with the rule
// scans use, and reports false, leaving the draft as read, when the items
// cannot reach it.
func reconcile(d *Draft, total int64) bool {
	amounts := make([]int64, len(d.Items))
	for i, it := range d.Items {
		amounts[i] = it.Amount
	}
	fixed, tax, ok := receipt.Reconcile(amounts, d.Tip, total)
	if !ok {
		return false
	}
	for i := range d.Items {
		d.Items[i].Amount = fixed[i]
	}
	d.Tax, d.Total = tax, total
	return true
}
