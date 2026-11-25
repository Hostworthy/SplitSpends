package bootstrap_test

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/receipt"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
)

// TestReceiptCleanup checks that the jobs remove a receipt no expense has
// used for a day, row and image, and keep one on a deleted expense (which
// can be restored) and one uploaded too recently.
func TestReceiptCleanup(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	upload := func() *receipt.Receipt {
		t.Helper()
		r, err := env.Services.Receipts.Upload(ctx, ada, png)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	unused, deleted := upload(), upload()
	e, err := env.Services.Expenses.Create(ctx, ada, expense.Input{GroupID: &g.ID, Description: "Bread", Currency: "EUR",
		Total: 300, Date: "2026-03-15", Method: expense.MethodEqual, ReceiptID: &deleted.ID,
		Payers: []expense.Payer{{UserID: ada, Amount: 300}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Services.Expenses.Delete(ctx, ada, e.ID); err != nil {
		t.Fatal(err)
	}
	count, size, err := repositories.NewReceipts(env.DB).Pending(ctx, ada)
	if err != nil || count != 1 || size != unused.Size {
		t.Fatalf("only the unused receipt counts as pending: %d receipts, %d bytes, %v", count, size, err)
	}
	env.Clock.Add(receipt.PendingTTL + time.Minute)
	recent := upload()

	rt := &bootstrap.Runtime{Config: env.Config, Log: slog.New(slog.DiscardHandler), Services: env.Services}
	if err := rt.RunJobsOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var missing *errs.Error
	if _, _, err := env.Services.Receipts.Open(ctx, ada, unused.ID); !errors.As(err, &missing) || missing.Kind != errs.KindNotFound {
		t.Fatalf("the unused receipt must be gone, got %v", err)
	}
	// The tests store images on disk.
	if _, err := env.Blobs.Open(ctx, unused.StorageKey); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the unused receipt's image must be gone, got %v", err)
	}
	for _, kept := range []*receipt.Receipt{deleted, recent} {
		_, body, err := env.Services.Receipts.Open(ctx, ada, kept.ID)
		if err != nil {
			t.Fatalf("receipt %s must stay: %v", kept.ID, err)
		}
		_ = body.Close()
	}
}
