package routes

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// TestAddFriendDoesNotRevealAccount checks that adding or inviting someone
// by email does not tell the caller an account holder's name or that an
// administrator disabled them.
func TestAddFriendDoesNotRevealAccount(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	admin := s.signIn(t, "admin@example.com", "Admin").Token
	ben := s.signIn(t, "ben@example.com", "Ben Real").User.ID
	mal := s.signIn(t, "mal@example.com", "Mal").Token
	friends := platform.Prefix + "/friends"

	s.call(t, http.MethodPost, platform.Prefix+"/admin/accounts/"+ben+"/disable", admin, nil).want(t, http.StatusOK)
	added := data[platform.Friend](t, s.call(t, http.MethodPost, friends, mal, platform.Invitee{Email: "ben@example.com"}).
		want(t, http.StatusCreated))
	if added.User.Status != "active" {
		t.Fatalf("a disabled account must look like any other account to other people, got status %q", added.User.Status)
	}
	for _, f := range items[platform.Friend](t, s.call(t, http.MethodGet, friends, mal, nil).want(t, http.StatusOK)).Items {
		if f.User.Status == "disabled" {
			t.Fatalf("the friend list shows that %s was disabled", f.User.ID)
		}
	}
	if accounts := items[platform.Account](t, s.call(t, http.MethodGet, platform.Prefix+"/admin/accounts?status=disabled", admin, nil).
		want(t, http.StatusOK)); len(accounts.Items) != 1 || accounts.Items[0].User.Status != "disabled" {
		t.Fatalf("administrators still see the real status, got %+v", accounts.Items)
	}

	s.signIn(t, "cy@example.com", "Cy Secret")
	sam := data[platform.Friend](t, s.call(t, http.MethodPost, friends, mal, platform.Invitee{Name: "Sam"}).want(t, http.StatusCreated))
	e := s.call(t, http.MethodPatch, friends+"/"+sam.User.ID, mal, platform.FriendInvite{Email: "cy@example.com"}).
		want(t, http.StatusConflict).failure(t)
	if strings.Contains(e.Error, "Cy") || strings.Contains(e.Error, "Secret") {
		t.Fatalf("the conflict names the account holder: %q", e.Error)
	}
}
