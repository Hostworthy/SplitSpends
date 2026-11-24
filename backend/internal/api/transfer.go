package api

import (
	"net/http"

	"github.com/atb00ker/splitspends/backend/internal/core/domains/expense"
	"github.com/atb00ker/splitspends/backend/internal/core/domains/transfer"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/platform"
	"github.com/google/uuid"
)

const maxImport = 16 << 20

func (a *API) importBundle(w http.ResponseWriter, r *http.Request) error {
	in, err := decodeLimit[platform.ImportRequest](r, maxImport)
	if err != nil {
		return err
	}
	b := transfer.Bundle{}
	if b.Me, err = uuid.Parse(in.MeID); err != nil {
		return errs.Validation("meId", "Invalid id.")
	}
	for _, p := range in.People {
		id, err := uuid.Parse(p.ID)
		if err != nil {
			return errs.Validation("people", "Invalid person id.")
		}
		b.People = append(b.People, transfer.Person{ID: id, Name: p.Name, Email: p.Email})
	}
	for _, g := range in.Groups {
		id, err := uuid.Parse(g.ID)
		if err != nil {
			return errs.Validation("groups", "Invalid group id.")
		}
		group := transfer.Group{ID: id, Patch: toPatch(g.GroupPatch)}
		for _, m := range g.MemberIDs {
			mid, err := uuid.Parse(m)
			if err != nil {
				return errs.Validation("groups", "Invalid member id.")
			}
			group.Members = append(group.Members, mid)
		}
		b.Groups = append(b.Groups, group)
	}
	for _, e := range in.Expenses {
		if e.ID == nil {
			return errs.Validation("expenses", "Every imported expense needs its id.")
		}
		input, err := toInput(e.ExpenseInput)
		if err != nil {
			return err
		}
		kind := expense.KindExpense
		if e.Kind == string(expense.KindPayment) {
			kind = expense.KindPayment
		}
		b.Expenses = append(b.Expenses, transfer.Expense{Kind: kind, Input: input})
	}
	res, err := a.svc.Transfer.Import(r.Context(), me(r), b)
	if err != nil {
		return err
	}
	out := platform.ImportResult{People: map[string]string{}, Groups: res.Groups, Expenses: res.Expenses}
	for local, server := range res.People {
		out.People[local.String()] = server.String()
	}
	return details(w, http.StatusOK, out)
}
