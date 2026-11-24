package repositories

import (
	"context"
	"encoding/json"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/stats"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/jackc/pgx/v5"
)

// Activities implements activity.Repository.
type Activities struct{ db *database.DB }

// NewActivities builds Activities.
func NewActivities(db *database.DB) *Activities { return &Activities{db: db} }

func (r *Activities) Create(ctx context.Context, a *activity.Activity) error {
	data, err := json.Marshal(a.Data)
	if err != nil {
		return err
	}
	_, err = r.db.Q(ctx).Exec(ctx, `INSERT INTO activities (id, actor_id, group_id, expense_id, created_at, kind, involved, data) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		a.ID, a.ActorID, a.GroupID, a.ExpenseID, a.CreatedAt, a.Kind, a.Involved, data)
	return err
}

func (r *Activities) List(ctx context.Context, f activity.Filter) ([]*activity.Activity, int, error) {
	cond := `$1 = ANY(involved) AND ($2::uuid IS NULL OR group_id = $2) AND ($3::uuid IS NULL OR expense_id = $3)`
	var total int
	if err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM activities WHERE `+cond, f.UserID, f.GroupID, f.ExpenseID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT id, actor_id, group_id, expense_id, created_at, kind, involved, data FROM activities
		WHERE `+cond+` ORDER BY created_at DESC LIMIT $4 OFFSET $5`, f.UserID, f.GroupID, f.ExpenseID, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	list, err := collect(rows, func(row pgx.Row) (*activity.Activity, error) {
		var a activity.Activity
		var data []byte
		if err := row.Scan(&a.ID, &a.ActorID, &a.GroupID, &a.ExpenseID, &a.CreatedAt, &a.Kind, &a.Involved, &data); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(data, &a.Data)
		return &a, nil
	})
	return list, total, err
}

// Stats implements stats.Repository.
type Stats struct{ db *database.DB }

// NewStats builds Stats.
func NewStats(db *database.DB) *Stats { return &Stats{db: db} }

func (r *Stats) Summary(ctx context.Context, q stats.Query) (*stats.Summary, error) {
	// In personal mode the amount is the caller's owed share; in group mode it
	// is the full expense total.
	var source string
	args := []any{q.Currency, q.From, q.To}
	if q.GroupID != nil {
		source = `SELECT e.category, e.date, e.total AS amount FROM expenses e
			WHERE e.group_id = $4 AND e.kind = 'expense' AND e.deleted_at IS NULL AND e.currency = $1 AND e.date BETWEEN $2 AND $3`
		args = append(args, *q.GroupID)
	} else {
		source = `SELECT e.category, e.date, s.owed AS amount FROM expenses e JOIN expense_shares s ON s.expense_id = e.id AND s.user_id = $4
			WHERE e.kind = 'expense' AND e.deleted_at IS NULL AND e.currency = $1 AND e.date BETWEEN $2 AND $3 AND s.owed > 0`
		args = append(args, q.UserID)
	}
	out := &stats.Summary{Currency: q.Currency, ByCategory: []stats.Amount{}, ByMonth: []stats.Amount{}, ByMember: []stats.Member{}}
	amounts := func(sql string) ([]stats.Amount, error) {
		rows, err := r.db.Q(ctx).Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		return collect(rows, func(row pgx.Row) (stats.Amount, error) {
			var a stats.Amount
			err := row.Scan(&a.Key, &a.Amount)
			return a, err
		})
	}
	var err error
	if out.ByCategory, err = amounts(`SELECT category, sum(amount)::bigint FROM (` + source + `) x GROUP BY category ORDER BY 2 DESC`); err != nil {
		return nil, err
	}
	if out.ByMonth, err = amounts(`SELECT to_char(date, 'YYYY-MM'), sum(amount)::bigint FROM (` + source + `) x GROUP BY 1 ORDER BY 1`); err != nil {
		return nil, err
	}
	for _, c := range out.ByCategory {
		out.Total += c.Amount
	}
	if q.GroupID != nil {
		rows, err := r.db.Q(ctx).Query(ctx, `SELECT s.user_id, sum(s.paid)::bigint, sum(s.owed)::bigint FROM expenses e JOIN expense_shares s ON s.expense_id = e.id
			WHERE e.group_id = $4 AND e.kind = 'expense' AND e.deleted_at IS NULL AND e.currency = $1 AND e.date BETWEEN $2 AND $3
			GROUP BY s.user_id ORDER BY 3 DESC`, args...)
		if err != nil {
			return nil, err
		}
		if out.ByMember, err = collect(rows, func(row pgx.Row) (stats.Member, error) {
			var m stats.Member
			err := row.Scan(&m.UserID, &m.Paid, &m.Owed)
			return m, err
		}); err != nil {
			return nil, err
		}
	}
	if out.ByCategory == nil {
		out.ByCategory = []stats.Amount{}
	}
	if out.ByMonth == nil {
		out.ByMonth = []stats.Amount{}
	}
	if out.ByMember == nil {
		out.ByMember = []stats.Member{}
	}
	return out, nil
}
