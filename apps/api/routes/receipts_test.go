package routes

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

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
// written as plain decimal text however they were entered.
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
			if sp.Value == nil || *sp.Value != want[sp.UserID] {
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

// TestLostReceiptImage checks that a receipt whose image is gone from
// storage answers a plain 404 error, with none of the image's headers.
func TestLostReceiptImage(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada")
	rc := data[platform.Receipt](t, s.upload(t, platform.Prefix+"/receipts", ada.Token, pixel(t)).want(t, http.StatusCreated))
	path := platform.Prefix + "/receipts/" + rc.ID
	ok := s.call(t, http.MethodGet, path, ada.Token, nil).want(t, http.StatusOK)
	if ct := ok.header.Get("Content-Type"); ct != "image/png" || !bytes.Equal(ok.body, pixel(t)) {
		t.Fatalf("want the image back, got %s %d bytes", ct, len(ok.body))
	}

	ctx := context.Background()
	var key string
	if err := s.DB.Pool.QueryRow(ctx, `SELECT storage_key FROM receipts WHERE id = $1`, rc.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if err := s.Blobs.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	res := s.call(t, http.MethodGet, path, ada.Token, nil).want(t, http.StatusNotFound)
	if e := res.failure(t); e.Code != "NOT_FOUND" {
		t.Fatalf("want NOT_FOUND, got %+v", e)
	}
	if ct := res.header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("want a JSON error, got Content-Type %q", ct)
	}
	if cc := res.header.Get("Cache-Control"); strings.Contains(cc, "max-age") {
		t.Fatalf("an error must not be cached, got Cache-Control %q", cc)
	}
}

// unreadable is a blob store whose images open but cannot be read.
type unreadable struct{ ports.BlobStore }

func (unreadable) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(iotest.ErrReader(errors.New("storage.read.failed"))), nil
}

// TestUnreadableReceiptImage checks that an image the store cannot read
// answers an error, with none of the image's headers.
func TestUnreadableReceiptImage(t *testing.T) {
	s := newServer(t, bootstraptest.Options{Blobs: func(b ports.BlobStore) ports.BlobStore { return unreadable{b} }})
	ada := s.signIn(t, "ada@example.com", "Ada")
	rc := data[platform.Receipt](t, s.upload(t, platform.Prefix+"/receipts", ada.Token, pixel(t)).want(t, http.StatusCreated))
	res := s.call(t, http.MethodGet, platform.Prefix+"/receipts/"+rc.ID, ada.Token, nil).want(t, http.StatusInternalServerError)
	if e := res.failure(t); e.Code != "INTERNAL" {
		t.Fatalf("want INTERNAL, got %+v", e)
	}
	if ct, cc := res.header.Get("Content-Type"), res.header.Get("Cache-Control"); ct != "application/json; charset=utf-8" || strings.Contains(cc, "max-age") {
		t.Fatalf("want an uncached JSON error, got Content-Type %q, Cache-Control %q", ct, cc)
	}
}
