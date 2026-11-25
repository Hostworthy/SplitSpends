package routes

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/assist"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/receipt"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/middleware"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func (a *API) uploadReceipt(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, receipt.MaxSize+(1<<20))
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		return errs.Validation("file", "Upload the image as multipart form field \"file\" (max 10 MB).")
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
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
	// A store that cannot read the image fails here, while an error can
	// still be answered in place of the image.
	image := bufio.NewReader(body)
	if _, err := image.Peek(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	w.Header().Set("Content-Type", rc.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(rc.Size, 10))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	if _, err := io.Copy(w, image); err != nil {
		// Part of the image is sent, so an error body would only be added
		// to it; the response falls short of its length instead.
		a.log.Warn("routes.receipt.stream.failed", "requestId", middleware.RequestID(r.Context()), "receiptId", rc.ID, "error", err)
	}
	return nil
}

func (a *API) scanReceipt(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "receiptId", "Receipt")
	if err != nil {
		return err
	}
	in, err := decodeOptional[platform.ScanRequest](w, r)
	if err != nil {
		return err
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
	in, err := decode[platform.DraftRequest](w, r)
	if err != nil {
		return err
	}
	req := assist.Request{Text: in.Text}
	if req.GroupID, err = optionalID(in.GroupID, "groupId"); err != nil {
		return err
	}
	if req.ReceiptID, err = optionalID(in.ReceiptID, "receiptId"); err != nil {
		return err
	}
	d, err := a.svc.Assist.Draft(r.Context(), me(r), req)
	if err != nil {
		return err
	}
	out := platform.ExpenseDraft{GroupID: idPtr(d.GroupID), ReceiptID: idPtr(d.ReceiptID), Description: d.Description,
		Currency: d.Currency, Total: d.Total, Date: d.Date, Category: d.Category, SplitMethod: string(d.Method),
		Payers: []platform.Payer{}, Splits: []platform.Split{}, Items: []platform.Item{}, Tax: d.Tax, Tip: d.Tip, Warnings: d.Warnings}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	for _, p := range d.Payers {
		out.Payers = append(out.Payers, platform.Payer{UserID: p.UserID.String(), Amount: p.Amount})
	}
	for _, s := range d.Splits {
		out.Splits = append(out.Splits, platform.Split{UserID: s.UserID.String(), Value: number(s.Value)})
	}
	for _, it := range d.Items {
		ids := make([]string, len(it.UserIDs))
		for i, id := range it.UserIDs {
			ids[i] = id.String()
		}
		out.Items = append(out.Items, platform.Item{Name: it.Name, Amount: it.Amount, UserIDs: ids})
	}
	return details(w, http.StatusOK, out)
}
