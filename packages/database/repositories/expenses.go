package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Expenses implements expense.Repository and balance.Ledger.
type Expenses struct{ db *database.DB }

// NewExpenses builds Expenses.
func NewExpenses(db *database.DB) *Expenses { return &Expenses{db: db} }

const expenseColumns = `e.id, e.group_id, e.receipt_id, e.created_by, e.updated_by, e.created_at, e.updated_at, e.deleted_at,
	e.total, e.tax, e.tip, e.date, e.next_date, e.kind, e.description, e.category, e.currency, e.notes, e.method, e.recurrence, e.items,
	(SELECT count(*) FROM comments c WHERE c.expense_id = e.id)`

func scanExpense(row pgx.Row) (*expense.Expense, error) {
	var e expense.Expense
	var items []byte
	err := row.Scan(&e.ID, &e.GroupID, &e.ReceiptID, &e.CreatedBy, &e.UpdatedBy, &e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
		&e.Total, &e.Tax, &e.Tip, &e.Date, &e.NextDate, &e.Kind, &e.Description, &e.Category, &e.Currency, &e.Notes, &e.Method, &e.Recurrence, &items, &e.CommentCount)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		if err := json.Unmarshal(items, &e.Items); err != nil {
			return nil, fmt.Errorf("database.expense.items.invalid: %w", err)
		}
	}
	return &e, nil
}

func (r *Expenses) Create(ctx context.Context, e *expense.Expense) error {
	items, err := itemsJSON(e.Items)
	if err != nil {
		return err
	}
	tag, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO expenses (id, group_id, receipt_id, created_by, updated_by, created_at, updated_at, deleted_at,
		total, tax, tip, date, next_date, kind, description, category, currency, notes, method, recurrence, items)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) ON CONFLICT (id) DO NOTHING`,
		e.ID, e.GroupID, e.ReceiptID, e.CreatedBy, e.UpdatedBy, e.CreatedAt, e.UpdatedAt, e.DeletedAt,
		e.Total, e.Tax, e.Tip, e.Date, e.NextDate, e.Kind, e.Description, e.Category, e.Currency, e.Notes, e.Method, e.Recurrence, items)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errs.Conflict("That expense id is already in use.")
	}
	return r.writeShares(ctx, e)
}

func itemsJSON(items []expense.Item) ([]byte, error) {
	if items == nil {
		items = []expense.Item{}
	}
	return json.Marshal(items)
}

func (r *Expenses) writeShares(ctx context.Context, e *expense.Expense) error {
	q := r.db.Q(ctx)
	if _, err := q.Exec(ctx, `DELETE FROM expense_shares WHERE expense_id=$1`, e.ID); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, s := range e.Shares {
		batch.Queue(`INSERT INTO expense_shares (expense_id, user_id, paid, owed, value) VALUES ($1,$2,$3,$4,$5)`, e.ID, s.UserID, s.Paid, s.Owed, s.Value)
	}
	return q.SendBatch(ctx, batch).Close()
}

func (r *Expenses) Update(ctx context.Context, e *expense.Expense) error {
	items, err := itemsJSON(e.Items)
	if err != nil {
		return err
	}
	_, err = r.db.Q(ctx).Exec(ctx, `UPDATE expenses SET receipt_id=$2, updated_by=$3, updated_at=$4, deleted_at=$5, total=$6, tax=$7, tip=$8,
		date=$9, next_date=$10, description=$11, category=$12, currency=$13, notes=$14, method=$15, recurrence=$16, items=$17 WHERE id=$1`,
		e.ID, e.ReceiptID, e.UpdatedBy, e.UpdatedAt, e.DeletedAt, e.Total, e.Tax, e.Tip, e.Date, e.NextDate, e.Description, e.Category, e.Currency,
		e.Notes, e.Method, e.Recurrence, items)
	if err != nil {
		return err
	}
	return r.writeShares(ctx, e)
}

func (r *Expenses) Get(ctx context.Context, id uuid.UUID) (*expense.Expense, error) {
	return r.one(ctx, `SELECT `+expenseColumns+` FROM expenses e WHERE e.id=$1`, id)
}

func (r *Expenses) Lock(ctx context.Context, id uuid.UUID) (*expense.Expense, error) {
	return r.one(ctx, `SELECT `+expenseColumns+` FROM expenses e WHERE e.id=$1 FOR UPDATE OF e`, id)
}

func (r *Expenses) one(ctx context.Context, query string, id uuid.UUID) (*expense.Expense, error) {
	e, err := scanExpense(r.db.Q(ctx).QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("Expense")
	}
	if err != nil {
		return nil, err
	}
	if err := r.attachShares(ctx, []*expense.Expense{e}); err != nil {
		return nil, err
	}
	return e, nil
}

func (r *Expenses) attachShares(ctx context.Context, list []*expense.Expense) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(list))
	byID := make(map[uuid.UUID]*expense.Expense, len(list))
	for i, e := range list {
		ids[i] = e.ID
		byID[e.ID] = e
		e.Shares = nil
	}
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT expense_id, user_id, paid, owed, value FROM expense_shares WHERE expense_id = ANY($1) ORDER BY expense_id, user_id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var s expense.Share
		if err := rows.Scan(&id, &s.UserID, &s.Paid, &s.Owed, &s.Value); err != nil {
			return err
		}
		byID[id].Shares = append(byID[id].Shares, s)
	}
	return rows.Err()
}

// visible restricts to expenses $1 may see.
const visible = `((e.group_id IS NOT NULL AND EXISTS (SELECT 1 FROM group_members m WHERE m.group_id = e.group_id AND m.user_id = $1 AND m.left_at IS NULL))
	OR EXISTS (SELECT 1 FROM expense_shares s WHERE s.expense_id = e.id AND s.user_id = $1))`

func (r *Expenses) List(ctx context.Context, f expense.Filter) ([]*expense.Expense, int, error) {
	where := []string{visible}
	args := []any{f.UserID}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if !f.IncludeDeleted {
		where = append(where, "e.deleted_at IS NULL")
	}
	if f.GroupID != nil {
		where = append(where, "e.group_id = "+arg(*f.GroupID))
	}
	if f.FriendID != nil {
		where = append(where, "EXISTS (SELECT 1 FROM expense_shares s WHERE s.expense_id = e.id AND s.user_id = $1)",
			"EXISTS (SELECT 1 FROM expense_shares s WHERE s.expense_id = e.id AND s.user_id = "+arg(*f.FriendID)+")")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		p := arg("%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%")
		where = append(where, "(e.description ILIKE "+p+" OR e.notes ILIKE "+p+")")
	}
	if f.Category != "" {
		where = append(where, "e.category = "+arg(f.Category))
	}
	if f.Kind != "" {
		where = append(where, "e.kind = "+arg(string(f.Kind)))
	}
	if f.From != nil {
		where = append(where, "e.date >= "+arg(*f.From))
	}
	if f.To != nil {
		where = append(where, "e.date <= "+arg(*f.To))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM expenses e WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := arg(f.Limit), arg(f.Offset)
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+expenseColumns+` FROM expenses e WHERE `+cond+
		` ORDER BY e.date DESC, e.created_at DESC LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, 0, err
	}
	list, err := collect(rows, scanExpense)
	if err != nil {
		return nil, 0, err
	}
	return list, total, r.attachShares(ctx, list)
}

func (r *Expenses) Due(ctx context.Context, day time.Time, limit int) ([]*expense.Expense, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT `+expenseColumns+` FROM expenses e
		WHERE e.deleted_at IS NULL AND e.recurrence <> 'none' AND e.next_date <= $1 ORDER BY e.next_date, e.id LIMIT $2`, day, limit)
	if err != nil {
		return nil, err
	}
	list, err := collect(rows, scanExpense)
	if err != nil {
		return nil, err
	}
	return list, r.attachShares(ctx, list)
}

func (r *Expenses) Claim(ctx context.Context, id uuid.UUID, day time.Time) (*expense.Expense, error) {
	e, err := scanExpense(r.db.Q(ctx).QueryRow(ctx, `SELECT `+expenseColumns+` FROM expenses e
		WHERE e.id=$1 AND e.deleted_at IS NULL AND e.recurrence <> 'none' AND e.next_date <= $2 FOR UPDATE OF e SKIP LOCKED`, id, day))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := r.attachShares(ctx, []*expense.Expense{e}); err != nil {
		return nil, err
	}
	return e, nil
}

func (r *Expenses) EndSeries(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.db.Q(ctx).Exec(ctx, `UPDATE expenses SET recurrence='none', next_date=NULL, updated_at=$2 WHERE id=$1`, id, at)
	return err
}

// Ledger reads entries for balance computations.

func (r *Expenses) entries(ctx context.Context, where string, args ...any) (map[uuid.UUID][]balance.Entry, []balance.Entry, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT e.id, e.group_id, e.currency, s.user_id, s.paid, s.owed
		FROM expenses e JOIN expense_shares s ON s.expense_id = e.id
		WHERE e.deleted_at IS NULL AND `+where+` ORDER BY e.id`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	byGroup := map[uuid.UUID][]balance.Entry{}
	var flat []balance.Entry
	var current uuid.UUID
	var entry *balance.Entry
	flush := func() {
		if entry == nil {
			return
		}
		flat = append(flat, *entry)
		if entry.GroupID != nil {
			byGroup[*entry.GroupID] = append(byGroup[*entry.GroupID], *entry)
		}
	}
	for rows.Next() {
		var id uuid.UUID
		var gid *uuid.UUID
		var currency string
		var s balance.Share
		if err := rows.Scan(&id, &gid, &currency, &s.UserID, &s.Paid, &s.Owed); err != nil {
			return nil, nil, err
		}
		if entry == nil || id != current {
			flush()
			current = id
			entry = &balance.Entry{GroupID: gid, Currency: currency}
		}
		entry.Shares = append(entry.Shares, s)
	}
	flush()
	return byGroup, flat, rows.Err()
}

func (r *Expenses) Group(ctx context.Context, groupID uuid.UUID) ([]balance.Entry, error) {
	_, flat, err := r.entries(ctx, "e.group_id = $1", groupID)
	return flat, err
}

func (r *Expenses) GroupsEntries(ctx context.Context, groupIDs []uuid.UUID) (map[uuid.UUID][]balance.Entry, error) {
	if len(groupIDs) == 0 {
		return map[uuid.UUID][]balance.Entry{}, nil
	}
	byGroup, _, err := r.entries(ctx, "e.group_id = ANY($1)", groupIDs)
	return byGroup, err
}

func (r *Expenses) NonGroup(ctx context.Context, userID uuid.UUID) ([]balance.Entry, error) {
	_, flat, err := r.entries(ctx, "e.group_id IS NULL AND EXISTS (SELECT 1 FROM expense_shares x WHERE x.expense_id = e.id AND x.user_id = $1)", userID)
	return flat, err
}

func (r *Expenses) Groups(ctx context.Context, userID uuid.UUID) ([]balance.GroupRef, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT g.id, g.name, g.simplify_debts FROM groups g
		JOIN group_members m ON m.group_id = g.id AND m.user_id = $1 AND m.left_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(row pgx.Row) (balance.GroupRef, error) {
		var g balance.GroupRef
		err := row.Scan(&g.ID, &g.Name, &g.Simplify)
		return g, err
	})
}

// Comments implements expense.CommentRepository.
type Comments struct{ db *database.DB }

// NewComments builds Comments.
func NewComments(db *database.DB) *Comments { return &Comments{db: db} }

func scanComment(row pgx.Row) (*expense.Comment, error) {
	var c expense.Comment
	err := row.Scan(&c.ID, &c.ExpenseID, &c.UserID, &c.CreatedAt, &c.Body)
	return &c, err
}

func (r *Comments) Create(ctx context.Context, c *expense.Comment) error {
	_, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO comments (id, expense_id, user_id, created_at, body) VALUES ($1,$2,$3,$4,$5)`,
		c.ID, c.ExpenseID, c.UserID, c.CreatedAt, c.Body)
	return err
}

func (r *Comments) Get(ctx context.Context, id uuid.UUID) (*expense.Comment, error) {
	c, err := scanComment(r.db.Q(ctx).QueryRow(ctx, `SELECT id, expense_id, user_id, created_at, body FROM comments WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFound("Comment")
	}
	return c, err
}

func (r *Comments) List(ctx context.Context, expenseID uuid.UUID) ([]*expense.Comment, error) {
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT id, expense_id, user_id, created_at, body FROM comments WHERE expense_id=$1 ORDER BY created_at`, expenseID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanComment)
}

func (r *Comments) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM comments WHERE id=$1`, id)
	return err
}
