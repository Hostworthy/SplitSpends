package routes

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// pixel is the smallest image a receipt upload accepts.
func pixel(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestOutsidersSeeNothing books a group expense with a receipt and a comment
// and checks that someone outside the group cannot read or change any of it,
// while a member who is not part of the expense can still read it.
func TestOutsidersSeeNothing(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	p := platform.Prefix
	ada := s.signIn(t, "ada@example.com", "Ada")
	ben := s.signIn(t, "ben@example.com", "Ben")
	cy := s.signIn(t, "cy@example.com", "Cy")
	dan := s.signIn(t, "dan@example.com", "Dan")

	name, currency := "Flat", "EUR"
	g := data[platform.Group](t, s.call(t, http.MethodPost, p+"/groups", ada.Token, platform.GroupCreate{
		GroupPatch: platform.GroupPatch{Name: &name, DefaultCurrency: &currency},
		Members:    []platform.Invitee{{Email: "ben@example.com"}, {Email: "cy@example.com"}},
	}).want(t, http.StatusCreated))
	if len(g.Members) != 3 {
		t.Fatalf("want 3 members, got %+v", g.Members)
	}
	rc := data[platform.Receipt](t, s.upload(t, p+"/receipts", ada.Token, pixel(t)).want(t, http.StatusCreated))
	e := data[platform.Expense](t, s.call(t, http.MethodPost, p+"/expenses", ada.Token, platform.ExpenseInput{
		GroupID: &g.ID, Description: "Groceries", Currency: "EUR", Total: 3000, SplitMethod: "equal", ReceiptID: &rc.ID,
		Payers: []platform.Payer{{UserID: ada.User.ID, Amount: 3000}},
		Splits: []platform.Split{{UserID: ada.User.ID}, {UserID: ben.User.ID}},
	}).want(t, http.StatusCreated))
	s.call(t, http.MethodPost, p+"/expenses/"+e.ID+"/comments", ada.Token,
		platform.CommentInput{Body: "Receipt attached"}).want(t, http.StatusCreated)

	// Cy is in the group but not in the expense: members see every expense.
	s.call(t, http.MethodGet, p+"/expenses/"+e.ID, cy.Token, nil).want(t, http.StatusOK)
	s.call(t, http.MethodGet, p+"/expenses/"+e.ID+"/comments", cy.Token, nil).want(t, http.StatusOK)

	update := platform.ExpenseInput{Description: "Mine now", Currency: "EUR", Total: 3000, SplitMethod: "equal",
		Payers: []platform.Payer{{UserID: dan.User.ID, Amount: 3000}}, Splits: []platform.Split{{UserID: dan.User.ID}}}
	rename := "Dan's"
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/groups/" + g.ID, nil},
		{http.MethodPatch, "/groups/" + g.ID, platform.GroupPatch{Name: &rename}},
		{http.MethodDelete, "/groups/" + g.ID, nil},
		{http.MethodPost, "/groups/" + g.ID + "/members", platform.Invitee{Email: "eve@example.com"}},
		{http.MethodDelete, "/groups/" + g.ID + "/members/" + ben.User.ID, nil},
		{http.MethodGet, "/groups/" + g.ID + "/balances", nil},
		{http.MethodGet, "/groups/" + g.ID + "/export", nil},
		{http.MethodGet, "/groups/" + g.ID + "/invites", nil},
		{http.MethodPost, "/groups/" + g.ID + "/invites", nil},
		{http.MethodGet, "/expenses/" + e.ID, nil},
		{http.MethodPut, "/expenses/" + e.ID, update},
		{http.MethodDelete, "/expenses/" + e.ID, nil},
		{http.MethodPost, "/expenses/" + e.ID + "/restore", nil},
		{http.MethodGet, "/expenses/" + e.ID + "/comments", nil},
		{http.MethodPost, "/expenses/" + e.ID + "/comments", platform.CommentInput{Body: "Hello"}},
		{http.MethodGet, "/receipts/" + rc.ID, nil},
	} {
		res := s.call(t, tc.method, p+tc.path, dan.Token, tc.body)
		if res.status != http.StatusNotFound {
			t.Errorf("outsider %s %s: want 404, got %d: %s", tc.method, tc.path, res.status, res.body)
			continue
		}
		if e := res.failure(t); e.Code != "NOT_FOUND" || e.Error == "" || e.RequestID == "" {
			t.Errorf("outsider %s %s: unexpected error envelope %+v", tc.method, tc.path, e)
		}
	}

	// Nothing the outsider tried changed anything, and lists stay private.
	got := data[platform.Expense](t, s.call(t, http.MethodGet, p+"/expenses/"+e.ID, ada.Token, nil).want(t, http.StatusOK))
	if got.Description != "Groceries" {
		t.Fatalf("an outsider changed the expense: %+v", got)
	}
	if l := items[platform.Group](t, s.call(t, http.MethodGet, p+"/groups", dan.Token, nil).want(t, http.StatusOK)); l.Total != 0 {
		t.Fatalf("outsider lists groups: %+v", l)
	}
	if l := items[platform.Expense](t, s.call(t, http.MethodGet, p+"/expenses", dan.Token, nil).want(t, http.StatusOK)); l.Total != 0 {
		t.Fatalf("outsider lists expenses: %+v", l)
	}
	if l := items[platform.Comment](t, s.call(t, http.MethodGet, p+"/expenses/"+e.ID+"/comments", ada.Token, nil).want(t, http.StatusOK)); l.Total != 1 {
		t.Fatalf("want Ada's comment only, got %+v", l)
	}
}

// TestErrorEnvelope checks the status and body for each kind of failure a
// client sees through the full middleware chain.
func TestErrorEnvelope(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	p := platform.Prefix
	ada := s.signIn(t, "ada@example.com", "Ada")

	for _, tc := range []struct {
		name, method, path, token string
		body                      any
		status                    int
		code, field               string
	}{
		{"unknown endpoint", http.MethodGet, p + "/nothing-here", "", nil, http.StatusNotFound, "NOT_FOUND", ""},
		{"no token", http.MethodGet, p + "/me", "", nil, http.StatusUnauthorized, "UNAUTHORIZED", ""},
		{"empty body", http.MethodPost, p + "/groups", ada.Token, nil, http.StatusUnprocessableEntity, "VALIDATION_ERROR", ""},
		{"wrong type", http.MethodPost, p + "/expenses", ada.Token, map[string]any{"total": "ten"}, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "total"},
		{"bad id", http.MethodGet, p + "/groups/not-an-id", ada.Token, nil, http.StatusNotFound, "NOT_FOUND", ""},
		{"not admin", http.MethodGet, p + "/admin/accounts", s.signIn(t, "ben@example.com", "Ben").Token, nil, http.StatusForbidden, "FORBIDDEN", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := s.call(t, tc.method, tc.path, tc.token, tc.body).want(t, tc.status)
			if ct := res.header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type %q", ct)
			}
			e := res.failure(t)
			if e.Code != tc.code || e.Field != tc.field || e.Error == "" {
				t.Errorf("want code %s field %q, got %+v", tc.code, tc.field, e)
			}
			if e.RequestID == "" || e.RequestID != res.header.Get("X-Request-Id") {
				t.Errorf("requestId %q must match the X-Request-Id header %q", e.RequestID, res.header.Get("X-Request-Id"))
			}
		})
	}
}
