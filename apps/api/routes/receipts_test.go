package routes

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/receipt"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// rent is an AI reader that reads every request as rent of 1000 in the
// group's currency, naming nobody.
type rent struct{}

func (rent) ReadReceipt(context.Context, ports.ReceiptInput) (ports.ReceiptExtract, error) {
	return ports.ReceiptExtract{Total: "1000"}, nil
}

func (rent) DraftExpense(context.Context, ports.DraftInput) (ports.DraftExtract, error) {
	return ports.DraftExtract{Description: "Rent", Amount: "1000"}, nil
}

// TestDraftUsesGroupDefaultSplit checks that a draft in a group with a
// percent or shares default split starts with that split and its values,
// written as JSON numbers however they were entered.
func TestDraftUsesGroupDefaultSplit(t *testing.T) {
	s := newServer(t, bootstraptest.Options{Reader: rent{}})
	ada := s.signIn(t, "ada@example.com", "Ada")
	ben := s.signIn(t, "ben@example.com", "Ben")
	g := s.group(t, ada.Token, "Flat", "ben@example.com")
	for _, c := range []struct {
		method           string
		ada, ben         string
		wantAda, wantBen string
	}{
		{"percent", "60", "40", "60", "40"},
		{"shares", ".5", "1.", "0.5", "1"},
	} {
		preset := platform.DefaultSplit{Method: c.method, Values: map[string]string{ada.User.ID: c.ada, ben.User.ID: c.ben}}
		s.call(t, http.MethodPatch, platform.Prefix+"/groups/"+g.ID, ada.Token,
			platform.GroupPatch{DefaultSplit: platform.Nullable[platform.DefaultSplit]{Set: true, Value: &preset}}).want(t, http.StatusOK)

		d := data[platform.ExpenseDraft](t, s.call(t, http.MethodPost, platform.Prefix+"/ai/expense-draft", ada.Token,
			platform.DraftRequest{Text: "rent 1000", GroupID: &g.ID}).want(t, http.StatusOK))
		if d.SplitMethod != c.method || len(d.Splits) != 2 {
			t.Fatalf("want the %s default, got %s %+v", c.method, d.SplitMethod, d.Splits)
		}
		want := map[string]string{ada.User.ID: c.wantAda, ben.User.ID: c.wantBen}
		for _, sp := range d.Splits {
			if sp.Value == nil || sp.Value.String() != want[sp.UserID] {
				t.Fatalf("want the default's values %v, got %+v", want, d.Splits)
			}
		}
	}
}

// TestUploadRefusals checks that a body above the upload limit, a form
// without the file field and a file that is not an image are refused with a
// validation error on file, and that nothing is stored for them.
func TestUploadRefusals(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada")
	path := platform.Prefix + "/receipts"

	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	if err := w.WriteField("note", "no file here"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, s.url+path, &form)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	for _, tc := range []struct {
		name string
		res  *reply
	}{
		{"too large", s.upload(t, path, ada.Token, append(pixel(t), make([]byte, receipt.MaxSize+(1<<20))...))},
		{"no file field", s.send(t, req, ada.Token)},
		{"not an image", s.upload(t, path, ada.Token, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"))},
	} {
		tc.res.want(t, http.StatusUnprocessableEntity)
		if e := tc.res.failure(t); e.Code != "VALIDATION_ERROR" || e.Field != "file" {
			t.Errorf("%s: want a validation error on file, got %+v", tc.name, e)
		}
	}
	var stored int
	if err := s.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM receipts`).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("nothing may be stored: %d receipts, %v", stored, err)
	}
}
