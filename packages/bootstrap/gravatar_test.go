package bootstrap_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
)

// TestGravatarPreference stores the opt-in and builds the picture URL only
// after it is turned on.
func TestGravatarPreference(t *testing.T) {
	ctx := context.Background()
	cfg := bootstraptest.Defaults()
	cfg.Gravatar = true
	env := bootstraptest.Open(t, bootstraptest.Options{Config: cfg, Now: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)})
	svc := env.Services
	login := env.SignIn(t, "ada@example.com", "Ada")
	if login.User.Gravatar || login.User.Picture(160) != "" {
		t.Fatal("Gravatar must be off for new accounts")
	}
	on := true
	if _, err := svc.Users.Update(ctx, login.User.ID, user.Patch{Gravatar: &on}); err != nil {
		t.Fatal(err)
	}
	u, err := svc.Users.Get(ctx, login.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !u.Gravatar || !strings.HasPrefix(u.Picture(160), "https://gravatar.com/avatar/") {
		t.Fatalf("preference not stored: %+v %q", u.Gravatar, u.Picture(160))
	}
}
