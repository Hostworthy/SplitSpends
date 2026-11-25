package activity

import (
	"testing"

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
