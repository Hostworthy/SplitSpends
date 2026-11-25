package activity

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func removed(actor, target uuid.UUID, targetName string) *Activity {
	return &Activity{ActorID: actor, Kind: GroupMemberRemoved,
		Data: map[string]any{"groupName": "Flat", "targetName": targetName, "targetId": target.String()}}
}

func TestDescribeRemovedSameName(t *testing.T) {
	got := Describe(removed(uuid.New(), uuid.New(), "Ben"), "Ben")
	if want := "Ben removed Ben from Flat"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDescribeLeftAfterRename(t *testing.T) {
	ben := uuid.New()
	got := Describe(removed(ben, ben, "Ben"), "Benjamin")
	if want := "Benjamin left Flat"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

type saved struct{ list []*Activity }

func (r *saved) Create(_ context.Context, a *Activity) error {
	r.list = append(r.list, a)
	return nil
}
func (r *saved) List(context.Context, Filter) ([]*Activity, int, error) {
	return r.list, len(r.list), nil
}

type fixed struct{ at time.Time }

func (c fixed) Now() time.Time { return c.at }

// TestRecordInvolvesActorOnce checks that every event is visible to its
// actor and lists each person once, however callers pass them.
func TestRecordInvolvesActorOnce(t *testing.T) {
	ada, ben, cy := uuid.New(), uuid.New(), uuid.New()
	for name, c := range map[string]struct{ in, want []uuid.UUID }{
		"actor missing":  {[]uuid.UUID{ben, cy}, []uuid.UUID{ben, cy, ada}},
		"actor included": {[]uuid.UUID{ada, ben}, []uuid.UUID{ada, ben}},
		"duplicates":     {[]uuid.UUID{ben, ben, ada, cy, ben}, []uuid.UUID{ben, ada, cy}},
		"nobody else":    {nil, []uuid.UUID{ada}},
	} {
		repo := &saved{}
		at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		if err := NewService(repo, fixed{at}).Record(context.Background(), Activity{ActorID: ada, Kind: GroupUpdated, Involved: c.in}); err != nil {
			t.Fatal(err)
		}
		a := repo.list[0]
		if !slices.Equal(a.Involved, c.want) {
			t.Errorf("%s: want involved %v, got %v", name, c.want, a.Involved)
		}
		if a.ID == uuid.Nil || !a.CreatedAt.Equal(at) || a.Data == nil {
			t.Errorf("%s: want an id, the clock's time and empty data, got %+v", name, a)
		}
	}
}
