package group

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type memGroups map[uuid.UUID]*Group

func (m memGroups) Create(_ context.Context, g *Group) error { m[g.ID] = g; return nil }
func (m memGroups) Get(_ context.Context, id uuid.UUID) (*Group, error) {
	g, ok := m[id]
	if !ok {
		return nil, errs.NotFound("Group")
	}
	cp := *g
	cp.Members = slices.Clone(g.Members)
	return &cp, nil
}
func (m memGroups) Lock(ctx context.Context, id uuid.UUID) (*Group, error)           { return m.Get(ctx, id) }
func (m memGroups) Hold(_ context.Context, _ uuid.UUID) error                        { return nil }
func (m memGroups) Update(_ context.Context, g *Group) error                         { m[g.ID] = g; return nil }
func (m memGroups) Delete(_ context.Context, id uuid.UUID) error                     { delete(m, id); return nil }
func (m memGroups) ForUser(context.Context, uuid.UUID, bool) ([]*Group, error)       { return nil, nil }
func (m memGroups) Joined(context.Context, uuid.UUID) (int, error)                   { return 0, nil }
func (m memGroups) AddMember(context.Context, uuid.UUID, uuid.UUID, time.Time) error { return nil }
func (m memGroups) RemoveMember(_ context.Context, groupID, userID uuid.UUID, _ time.Time) error {
	g := m[groupID]
	g.Members = slices.DeleteFunc(g.Members, func(id uuid.UUID) bool { return id == userID })
	return nil
}

type memUsers map[uuid.UUID]*user.User

func (m memUsers) Get(_ context.Context, id uuid.UUID) (*user.User, error) {
	u, ok := m[id]
	if !ok {
		return nil, errs.NotFound("User")
	}
	return u, nil
}
func (m memUsers) List(_ context.Context, ids []uuid.UUID) ([]*user.User, error) {
	var out []*user.User
	for _, id := range ids {
		if u, ok := m[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}
func (m memUsers) Index(context.Context, []uuid.UUID) (map[uuid.UUID]*user.User, error) {
	return nil, nil
}
func (m memUsers) Ensure(context.Context, string, string, *user.User) (*user.User, bool, error) {
	return nil, false, nil
}
func (m memUsers) SendInvite(context.Context, *user.User, *user.User) {}

// owing holds the people who are not settled in any group.
type owing map[uuid.UUID]bool

func (o owing) For(context.Context, uuid.UUID) (*balance.Sheet, error) { return nil, nil }
func (o owing) Group(context.Context, uuid.UUID, bool) (*balance.GroupView, error) {
	return nil, nil
}
func (o owing) Settled(_ context.Context, _, userID uuid.UUID) (bool, error) { return !o[userID], nil }
func (o owing) GroupSettled(context.Context, uuid.UUID) (bool, error)        { return len(o) == 0, nil }

type feed struct{ list []activity.Activity }

func (f *feed) Record(_ context.Context, a activity.Activity) error {
	f.list = append(f.list, a)
	return nil
}

type direct struct{}

func (direct) Do(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

func TestRemoveMember(t *testing.T) {
	ctx := context.Background()
	ada, ben, cy, sam, mal := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	people := memUsers{
		ada: {ID: ada, Name: "Ada", Status: user.StatusActive},
		ben: {ID: ben, Name: "Ben", Status: user.StatusActive},
		cy:  {ID: cy, Name: "Cy", Status: user.StatusActive},
		sam: {ID: sam, Name: "Sam", Status: user.StatusPlaceholder},
		mal: {ID: mal, Name: "Mal", Status: user.StatusActive},
	}
	for _, tc := range []struct {
		name          string
		members       []uuid.UUID
		owing         owing
		actor, member uuid.UUID
		want          errs.Kind
	}{
		{name: "a member removes another", members: []uuid.UUID{ada, ben}, actor: ada, member: ben},
		{name: "a member leaves", members: []uuid.UUID{ada, ben}, actor: ben, member: ben},
		{name: "a stranger", members: []uuid.UUID{ada, ben}, actor: mal, member: ben, want: errs.KindNotFound},
		{name: "someone not in the group", members: []uuid.UUID{ada, ben}, actor: ada, member: cy, want: errs.KindNotFound},
		{name: "someone who still owes", members: []uuid.UUID{ada, ben}, owing: owing{ben: true}, actor: ada, member: ben,
			want: errs.KindConflict},
		{name: "the last member who can sign in", members: []uuid.UUID{ada, sam}, actor: ada, member: ada, want: errs.KindConflict},
	} {
		g := &Group{ID: uuid.New(), Name: "Flat", Members: tc.members}
		repo := memGroups{g.ID: g}
		f := &feed{}
		svc := NewService(Deps{Repo: repo, Users: people, Balances: tc.owing, Activity: f,
			Tx: direct{}, Clock: clock.Fixed{At: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)}, BaseURL: "http://test"})
		err := svc.RemoveMember(ctx, tc.actor, g.ID, tc.member)
		if tc.want != "" {
			if !errs.Is(err, tc.want) {
				t.Errorf("%s: want %s, got %v", tc.name, tc.want, err)
			}
			if len(repo[g.ID].Members) != len(tc.members) || len(f.list) != 0 {
				t.Errorf("%s: nothing may change: members %v, feed %v", tc.name, repo[g.ID].Members, f.list)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if slices.Contains(repo[g.ID].Members, tc.member) {
			t.Errorf("%s: still a member", tc.name)
		}
		if len(f.list) != 1 || f.list[0].Kind != activity.GroupMemberRemoved || f.list[0].ActorID != tc.actor {
			t.Errorf("%s: the removal is in the feed: %+v", tc.name, f.list)
		}
	}
}
