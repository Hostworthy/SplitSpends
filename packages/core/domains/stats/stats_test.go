package stats

import (
	"context"
	"testing"
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
