package expense

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/google/uuid"
)

// ExportPage is how many expenses an export reads at a time, so a large
// group is never held in memory.
const ExportPage = 200

// Table is the head of a group's export.
type Table struct {
	Name string
	// People get a column each: the current members as Users.List orders
	// them, then former members who are in an expense, by name and then id,
	// so two exports of the same group are the same file.
	People []*user.User
}

// Row is one expense of an export, amounts in minor units of Currency. Nets
// holds what each of Table.People paid minus what they owed, in that order.
type Row struct {
	Date        time.Time
	Description string
	Category    string
	Kind        Kind
	Currency    string
	Total       int64
	Nets        []int64
}

// Export reads a group the actor belongs to for a spreadsheet: head is called
// once with the columns, then row once per live expense, oldest first. The
// expenses are read twice, a page at a time: once to find the people who
// need a column, once for the rows.
func (s *Service) Export(ctx context.Context, actor, groupID uuid.UUID, head func(Table) error, row func(Row) error) error {
	g, err := s.groups.Member(ctx, actor, groupID)
	if err != nil {
		return err
	}
	members, err := s.users.List(ctx, g.Members)
	if err != nil {
		return err
	}
	seen := map[uuid.UUID]bool{}
	for _, m := range g.Members {
		seen[m] = true
	}
	var others []uuid.UUID
	err = s.each(ctx, groupID, func(e *Expense) error {
		for _, p := range e.Participants() {
			if !seen[p] {
				seen[p] = true
				others = append(others, p)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	index, err := s.users.Index(ctx, others)
	if err != nil {
		return err
	}
	former := make([]*user.User, 0, len(index))
	for _, u := range index {
		former = append(former, u)
	}
	slices.SortFunc(former, func(a, b *user.User) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID.String(), b.ID.String()))
	})
	t := Table{Name: g.Name, People: append(members, former...)}
	if err := head(t); err != nil {
		return err
	}
	return s.each(ctx, groupID, func(e *Expense) error {
		nets := make([]int64, len(t.People))
		for i, p := range t.People {
			nets[i] = e.Net(p.ID)
		}
		return row(Row{Date: e.Date, Description: e.Description, Category: e.Category, Kind: e.Kind,
			Currency: e.Currency, Total: e.Total, Nets: nets})
	})
}

// each calls fn with every live expense of the group, oldest first.
func (s *Service) each(ctx context.Context, groupID uuid.UUID, fn func(*Expense) error) error {
	var last *Expense
	for {
		page, err := s.repo.After(ctx, groupID, last, ExportPage)
		if err != nil {
			return err
		}
		for _, e := range page {
			if err := fn(e); err != nil {
				return err
			}
		}
		if len(page) < ExportPage {
			return nil
		}
		last = page[len(page)-1]
	}
}
