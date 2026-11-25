package bootstrap_test

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/assist"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/receipt"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/storage/storagetest"
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

// TestReceiptAccess checks who may open a receipt on an expense: its
// uploader, the group's current members and the people sharing the expense,
// also after it was deleted (it can be restored), but not a member who left
// without a share, a friend outside the group or someone who knows nobody.
func TestReceiptAccess(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	v, err := svc.Groups.AddMember(ctx, ada, g.ID, group.Invitee{Email: "cy@example.com", Name: "Cy"})
	if err != nil {
		t.Fatal(err)
	}
	var cy uuid.UUID
	for _, m := range v.Members {
		if m != ada && m != ben {
			cy = m
		}
	}
	zoe := env.SignIn(t, "zoe@example.com", "Zoe").User.ID
	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID
	if _, err := svc.Friends.Add(ctx, ada, "zoe@example.com", ""); err != nil {
		t.Fatal(err)
	}
	book := func(groupID *uuid.UUID, other uuid.UUID) (*receipt.Receipt, *expense.Expense) {
		t.Helper()
		r, err := svc.Receipts.Upload(ctx, ada, png)
		if err != nil {
			t.Fatal(err)
		}
		e, err := svc.Expenses.Create(ctx, ada, expense.Input{GroupID: groupID, Description: "Bread", Currency: "EUR",
			Total: 300, Date: "2026-03-15", Method: expense.MethodEqual, ReceiptID: &r.ID,
			Payers: []expense.Payer{{UserID: ada, Amount: 300}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: other}}})
		if err != nil {
			t.Fatal(err)
		}
		return r, e
	}
	check := func(step string, r *receipt.Receipt, who map[string]uuid.UUID, visible bool) {
		t.Helper()
		for name, id := range who {
			_, body, err := svc.Receipts.Open(ctx, id, r.ID)
			if err == nil {
				_ = body.Close()
			}
			if visible && err != nil {
				t.Errorf("%s: %s must see the receipt, got %v", step, name, err)
			}
			if !visible && !errs.Is(err, errs.KindNotFound) {
				t.Errorf("%s: %s must not see the receipt, got %v", step, name, err)
			}
		}
	}

	shared, bread := book(&g.ID, ben)
	dinner, _ := book(nil, zoe)
	check("group expense", shared, map[string]uuid.UUID{"Ada": ada, "Ben": ben, "Cy": cy}, true)
	check("group expense", shared, map[string]uuid.UUID{"Zoe": zoe, "Mal": mal}, false)
	check("expense outside groups", dinner, map[string]uuid.UUID{"Ada": ada, "Zoe": zoe}, true)
	check("expense outside groups", dinner, map[string]uuid.UUID{"Ben": ben, "Cy": cy, "Mal": mal}, false)

	if err := svc.Groups.RemoveMember(ctx, cy, g.ID, cy); err != nil {
		t.Fatal(err)
	}
	check("after Cy left", shared, map[string]uuid.UUID{"Cy": cy}, false)

	if err := svc.Expenses.Delete(ctx, ada, bread.ID); err != nil {
		t.Fatal(err)
	}
	check("deleted expense", shared, map[string]uuid.UUID{"Ada": ada, "Ben": ben}, true)
	check("deleted expense", shared, map[string]uuid.UUID{"Cy": cy, "Mal": mal}, false)
}

// uploading is the receipt repository with a hook that runs right before a
// receipt row is written.
type uploading struct {
	*repositories.Receipts
	before func()
}

func (u uploading) Create(ctx context.Context, r *receipt.Receipt) error {
	u.before()
	return u.Receipts.Create(ctx, r)
}

func TestUploadQuotaIsAtomic(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	for range receipt.MaxPending - 1 {
		if _, err := env.Services.Receipts.Upload(ctx, ada, png); err != nil {
			t.Fatal(err)
		}
	}
	// One place is left. Right before it is taken, another upload arrives.
	other := make(chan error, 1)
	var once sync.Once
	repo := uploading{repositories.NewReceipts(env.DB), func() {
		once.Do(func() {
			meanwhile(t, env, func() {
				_, err := env.Services.Receipts.Upload(ctx, ada, png)
				other <- err
			})
		})
	}}
	svc := receipt.NewService(repo, repositories.NewBlobs(env.DB), nil, database.NewTx(env.DB), env.Clock)
	_, err := svc.Upload(ctx, ada, png)
	late := <-other
	if (err == nil) == (late == nil) {
		t.Fatalf("exactly one upload may take the last place, got %v and %v", err, late)
	}
	if n, _, err := repositories.NewReceipts(env.DB).Pending(ctx, ada); err != nil || n != receipt.MaxPending {
		t.Fatalf("want %d receipts waiting for an expense, got %d, %v", receipt.MaxPending, n, err)
	}
}

// attaching is the expense repository with a hook that runs right after an
// expense is written, before its transaction commits.
type attaching struct {
	*repositories.Expenses
	after func()
}

func (a attaching) Create(ctx context.Context, e *expense.Expense) error {
	if err := a.Expenses.Create(ctx, e); err != nil {
		return err
	}
	a.after()
	return nil
}

// stale uploads a receipt for Ada that has waited for an expense longer than
// cleanup allows.
func stale(t *testing.T, env *bootstraptest.Env, ada uuid.UUID) *receipt.Receipt {
	t.Helper()
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	r, err := env.Services.Receipts.Upload(context.Background(), ada, png)
	if err != nil {
		t.Fatal(err)
	}
	env.Clock.Add(receipt.PendingTTL + time.Minute)
	return r
}

// bread is an expense Ada paid and shares equally with Ben, with a receipt.
func bread(groupID, receiptID, ada, ben uuid.UUID) expense.Input {
	return expense.Input{GroupID: &groupID, Description: "Bread", Currency: "EUR", Total: 300, Date: "2026-03-15",
		Method: expense.MethodEqual, ReceiptID: &receiptID,
		Payers: []expense.Payer{{UserID: ada, Amount: 300}}, Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}}
}

// TestCleanupKeepsReceiptBeingAttached checks that a cleanup run that starts
// while an expense is saved with an old unused receipt leaves the receipt on
// the expense.
func TestCleanupKeepsReceiptBeingAttached(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	r := stale(t, env, ada)
	// Right after the expense is written, the cleanup job runs.
	removed := make(chan int, 1)
	svc := wired(env, attaching{repositories.NewExpenses(env.DB), func() {
		meanwhile(t, env, func() {
			n, err := env.Services.Receipts.Cleanup(ctx)
			if err != nil {
				t.Error(err)
			}
			removed <- n
		})
	}})
	e, err := svc.Create(ctx, ada, bread(g.ID, r.ID, ada, ben))
	if err != nil {
		t.Fatal(err)
	}
	if n := <-removed; n != 0 {
		t.Fatalf("the receipt being attached must not be removed, %d removed", n)
	}
	saved, err := env.Services.Expenses.Get(ctx, ada, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ReceiptID == nil || *saved.ReceiptID != r.ID {
		t.Fatalf("the expense must keep its receipt, got %v", saved.ReceiptID)
	}
	_, body, err := env.Services.Receipts.Open(ctx, ada, r.ID)
	if err != nil {
		t.Fatalf("the receipt must stay: %v", err)
	}
	_ = body.Close()
}

// removing is a blob store with a hook that runs right before an image is
// removed.
type removing struct {
	ports.BlobStore
	before func()
}

func (r removing) Delete(ctx context.Context, key string) error {
	r.before()
	return r.BlobStore.Delete(ctx, key)
}

// TestAttachRefusesReceiptBeingRemoved checks that an expense saved with an
// old unused receipt while cleanup removes it is refused, rather than saved
// without the receipt.
func TestAttachRefusesReceiptBeingRemoved(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	r := stale(t, env, ada)
	// Once the cleanup has removed the row and before it commits, Ada saves
	// an expense with the receipt.
	created := make(chan error, 1)
	blobs := removing{env.Blobs, func() {
		meanwhile(t, env, func() {
			_, err := env.Services.Expenses.Create(ctx, ada, bread(g.ID, r.ID, ada, ben))
			created <- err
		})
	}}
	svc := receipt.NewService(repositories.NewReceipts(env.DB), blobs, nil, database.NewTx(env.DB), env.Clock)
	if removed, err := svc.Cleanup(ctx); err != nil || removed != 1 {
		t.Fatalf("the unused receipt must be removed: %d removed, %v", removed, err)
	}
	err := <-created
	var invalid *errs.Error
	if !errors.As(err, &invalid) || invalid.Kind != errs.KindValidation || invalid.Field != "receiptId" {
		t.Fatalf("an expense with a receipt being removed must be refused, got %v", err)
	}
	if n := len(groupExpenses(t, env, ada, g.ID)); n != 0 {
		t.Fatalf("the refused expense must not be saved, %d saved", n)
	}
}

// TestBlobs checks the PostgreSQL blob store keeps the blob store contract.
func TestBlobs(t *testing.T) {
	env := bootstraptest.Open(t, bootstraptest.Options{})
	storagetest.Run(t, repositories.NewBlobs(env.DB))
}

// drafts is an AI reader that counts the drafts it is asked for.
type drafts struct {
	ports.Assistant
	calls int
}

func (d *drafts) DraftExpense(context.Context, ports.DraftInput) (ports.DraftExtract, error) {
	d.calls++
	return ports.DraftExtract{Description: "Bread", Amount: "3"}, nil
}

// TestDraftNeedsMembership checks that a draft in a group the user is not
// in, or with a receipt they cannot see, fails before anything about the
// group or the receipt reaches the AI provider.
func TestDraftNeedsMembership(t *testing.T) {
	ctx := context.Background()
	reader := &drafts{}
	env := bootstraptest.Open(t, bootstraptest.Options{Reader: reader})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	mal := env.SignIn(t, "mal@example.com", "Mal").User.ID
	name := "Flat"
	g, err := env.Services.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := env.Services.Receipts.Upload(ctx, ada, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...))
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range []assist.Request{
		{Text: "bread 3", GroupID: &g.ID},
		{Text: "bread 3", ReceiptID: &r.ID},
	} {
		if _, err := env.Services.Assist.Draft(ctx, mal, req); !errs.Is(err, errs.KindNotFound) {
			t.Errorf("%+v: want not found, got %v", req, err)
		}
	}
	if reader.calls != 0 {
		t.Fatalf("nothing may reach the AI provider: %d drafts", reader.calls)
	}
	if _, err := env.Services.Assist.Draft(ctx, ada, assist.Request{Text: "bread 3", GroupID: &g.ID, ReceiptID: &r.ID}); err != nil || reader.calls != 1 {
		t.Fatalf("Ada drafts in her own group with her receipt: %v, %d drafts", err, reader.calls)
	}
}

// TestDraftKeepsOnlyOwnReceipt checks that a draft read from a receipt
// someone else uploaded leaves the receipt off, since only the uploader may
// attach it, so the draft can be saved as it is.
func TestDraftKeepsOnlyOwnReceipt(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{Reader: &drafts{}})
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	ben := env.SignIn(t, "ben@example.com", "Ben").User.ID
	name := "Flat"
	g, err := svc.Groups.Create(ctx, ada, group.CreateInput{Patch: group.Patch{Name: &name}, People: []uuid.UUID{ben}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.Receipts.Upload(ctx, ada, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Expenses.Create(ctx, ada, expense.Input{GroupID: &g.ID, Description: "Bread", Currency: "EUR", Total: 300,
		Method: expense.MethodEqual, ReceiptID: &r.ID, Payers: []expense.Payer{{UserID: ada, Amount: 300}},
		Splits: []expense.SplitInput{{UserID: ada}, {UserID: ben}}}); err != nil {
		t.Fatal(err)
	}

	mine, err := svc.Assist.Draft(ctx, ada, assist.Request{GroupID: &g.ID, ReceiptID: &r.ID})
	if err != nil || mine.ReceiptID == nil || *mine.ReceiptID != r.ID {
		t.Fatalf("Ada's draft should keep her receipt: %+v, %v", mine, err)
	}
	d, err := svc.Assist.Draft(ctx, ben, assist.Request{GroupID: &g.ID, ReceiptID: &r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if d.ReceiptID != nil {
		t.Fatalf("Ben's draft must not carry Ada's receipt, got %s", d.ReceiptID)
	}
	if _, err := svc.Expenses.Create(ctx, ben, expense.Input{GroupID: d.GroupID, ReceiptID: d.ReceiptID, Description: d.Description,
		Currency: d.Currency, Total: d.Total, Date: d.Date, Category: d.Category, Method: d.Method, Payers: d.Payers,
		Splits: d.Splits}); err != nil {
		t.Fatalf("Ben's draft should save as it is: %v", err)
	}
}

// TestDraftNeedsReadableImage checks that a draft from a HEIC receipt, which
// is stored but which AI providers cannot read, is refused before the
// provider is asked.
func TestDraftNeedsReadableImage(t *testing.T) {
	ctx := context.Background()
	reader := &drafts{}
	env := bootstraptest.Open(t, bootstraptest.Options{Reader: reader})
	ada := env.SignIn(t, "ada@example.com", "Ada").User.ID
	heic := append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), make([]byte, 64)...)
	r, err := env.Services.Receipts.Upload(ctx, ada, heic)
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.Services.Assist.Draft(ctx, ada, assist.Request{ReceiptID: &r.ID})
	if e, ok := errs.As(err); !ok || e.Kind != errs.KindValidation || e.Field != "file" {
		t.Fatalf("want a validation error on file, got %v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("a HEIC receipt must not reach the AI provider: %d drafts", reader.calls)
	}
}
