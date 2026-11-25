package routes

import (
	"net/http"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// TestApprovalsPreference turns the approvals notification off for email
// and checks that GET /me reports it, with push untouched.
func TestApprovalsPreference(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	token := s.signIn(t, "ada@example.com", "Ada").Token
	body := map[string]any{"notifications": map[string]any{"approvals": false}}
	s.call(t, http.MethodPatch, platform.Prefix+"/me", token, body).want(t, http.StatusOK)
	notifications := data[map[string]any](t, s.call(t, http.MethodGet, platform.Prefix+"/me", token, nil).want(t, http.StatusOK))["notifications"].(map[string]any)
	if notifications["approvals"] != false || notifications["push"].(map[string]any)["approvals"] != true {
		t.Fatalf("approvals must be off for email and on for push: %v", notifications)
	}
}
