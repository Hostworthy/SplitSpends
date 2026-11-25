package routes

import (
	"log/slog"
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

// TestGravatarFollowsHost checks that turning Gravatar on works and shows a
// picture only where the host allows pictures, and that building another API
// with another configuration in the same process changes nothing.
func TestGravatarFollowsHost(t *testing.T) {
	yes := true
	enable := platform.MePatch{Gravatar: &yes}
	t.Run("pictures on", func(t *testing.T) {
		on := bootstraptest.Defaults()
		on.Gravatar = true
		s := newServer(t, bootstraptest.Options{Config: on})
		off := s.Config.Settings()
		off.Gravatar = false
		New(Deps{Services: s.Services, Settings: off, Log: slog.New(slog.DiscardHandler)})
		token := s.signIn(t, "ada@example.com", "Ada").Token
		me := data[platform.Me](t, s.call(t, http.MethodPatch, platform.Prefix+"/me", token, enable).want(t, http.StatusOK))
		if !me.Gravatar || me.AvatarURL == nil {
			t.Fatalf("want a picture, got %+v", me)
		}
	})
	t.Run("pictures off", func(t *testing.T) {
		s := newServer(t, bootstraptest.Options{})
		token := s.signIn(t, "ada@example.com", "Ada").Token
		f := s.call(t, http.MethodPatch, platform.Prefix+"/me", token, enable).want(t, http.StatusUnprocessableEntity).failure(t)
		if f.Field != "gravatar" {
			t.Fatalf("want an error on gravatar, got %+v", f)
		}
	})
}
