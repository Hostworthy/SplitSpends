// Package stats summarizes spending for charts.
package stats

import (
	"context"
	"sort"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Query selects what to summarize. Without GroupID the caller's own share of
// every expense is used; with GroupID the group's full spending is used.
type Query struct {
	UserID   uuid.UUID
	GroupID  *uuid.UUID
	Currency string
	From     time.Time
	To       time.Time
}

// Amount is a labelled total. Name is the category name for ByCategory.
type Amount struct {
	Key    string
	Name   string
	Amount int64
}

// Member is what one person paid and owed.
type Member struct {
	UserID uuid.UUID
	Paid   int64
	Owed   int64
}

// Summary is the chart data.
type Summary struct {
	Currency   string
	Total      int64
	ByCategory []Amount
	ByMonth    []Amount
	ByMember   []Member
}

// Repository runs the aggregate queries. Payments are excluded.
type Repository interface {
	Summary(ctx context.Context, q Query) (*Summary, error)
}

// Names resolves category ids to display names. Ids without a name were
// deleted (custom categories) or never existed.
type Names interface {
	Names(ctx context.Context, ids []string) (map[string]string, error)
}

// Groups is what stats need from the group context.
type Groups interface {
	// Member returns the group if actor is a current member, and
	// errs.NotFound otherwise.
	Member(ctx context.Context, actor, id uuid.UUID) (*group.Group, error)
}

// Users reads the caller's profile for their default currency.
type Users interface {
	Get(ctx context.Context, id uuid.UUID) (*user.User, error)
}

// Service validates access and fills defaults.
type Service struct {
	repo   Repository
	groups Groups
	users  Users
	names  Names
}

// NewService builds a Service.
func NewService(repo Repository, groups Groups, users Users, names Names) *Service {
	return &Service{repo: repo, groups: groups, users: users, names: names}
}

// Summary returns spending statistics.
func (s *Service) Summary(ctx context.Context, q Query) (*Summary, error) {
	if q.GroupID != nil {
		g, err := s.groups.Member(ctx, q.UserID, *q.GroupID)
		if err != nil {
			return nil, err
		}
		if q.Currency == "" {
			q.Currency = g.DefaultCurrency
		}
	}
	if q.Currency == "" {
		me, err := s.users.Get(ctx, q.UserID)
		if err != nil {
			return nil, err
		}
		q.Currency = me.DefaultCurrency
	}
	c, ok := money.Lookup(q.Currency)
	if !ok {
		return nil, errs.Validation("currency", "Unsupported currency.")
	}
	q.Currency = c.Code
	if q.To.IsZero() {
		q.To = time.Now().UTC()
	}
	if q.From.IsZero() {
		q.From = q.To.AddDate(-1, 0, 0)
	}
	if q.From.After(q.To) {
		return nil, errs.Validation("from", "The start date must be before the end date.")
	}
	sum, err := s.repo.Summary(ctx, q)
	if err != nil {
		return nil, err
	}
	if sum.ByCategory, err = s.label(ctx, sum.ByCategory); err != nil {
		return nil, err
	}
	return sum, nil
}

// label names each category total. Spending under deleted custom categories
// counts as General, the way clients show those expenses.
func (s *Service) label(ctx context.Context, totals []Amount) ([]Amount, error) {
	ids := make([]string, 0, len(totals)+1)
	for _, a := range totals {
		ids = append(ids, a.Key)
	}
	ids = append(ids, expense.DefaultCategory)
	names, err := s.names.Names(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Amount, 0, len(totals))
	index := map[string]int{}
	for _, a := range totals {
		if _, ok := names[a.Key]; !ok {
			a.Key = expense.DefaultCategory
		}
		if i, ok := index[a.Key]; ok {
			out[i].Amount += a.Amount
			continue
		}
		index[a.Key] = len(out)
		out = append(out, Amount{Key: a.Key, Name: names[a.Key], Amount: a.Amount})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Amount > out[j].Amount })
	return out, nil
}
