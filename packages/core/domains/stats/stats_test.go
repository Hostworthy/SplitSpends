package stats

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type names map[string]string

func (n names) Names(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if v, ok := n[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func TestLabelMergesDeletedCategories(t *testing.T) {
	s := &Service{names: names{"general": "General", "food.dining": "Dining out", "custom.live": "Climbing"}}
	got, err := s.label(context.Background(), []Amount{
		{Key: "food.dining", Amount: 500},
		{Key: "custom.live", Amount: 400},
		{Key: "custom.gone", Amount: 300},
		{Key: "general", Amount: 250},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Amount{
		{Key: "general", Name: "General", Amount: 550},
		{Key: "food.dining", Name: "Dining out", Amount: 500},
		{Key: "custom.live", Name: "Climbing", Amount: 400},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

type members map[uuid.UUID]*group.Group

func (m members) Member(_ context.Context, actor, id uuid.UUID) (*group.Group, error) {
	g, ok := m[id]
	if !ok || !g.HasMember(actor) {
		return nil, errs.NotFound("Group")
	}
	return g, nil
}

type people map[uuid.UUID]*user.User

func (p people) Get(_ context.Context, id uuid.UUID) (*user.User, error) { return p[id], nil }

// asked records the query the service passes on.
type asked struct{ q *Query }

func (a *asked) Summary(_ context.Context, q Query) (*Summary, error) {
	a.q = &q
	return &Summary{Currency: q.Currency}, nil
}

// TestSummaryDefaultsToLastYear checks that a query without dates covers the
// year up to and including today.
func TestSummaryDefaultsToLastYear(t *testing.T) {
	ada := uuid.New()
	repo := &asked{}
	now := time.Date(2026, 3, 15, 23, 30, 0, 0, time.UTC)
	s := NewService(repo, members{}, people{ada: {ID: ada, DefaultCurrency: "EUR"}}, names{}, clock.Fixed{At: now})
	if _, err := s.Summary(context.Background(), Query{UserID: ada}); err != nil {
		t.Fatal(err)
	}
	from, to := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if !repo.q.From.Equal(from) || !repo.q.To.Equal(to) {
		t.Fatalf("want %s to %s, got %s to %s", from, to, repo.q.From, repo.q.To)
	}
}

func TestSummaryNeedsMembership(t *testing.T) {
	ctx := context.Background()
	ada, mal := uuid.New(), uuid.New()
	g := &group.Group{ID: uuid.New(), DefaultCurrency: "JPY", Members: []uuid.UUID{ada}}
	repo := &asked{}
	s := NewService(repo, members{g.ID: g}, people{ada: {ID: ada, DefaultCurrency: "EUR"}}, names{},
		clock.Fixed{At: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)})
	for name, id := range map[string]uuid.UUID{"a group Mal is not in": g.ID, "a group that does not exist": uuid.New()} {
		if _, err := s.Summary(ctx, Query{UserID: mal, GroupID: &id}); !errs.Is(err, errs.KindNotFound) {
			t.Errorf("%s must be not found, got %v", name, err)
		}
	}
	if repo.q != nil {
		t.Fatalf("nothing may be summarized for Mal: %+v", repo.q)
	}
	if _, err := s.Summary(ctx, Query{UserID: ada, GroupID: &g.ID}); err != nil {
		t.Fatal(err)
	}
	if repo.q == nil || repo.q.Currency != "JPY" {
		t.Fatalf("a group summary is in the group's currency: %+v", repo.q)
	}
}
