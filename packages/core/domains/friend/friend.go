// Package friend owns friendships. People who share a group are treated as
// friends too, so the friend list is explicit friendships plus co-members.
package friend

import (
	"context"
	"sort"
	"strings"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Repository persists friendships.
type Repository interface {
	Add(ctx context.Context, a, b uuid.UUID) error
	// Remove reports whether there was an explicit friendship to end.
	Remove(ctx context.Context, a, b uuid.UUID) (bool, error)
	// List returns explicit friends plus active group co-members.
	List(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	// Connected reports whether a and b are friends or share a group.
	Connected(ctx context.Context, a, b uuid.UUID) (bool, error)
}

// Service implements friend use cases.
type Service struct {
	repo     Repository
	users    *user.Service
	balances *balance.Service
	activity *activity.Service
	tx       ports.Tx
	clock    clock.Clock
}

// NewService builds a Service.
func NewService(repo Repository, users *user.Service, balances *balance.Service, act *activity.Service, tx ports.Tx, c clock.Clock) *Service {
	return &Service{repo: repo, users: users, balances: balances, activity: act, tx: tx, clock: c}
}

// Summary is a friend and the total balance with them.
type Summary struct {
	User     *user.User
	Balances money.Balances
}

// GroupBalance is the balance with a friend inside one group.
type GroupBalance struct {
	GroupID   uuid.UUID
	GroupName string
	Balances  money.Balances
}

// Detail is the full balance picture with one friend.
type Detail struct {
	Summary
	Groups   []GroupBalance
	NonGroup money.Balances
}

// List returns friends sorted by name, with balances.
func (s *Service) List(ctx context.Context, me uuid.UUID) ([]Summary, error) {
	ids, err := s.repo.List(ctx, me)
	if err != nil {
		return nil, err
	}
	sheet, err := s.balances.For(ctx, me)
	if err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]bool{me: true}
	all := []uuid.UUID{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			all = append(all, id)
		}
	}
	for id, f := range sheet.Friends {
		if !seen[id] && !f.Total.Settled() {
			seen[id] = true
			all = append(all, id)
		}
	}
	people, err := s.users.List(ctx, all)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(people))
	for _, u := range people {
		b := money.Balances{}
		if f := sheet.Friends[u.ID]; f != nil {
			b = f.Total
		}
		out = append(out, Summary{User: u, Balances: b})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].User.Name) < strings.ToLower(out[j].User.Name)
	})
	return out, nil
}

// Add befriends someone by email, inviting them when they are new.
func (s *Service) Add(ctx context.Context, me uuid.UUID, email, name string) (*Summary, error) {
	actor, err := s.users.Get(ctx, me)
	if err != nil {
		return nil, err
	}
	var friend *user.User
	var invited bool
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		u, created, err := s.users.Ensure(ctx, email, name, actor)
		if err != nil {
			return err
		}
		if u.ID == me {
			return errs.Validation("email", "You cannot add yourself as a friend.")
		}
		if err := s.repo.Add(ctx, me, u.ID); err != nil {
			return err
		}
		friend, invited = u, created
		return s.activity.Record(ctx, activity.Activity{ActorID: me, Kind: activity.FriendAdded,
			Involved: []uuid.UUID{u.ID}, Data: map[string]any{"targetName": u.Name, "targetId": u.ID.String()}})
	})
	if err != nil {
		return nil, err
	}
	if invited {
		s.users.SendInvite(ctx, friend, actor)
	}
	d, err := s.Detail(ctx, me, friend.ID)
	if err != nil {
		return nil, err
	}
	return &d.Summary, nil
}

// Detail returns balances with one friend, broken down by group.
func (s *Service) Detail(ctx context.Context, me, id uuid.UUID) (*Detail, error) {
	ok, err := s.repo.Connected(ctx, me, id)
	if err != nil {
		return nil, err
	}
	sheet, err := s.balances.For(ctx, me)
	if err != nil {
		return nil, err
	}
	view := sheet.Friends[id]
	if !ok && (view == nil || view.Total.Settled()) {
		return nil, errs.NotFound("Friend")
	}
	u, err := s.users.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &Detail{Summary: Summary{User: u, Balances: money.Balances{}}, NonGroup: money.Balances{}}
	if view != nil {
		d.Balances = view.Total
		d.NonGroup = view.NonGroup
		for gid, b := range view.ByGroup {
			if !b.Settled() {
				d.Groups = append(d.Groups, GroupBalance{GroupID: gid, GroupName: sheet.Names[gid], Balances: b})
			}
		}
		sort.Slice(d.Groups, func(i, j int) bool { return d.Groups[i].GroupName < d.Groups[j].GroupName })
	}
	return d, nil
}

// Remove ends an explicit friendship once everything is settled. Someone
// who is a friend only through a shared group stays one, so that is a
// conflict rather than a removal that changes nothing.
func (s *Service) Remove(ctx context.Context, me, id uuid.UUID) error {
	d, err := s.Detail(ctx, me, id)
	if err != nil {
		return err
	}
	if !d.Balances.Settled() {
		return errs.Conflict("Settle up with " + d.User.Name + " before removing them.")
	}
	removed, err := s.repo.Remove(ctx, me, id)
	if err != nil || removed {
		return err
	}
	connected, err := s.repo.Connected(ctx, me, id)
	if err != nil {
		return err
	}
	if connected {
		return errs.Conflict("You share a group with " + d.User.Name + ". Leave the group to stop being connected.")
	}
	return nil
}

// Connected reports whether a and b may share non-group expenses.
func (s *Service) Connected(ctx context.Context, a, b uuid.UUID) (bool, error) {
	if a == b {
		return true, nil
	}
	return s.repo.Connected(ctx, a, b)
}
