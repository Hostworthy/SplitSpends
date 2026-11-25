package repositories_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
)

// TestUnreadableJSONIsAnError checks that stored JSON the repositories cannot
// decode is reported rather than replaced with defaults.
func TestUnreadableJSONIsAnError(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE users SET notifications = '[true]'::jsonb WHERE id = $1`, ada); err != nil {
		t.Fatal(err)
	}
	if _, err := repositories.NewUsers(env.DB).Get(ctx, ada); err == nil || !strings.Contains(err.Error(), "database.user.notifications.invalid") {
		t.Errorf("unreadable notification preferences: got %v", err)
	}
	if _, err := env.DB.Pool.Exec(ctx, `INSERT INTO activities (id, actor_id, created_at, kind, involved, data)
		VALUES ($1, $2, now(), 'expenseCreated', ARRAY[$2]::uuid[], '[1]'::jsonb)`, uuid.New(), ada); err != nil {
		t.Fatal(err)
	}
	_, _, err := repositories.NewActivities(env.DB).List(ctx, activity.Filter{UserID: ada, Limit: 10})
	if err == nil || !strings.Contains(err.Error(), "database.activity.data.invalid") {
		t.Errorf("unreadable activity data: got %v", err)
	}
}
