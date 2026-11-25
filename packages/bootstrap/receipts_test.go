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
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/receipt"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
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

// TestReceiptAttach checks that only the uploader may put a receipt on an
// expense, while everyone who sees the expense sees the receipt and may edit
// the expense without losing it.
func TestReceiptAttach(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc, repo := env.Services, repositories.NewReceipts(env.DB)
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	r, err := svc.Receipts.Upload(ctx, ada, png)
	if err != nil {
		t.Fatal(err)
	}
	bread := func(groupID uuid.UUID, receiptID *uuid.UUID, payer, other uuid.UUID) expense.Input {
		return expense.Input{GroupID: &groupID, Description: "Bread", Currency: "EUR", Total: 300, Date: "2026-03-15",
			Method: expense.MethodEqual, ReceiptID: receiptID,
			Payers: []expense.Payer{{UserID: payer, Amount: 300}}, Splits: []expense.SplitInput{{UserID: payer}, {UserID: other}}}
	}
	e, err := svc.Expenses.Create(ctx, ada, bread(g.ID, &r.ID, ada, ben))
	if err != nil {
		t.Fatal(err)
	}

	if owned, err := repo.Owned(ctx, r.ID, ben); err != nil || owned {
		t.Fatalf("Ben did not upload the receipt: owned %v, %v", owned, err)
	}
	if visible, err := repo.Visible(ctx, r.ID, ben); err != nil || !visible {
		t.Fatalf("Ben sees the receipt on the group's expense: visible %v, %v", visible, err)
	}

	name := "Trip"
	trip, err := svc.Groups.Create(ctx, ben, group.CreateInput{Patch: group.Patch{Name: &name},
		Members: []group.Invitee{{Email: "mal@example.com", Name: "Mal"}}})
	if err != nil {
		t.Fatal(err)
	}
	mal := trip.Members[0]
	if mal == ben {
		mal = trip.Members[1]
	}
	_, err = svc.Expenses.Create(ctx, ben, bread(trip.ID, &r.ID, ben, mal))
	var invalid *errs.Error
	if !errors.As(err, &invalid) || invalid.Kind != errs.KindValidation || invalid.Field != "receiptId" {
		t.Fatalf("Ben may not share Ada's receipt with another group, got %v", err)
	}

	edit := bread(g.ID, &r.ID, ada, ben)
	edit.Description = "Bread and butter"
	updated, err := svc.Expenses.Update(ctx, ben, e.ID, edit)
	if err != nil {
		t.Fatalf("Ben edits the expense and keeps its receipt: %v", err)
	}
	if updated.ReceiptID == nil || *updated.ReceiptID != r.ID {
		t.Fatalf("the receipt must stay on the expense, got %v", updated.ReceiptID)
	}
}
