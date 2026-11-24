package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
)

// TestGravatarPreference stores the opt-in and builds the picture URL only
// after it is turned on.
func TestGravatarPreference(t *testing.T) {
	ctx := context.Background()
	svc, notes := setup(t, time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	if err := svc.Auth.RequestCode(ctx, "ada@example.com", ""); err != nil {
		t.Fatal(err)
	}
	login, err := svc.Auth.Verify(ctx, "ada@example.com", notes.codes["ada@example.com"], "Ada", "test")
	if err != nil {
		t.Fatal(err)
	}
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
