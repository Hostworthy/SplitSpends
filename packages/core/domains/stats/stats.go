// Package stats summarizes spending for charts.
package stats

import (
	"context"
	"time"

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

// Amount is a labelled total.
type Amount struct {
	Key    string
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

// Service validates access and fills defaults.
type Service struct {
	repo   Repository
	groups group.Repository
	users  *user.Service
}

// NewService builds a Service.
func NewService(repo Repository, groups group.Repository, users *user.Service) *Service {
	return &Service{repo: repo, groups: groups, users: users}
}

// Summary returns spending statistics.
func (s *Service) Summary(ctx context.Context, q Query) (*Summary, error) {
	if q.GroupID != nil {
		g, err := s.groups.Get(ctx, *q.GroupID)
		if err != nil {
			return nil, err
		}
		if !g.HasMember(q.UserID) {
			return nil, errs.NotFound("Group")
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
	return s.repo.Summary(ctx, q)
}
