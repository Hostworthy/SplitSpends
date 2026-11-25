package routes

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

// TestImportLimit checks that imports are counted per signed-in person: the
// eleventh in an hour is refused, while someone else can still import.
func TestImportLimit(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada").Token
	ben := s.signIn(t, "ben@example.com", "Ben").Token
	path := platform.Prefix + "/import"
	bundle := platform.ImportRequest{MeID: uuid.NewString()}

	for range 10 {
		s.call(t, http.MethodPost, path, ada, bundle).want(t, http.StatusOK)
	}
	if e := s.call(t, http.MethodPost, path, ada, bundle).want(t, http.StatusTooManyRequests).failure(t); e.Code != "RATE_LIMITED" {
		t.Fatalf("unexpected error %+v", e)
	}
	s.call(t, http.MethodPost, path, ben, bundle).want(t, http.StatusOK)
}

// TestSocialLimit checks that adding people and reminding them share one
// budget per signed-in person: the 21st such request in a minute is refused,
// while someone else can still add people.
func TestSocialLimit(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada").Token
	ben := s.signIn(t, "ben@example.com", "Ben").Token
	friends := platform.Prefix + "/friends"
	for i := range 19 {
		s.call(t, http.MethodPost, friends, ada, platform.Invitee{Name: fmt.Sprintf("Person %d", i)}).want(t, http.StatusCreated)
	}
	name := "Trip"
	s.call(t, http.MethodPost, platform.Prefix+"/groups", ada, platform.GroupCreate{GroupPatch: platform.GroupPatch{Name: &name}}).want(t, http.StatusCreated)
	if e := s.call(t, http.MethodPost, friends, ada, platform.Invitee{Name: "One more"}).want(t, http.StatusTooManyRequests).failure(t); e.Code != "RATE_LIMITED" {
		t.Fatalf("unexpected error %+v", e)
	}
	s.call(t, http.MethodPost, friends, ben, platform.Invitee{Name: "Sam"}).want(t, http.StatusCreated)
}
