package routes

import (
	"net/http"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func TestUpdateVersion(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	p := platform.Prefix
	ada := s.signIn(t, "ada@example.com", "Ada")
	ben := s.signIn(t, "ben@example.com", "Ben")
	g := s.group(t, ada.Token, "Flat", "ben@example.com")
	e := s.expense(t, ada.Token, g.ID, 3000, ada.User.ID, ada.User.ID, ben.User.ID)
	edit := func(total int64, at string) platform.ExpenseInput {
		return platform.ExpenseInput{Description: "Groceries", Currency: "EUR", Total: total, SplitMethod: "equal", UpdatedAt: &at,
			Payers: []platform.Payer{{UserID: ada.User.ID, Amount: total}},
			Splits: []platform.Split{{UserID: ada.User.ID}, {UserID: ben.User.ID}}}
	}
	path := p + "/expenses/" + e.ID

	s.Clock.Add(time.Minute)
	saved := data[platform.Expense](t, s.call(t, http.MethodPut, path, ben.Token, edit(4000, e.UpdatedAt)).want(t, http.StatusOK))
	if saved.UpdatedAt == e.UpdatedAt {
		t.Fatal("an edit must move updatedAt")
	}
	s.Clock.Add(time.Minute)
	if f := s.call(t, http.MethodPut, path, ada.Token, edit(5000, e.UpdatedAt)).want(t, http.StatusConflict).failure(t); f.Code != "CONFLICT" {
		t.Fatalf("want CONFLICT, got %+v", f)
	}
	if f := s.call(t, http.MethodPut, path, ada.Token, edit(5000, "yesterday")).want(t, http.StatusUnprocessableEntity).failure(t); f.Field != "updatedAt" {
		t.Fatalf("want an error on updatedAt, got %+v", f)
	}
	s.call(t, http.MethodPut, path, ada.Token, edit(5000, saved.UpdatedAt)).want(t, http.StatusOK)
}
