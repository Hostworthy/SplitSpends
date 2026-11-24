package api

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/atb00ker/splitspends/backend/internal/core/domains/receipt"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/platform"
)

func (a *API) uploadReceipt(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, receipt.MaxSize+(1<<20))
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		return errs.Validation("file", "Upload the image as multipart form field \"file\" (max 10 MB).")
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("file")
	if err != nil {
		return errs.Validation("file", "Upload the image as multipart form field \"file\".")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, receipt.MaxSize+1))
	if err != nil {
		return err
	}
	rc, err := a.svc.Receipts.Upload(r.Context(), me(r), data)
	if err != nil {
		return err
	}
	return details(w, http.StatusCreated, platform.Receipt{ID: rc.ID.String(), ContentType: rc.ContentType, Size: rc.Size, CreatedAt: stamp(rc.CreatedAt)})
}

func (a *API) getReceipt(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "receiptId", "Receipt")
	if err != nil {
		return err
	}
	rc, body, err := a.svc.Receipts.Open(r.Context(), me(r), id)
	if err != nil {
		return err
	}
	defer body.Close()
	w.Header().Set("Content-Type", rc.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(rc.Size, 10))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	_, err = io.Copy(w, body)
	return err
}

func (a *API) scanReceipt(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "receiptId", "Receipt")
	if err != nil {
		return err
	}
	in := platform.ScanRequest{}
	if r.ContentLength > 0 {
		if in, err = decode[platform.ScanRequest](r); err != nil {
			return err
		}
	}
	s, err := a.svc.Receipts.Scan(r.Context(), me(r), id, in.Currency)
	if err != nil {
		return err
	}
	out := platform.ReceiptScan{Merchant: s.Merchant, Currency: s.Currency, Items: []platform.ScanItem{}, Subtotal: s.Subtotal,
		Tax: s.Tax, Tip: s.Tip, Total: s.Total, Category: s.Category}
	if s.Date != "" {
		out.Date = &s.Date
	}
	for _, it := range s.Items {
		out.Items = append(out.Items, platform.ScanItem{Name: it.Name, Quantity: it.Quantity, Amount: it.Amount})
	}
	return details(w, http.StatusOK, out)
}

func (a *API) draftExpense(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.DraftRequest](r)
	if err != nil {
		return err
	}
	groupID, err := optionalID(in.GroupID, "groupId")
	if err != nil {
		return err
	}
	d, err := a.svc.Assist.Draft(r.Context(), me(r), in.Text, groupID)
	if err != nil {
		return err
	}
	out := platform.ExpenseDraft{GroupID: in.GroupID, Description: d.Description, Currency: d.Currency, Total: d.Total, Date: d.Date,
		Category: d.Category, SplitMethod: "equal", Payers: []platform.Payer{}, Splits: []platform.Split{}, Warnings: d.Warnings}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	if _, err := time.Parse(time.DateOnly, out.Date); err != nil {
		out.Date = time.Now().UTC().Format(time.DateOnly)
	}
	for _, p := range d.Payers {
		out.Payers = append(out.Payers, platform.Payer{UserID: p.UserID.String(), Amount: p.Amount})
	}
	for _, s := range d.Splits {
		out.Splits = append(out.Splits, platform.Split{UserID: s.UserID.String()})
	}
	return details(w, http.StatusOK, out)
}
