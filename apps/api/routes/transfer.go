package routes

import (
	"fmt"
	"net/http"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/category"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/transfer"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

const maxImport = 16 << 20

func (a *API) importBundle(w http.ResponseWriter, r *http.Request) error {
	in, err := decodeLimit[platform.ImportRequest](w, r, maxImport)
	if err != nil {
		return err
	}
	b := transfer.Bundle{}
	if b.Me, err = requiredID(in.MeID, "meId"); err != nil {
		return err
	}
	for i, p := range in.People {
		id, err := requiredID(p.ID, fmt.Sprintf("people[%d].id", i))
		if err != nil {
			return err
		}
		b.People = append(b.People, transfer.Person{ID: id, Name: p.Name, Email: p.Email})
	}
	for i, c := range in.Categories {
		if c.ID == "" {
			return errs.Validation(fmt.Sprintf("categories[%d].id", i), "Every imported category needs its id.")
		}
		b.Categories = append(b.Categories, transfer.Category{
			Input:   category.Input{ID: &c.ID, Name: &c.Name, Icon: &c.Icon, Group: &c.Group},
			Deleted: c.Deleted,
		})
	}
	for i, g := range in.Groups {
		at := fmt.Sprintf("groups[%d]", i)
		id, err := requiredID(g.ID, at+".id")
		if err != nil {
			return err
		}
		patch, err := toPatch(g.GroupPatch)
		if err != nil {
			return nest(err, at)
		}
		group := transfer.Group{ID: id, Patch: patch}
		for j, m := range g.MemberIDs {
			mid, err := requiredID(m, fmt.Sprintf("%s.memberIds[%d]", at, j))
			if err != nil {
				return err
			}
			group.Members = append(group.Members, mid)
		}
		b.Groups = append(b.Groups, group)
	}
	for i, e := range in.Expenses {
		at := fmt.Sprintf("expenses[%d]", i)
		if e.ID == nil {
			return errs.Validation(at+".id", "Every imported expense needs its id.")
		}
		input, err := toInput(e.ExpenseInput)
		if err != nil {
			return nest(err, at)
		}
		kind := expense.Kind(e.Kind)
		if kind == "" {
			kind = expense.KindExpense
		}
		if kind != expense.KindExpense && kind != expense.KindPayment {
			return errs.Validation(at+".kind", "Kind is expense or payment.")
		}
		b.Expenses = append(b.Expenses, transfer.Expense{Kind: kind, Input: input})
	}
	res, err := a.svc.Transfer.Import(r.Context(), me(r), b)
	if err != nil {
		return err
	}
	out := platform.ImportResult{People: map[string]string{}, Categories: res.Categories, Groups: res.Groups, Expenses: res.Expenses}
	for local, server := range res.People {
		out.People[local.String()] = server.String()
	}
	return details(w, http.StatusOK, out)
}
