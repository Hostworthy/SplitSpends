package routes

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

// TestListsPage checks that lists which used to return everything take
// limit and offset, report the full total, and still return everything when
// a client sends no limit.
func TestListsPage(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	p := platform.Prefix
	ada := s.signIn(t, "ada@example.com", "Ada")
	ben := s.signIn(t, "ben@example.com", "Ben")
	g := s.group(t, ada.Token, "Flat", "ben@example.com")
	s.group(t, ada.Token, "Trip")
	s.group(t, ada.Token, "Club")
	e := s.expense(t, ada.Token, g.ID, 3000, ada.User.ID, ada.User.ID, ben.User.ID)
	for i := range 12 {
		s.Clock.Add(time.Second)
		s.call(t, http.MethodPost, p+"/expenses/"+e.ID+"/comments", ada.Token,
			platform.CommentInput{Body: fmt.Sprintf("Note %d", i+1)}).want(t, http.StatusCreated)
	}
	for _, name := range []string{"Cy", "Dan"} {
		s.call(t, http.MethodPost, p+"/friends", ada.Token, platform.Invitee{Name: name}).want(t, http.StatusCreated)
	}
	for range 2 {
		s.call(t, http.MethodPost, p+"/groups/"+g.ID+"/invites", ada.Token, nil).want(t, http.StatusCreated)
	}

	comments := func(query string) platform.List[platform.Comment] {
		return items[platform.Comment](t, s.call(t, http.MethodGet, p+"/expenses/"+e.ID+"/comments"+query, ben.Token, nil).want(t, http.StatusOK))
	}
	if l := comments("?limit=10"); len(l.Items) != 10 || l.Total != 12 || l.Items[0].Body != "Note 1" {
		t.Fatalf("want the first 10 of 12 comments, got %d of %d", len(l.Items), l.Total)
	}
	if l := comments("?limit=10&offset=10"); len(l.Items) != 2 || l.Total != 12 || l.Items[1].Body != "Note 12" {
		t.Fatalf("want the last 2 of 12 comments, got %d of %d", len(l.Items), l.Total)
	}

	for _, c := range []struct {
		path        string
		total, page int
	}{
		{"/expenses/" + e.ID + "/comments", 12, 5},
		{"/groups", 3, 2},
		{"/friends", 3, 2},
		{"/groups/" + g.ID + "/invites", 2, 1},
	} {
		all := items[any](t, s.call(t, http.MethodGet, p+c.path, ada.Token, nil).want(t, http.StatusOK))
		if len(all.Items) != c.total || all.Total != c.total {
			t.Errorf("GET %s without a limit: want all %d, got %d of %d", c.path, c.total, len(all.Items), all.Total)
		}
		paged := items[any](t, s.call(t, http.MethodGet, p+c.path+"?limit="+fmt.Sprint(c.page)+"&offset=1", ada.Token, nil).want(t, http.StatusOK))
		if len(paged.Items) != min(c.page, c.total-1) || paged.Total != c.total {
			t.Errorf("GET %s?limit=%d&offset=1: want %d of %d, got %d of %d", c.path, c.page, min(c.page, c.total-1), c.total, len(paged.Items), paged.Total)
		}
	}
}

// TestExportPages checks that an export reads a group of more expenses than
// one page holds and writes every one of them, oldest first.
func TestExportPages(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada")
	ben := s.signIn(t, "ben@example.com", "Ben")
	g := s.group(t, ada.Token, "Flat", "ben@example.com")
	groupID, adaID, benID := uuid.MustParse(g.ID), uuid.MustParse(ada.User.ID), uuid.MustParse(ben.User.ID)
	const count = expense.ExportPage + 1
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range count {
		_, err := s.Services.Expenses.Create(context.Background(), adaID, expense.Input{GroupID: &groupID,
			Description: fmt.Sprintf("Item %d", i), Currency: "EUR", Total: 1000, Date: start.AddDate(0, 0, i).Format(time.DateOnly),
			Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: adaID, Amount: 1000}},
			Splits: []expense.SplitInput{{UserID: adaID}, {UserID: benID}}})
		if err != nil {
			t.Fatal(err)
		}
	}

	res := s.call(t, http.MethodGet, platform.Prefix+"/groups/"+g.ID+"/export", ben.Token, nil).want(t, http.StatusOK)
	rows, err := csv.NewReader(strings.NewReader(string(res.body))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != count+1 || strings.Join(rows[0], ",") != "Date,Description,Category,Kind,Currency,Cost,Ada,Ben" {
		t.Fatalf("want a header and %d rows, got %d rows starting %v", count, len(rows), rows[0])
	}
	for i, row := range rows[1:] {
		if row[1] != fmt.Sprintf("Item %d", i) || row[6] != "5.00" || row[7] != "-5.00" {
			t.Fatalf("row %d: want Item %d with Ada +5.00 and Ben -5.00, got %v", i+1, i, row)
		}
	}
}

// failingPages reads expenses from the database but fails the read with
// number fail, counting from one.
type failingPages struct {
	*repositories.Expenses
	calls, fail int
}

func (f *failingPages) After(ctx context.Context, groupID uuid.UUID, last *expense.Expense, limit int) ([]*expense.Expense, error) {
	f.calls++
	if f.calls == f.fail {
		return nil, errors.New("connection lost")
	}
	return f.Expenses.After(ctx, groupID, last, limit)
}

// TestExportFailureAbortsDownload checks that an export which fails after
// rows were sent ends the download as broken, instead of adding an error
// body to a file that would then look complete.
func TestExportFailureAbortsDownload(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada")
	g := s.group(t, ada.Token, "Flat")
	groupID, adaID := uuid.MustParse(g.ID), uuid.MustParse(ada.User.ID)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range expense.ExportPage + 1 {
		_, err := s.Services.Expenses.Create(context.Background(), adaID, expense.Input{GroupID: &groupID,
			Description: fmt.Sprintf("Item %d", i), Currency: "EUR", Total: 1000, Date: start.AddDate(0, 0, i).Format(time.DateOnly),
			Method: expense.MethodEqual, Payers: []expense.Payer{{UserID: adaID, Amount: 1000}},
			Splits: []expense.SplitInput{{UserID: adaID}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Two reads find the columns; the second page of rows fails after the
	// first page was written out.
	s.Services.Expenses = expense.NewService(expense.Deps{Repo: &failingPages{Expenses: repositories.NewExpenses(s.DB), fail: 4},
		Groups: s.Services.Groups, Users: s.Services.Users})

	req := s.request(t, http.MethodGet, platform.Prefix+"/groups/"+g.ID+"/export", nil)
	req.Header.Set("Authorization", "Bearer "+ada.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err == nil {
		t.Fatalf("want the download to break off, got status %d and a complete body ending %q", res.StatusCode, body[max(0, len(body)-120):])
	}
	if strings.Contains(string(body), `"code"`) {
		t.Errorf("an error body was added to the file: %q", body[max(0, len(body)-120):])
	}
}
