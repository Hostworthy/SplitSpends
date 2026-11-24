package balance

import (
	"context"

	"github.com/Hostworthy/SplitSpends/backend/internal/core/shared/money"
	"github.com/google/uuid"
)

// GroupRef is the slice of a group that balance needs.
type GroupRef struct {
	ID       uuid.UUID
	Name     string
	Simplify bool
}

// Ledger reads the entries balances are computed from.
type Ledger interface {
	// Group returns entries of one group.
	Group(ctx context.Context, groupID uuid.UUID) ([]Entry, error)
	// Groups returns the active groups of userID.
	Groups(ctx context.Context, userID uuid.UUID) ([]GroupRef, error)
	// GroupsEntries returns entries of many groups keyed by group id.
	GroupsEntries(ctx context.Context, groupIDs []uuid.UUID) (map[uuid.UUID][]Entry, error)
	// NonGroup returns entries without a group that involve userID.
	NonGroup(ctx context.Context, userID uuid.UUID) ([]Entry, error)
}

// Service answers balance questions.
type Service struct {
	ledger Ledger
}

// NewService builds a Service.
func NewService(ledger Ledger) *Service { return &Service{ledger: ledger} }

// GroupView is the balance sheet of one group.
type GroupView struct {
	Members map[uuid.UUID]money.Balances
	Debts   []Debt
}

// Group computes balances and debts of a group.
func (s *Service) Group(ctx context.Context, groupID uuid.UUID, simplify bool) (*GroupView, error) {
	entries, err := s.ledger.Group(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return &GroupView{Members: Nets(entries), Debts: Debts(entries, simplify)}, nil
}

// FriendView is the balance between the caller and one other person.
type FriendView struct {
	Total    money.Balances
	ByGroup  map[uuid.UUID]money.Balances
	NonGroup money.Balances
}

// Sheet is everything the caller owes and is owed.
type Sheet struct {
	Friends map[uuid.UUID]*FriendView
	Groups  map[uuid.UUID]money.Balances
	Names   map[uuid.UUID]string
}

// For computes the caller's balances with every person and group.
func (s *Service) For(ctx context.Context, me uuid.UUID) (*Sheet, error) {
	groups, err := s.ledger.Groups(ctx, me)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	entries, err := s.ledger.GroupsEntries(ctx, ids)
	if err != nil {
		return nil, err
	}
	nonGroup, err := s.ledger.NonGroup(ctx, me)
	if err != nil {
		return nil, err
	}

	sheet := &Sheet{Friends: map[uuid.UUID]*FriendView{}, Groups: map[uuid.UUID]money.Balances{}, Names: map[uuid.UUID]string{}}
	friend := func(id uuid.UUID) *FriendView {
		f, ok := sheet.Friends[id]
		if !ok {
			f = &FriendView{Total: money.Balances{}, ByGroup: map[uuid.UUID]money.Balances{}, NonGroup: money.Balances{}}
			sheet.Friends[id] = f
		}
		return f
	}
	collect := func(debts []Debt, apply func(other uuid.UUID, currency string, amount int64)) {
		for _, d := range debts {
			switch me {
			case d.To:
				apply(d.From, d.Currency, d.Amount)
			case d.From:
				apply(d.To, d.Currency, -d.Amount)
			}
		}
	}

	for _, g := range groups {
		sheet.Names[g.ID] = g.Name
		list := entries[g.ID]
		mine := Nets(list)[me]
		if mine == nil {
			mine = money.Balances{}
		}
		sheet.Groups[g.ID] = mine
		collect(Debts(list, g.Simplify), func(other uuid.UUID, currency string, amount int64) {
			f := friend(other)
			b, ok := f.ByGroup[g.ID]
			if !ok {
				b = money.Balances{}
				f.ByGroup[g.ID] = b
			}
			b.Add(currency, amount)
			f.Total.Add(currency, amount)
		})
	}
	collect(Pairwise(nonGroup), func(other uuid.UUID, currency string, amount int64) {
		f := friend(other)
		f.NonGroup.Add(currency, amount)
		f.Total.Add(currency, amount)
	})
	return sheet, nil
}

// Totals splits a sheet into overall net, what others owe you and what you owe.
func (sh *Sheet) Totals() (net, owed, owing money.Balances) {
	net, owed, owing = money.Balances{}, money.Balances{}, money.Balances{}
	for _, f := range sh.Friends {
		for c, a := range f.Total {
			net.Add(c, a)
			if a > 0 {
				owed.Add(c, a)
			} else if a < 0 {
				owing.Add(c, -a)
			}
		}
	}
	return net, owed, owing
}

// Settled reports whether userID has no outstanding balance in groupID.
func (s *Service) Settled(ctx context.Context, groupID, userID uuid.UUID) (bool, error) {
	entries, err := s.ledger.Group(ctx, groupID)
	if err != nil {
		return false, err
	}
	return Nets(entries)[userID].Settled(), nil
}

// GroupSettled reports whether nobody owes anything in groupID.
func (s *Service) GroupSettled(ctx context.Context, groupID uuid.UUID) (bool, error) {
	entries, err := s.ledger.Group(ctx, groupID)
	if err != nil {
		return false, err
	}
	for _, b := range Nets(entries) {
		if !b.Settled() {
			return false, nil
		}
	}
	return true, nil
}
