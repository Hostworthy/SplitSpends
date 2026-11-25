package repositories_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
)

// TestFeedUsesInvolvedIndex checks that the feed is found through the index
// on who is involved, so its cost follows the caller's own activity rather
// than everyone's.
func TestFeedUsesInvolvedIndex(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	// Tens of thousands of entries, each involving Ada and someone else, a few of
	// them also the person whose feed is read.
	ben := uuid.New()
	_, err := env.DB.Pool.Exec(ctx, `INSERT INTO activities (id, actor_id, created_at, kind, involved)
		SELECT gen_random_uuid(), $1, now() - n * interval '1 minute', 'expenseCreated',
			CASE WHEN n % 5000 = 0 THEN ARRAY[$1, $2]::uuid[] ELSE ARRAY[$1, gen_random_uuid()] END
		FROM generate_series(1, 50000) n`, ada, ben)
	if err != nil {
		t.Fatal(err)
	}
	// As autovacuum would, move the new entries out of the index's pending
	// list and gather statistics.
	if _, err := env.DB.Pool.Exec(ctx, `VACUUM ANALYZE activities`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`SELECT count(*) FROM activities WHERE ` + repositories.Feed,
		`SELECT id FROM activities WHERE ` + repositories.Feed + ` ORDER BY created_at DESC, id DESC LIMIT 20`,
	} {
		rows, err := env.DB.Pool.Query(ctx, `EXPLAIN `+query, ben, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, line)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if text := strings.Join(plan, "\n"); !strings.Contains(text, "activities_involved_idx") {
			t.Fatalf("the feed should use activities_involved_idx:\n%s", text)
		}
	}
}
