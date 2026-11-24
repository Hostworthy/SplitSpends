// Package assist turns free text into expense drafts with the optional AI
// adapter. Drafts are never saved; the user reviews them first.
package assist

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/atb00ker/splitspends/backend/internal/core/domains/expense"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/friend"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/group"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/user"
	"github.com/atb00ker/splitspends/backend/internal/core/ports"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/clock"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/money"
	"github.com/google/uuid"
)

// Draft is a reviewable expense proposal.
type Draft struct {
	GroupID     *uuid.UUID
	Description string
	Currency    string
	Total       int64
	Date        string
	Category    string
	Payers      []expense.Payer
	Splits      []expense.SplitInput
	Warnings    []string
}

// Service builds drafts.
type Service struct {
	reader  ports.ReceiptReader
	users   *user.Service
	groups  *group.Service
	friends friend.Repository
	clock   clock.Clock
}

// NewService builds a Service. reader may be nil when AI is disabled.
func NewService(reader ports.ReceiptReader, users *user.Service, groups *group.Service, friends friend.Repository, c clock.Clock) *Service {
	return &Service{reader: reader, users: users, groups: groups, friends: friends, clock: c}
}

// Draft reads text such as "dinner 60 paid by me split with Sam and Alex".
func (s *Service) Draft(ctx context.Context, actor uuid.UUID, text string, groupID *uuid.UUID) (*Draft, error) {
	if s.reader == nil {
		return nil, errs.Unavailable("AI features are not enabled on this server.")
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > 500 {
		return nil, errs.Validation("text", "Describe the expense in up to 500 characters.")
	}
	me, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	currency := me.DefaultCurrency
	var ids []uuid.UUID
	if groupID != nil {
		g, err := s.groups.Member(ctx, actor, *groupID)
		if err != nil {
			return nil, err
		}
		ids, currency = g.Members, g.DefaultCurrency
	} else if ids, err = s.friends.List(ctx, actor); err != nil {
		return nil, err
	}
	people, err := s.users.List(ctx, ids)
	if err != nil {
		return nil, err
	}
	known := map[string]uuid.UUID{me.ID.String(): me.ID}
	candidates := []ports.Person{}
	for _, u := range people {
		if u.ID == actor {
			continue
		}
		known[u.ID.String()] = u.ID
		candidates = append(candidates, ports.Person{ID: u.ID.String(), Name: u.Name, Email: u.Email})
	}

	tz, err := time.LoadLocation(me.Timezone)
	if err != nil {
		tz = time.UTC
	}
	today := s.clock.Now().In(tz).Format(time.DateOnly)
	ext, err := s.reader.DraftExpense(ctx, ports.DraftInput{Text: text, Today: today, Currency: currency,
		Me: ports.Person{ID: me.ID.String(), Name: me.Name, Email: me.Email}, People: candidates, Categories: expense.CategoryIDs()})
	if err != nil {
		return nil, err
	}

	d := &Draft{GroupID: groupID, Description: strings.TrimSpace(ext.Description), Currency: currency, Date: today, Category: expense.DefaultCategory}
	if c, ok := money.Lookup(ext.Currency); ok {
		d.Currency = c.Code
	}
	cur, _ := money.Lookup(d.Currency)
	if v, ok := expense.ParseDecimal(ext.Amount, cur.Exponent); ok && v > 0 {
		d.Total = v
	} else {
		d.Warnings = append(d.Warnings, "Could not find an amount.")
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
	payer, ok := known[ext.PayerID]
	if !ok {
		payer = actor
	}
	d.Payers = []expense.Payer{{UserID: payer, Amount: d.Total}}
	seen := map[uuid.UUID]bool{}
	for _, raw := range ext.ParticipantIDs {
		if id, ok := known[raw]; ok && !seen[id] {
			seen[id] = true
			d.Splits = append(d.Splits, expense.SplitInput{UserID: id})
		}
	}
	if len(d.Splits) == 0 {
		d.Splits = []expense.SplitInput{{UserID: actor}}
	}
	for _, name := range ext.Unmatched {
		if name = strings.TrimSpace(name); name != "" {
			d.Warnings = append(d.Warnings, "Could not match “"+name+"” to anyone you know.")
		}
	}
	return d, nil
}
