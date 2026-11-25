package bootstrap_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
)

// concurrently runs fn n times at once and returns the errors in order.
func concurrently(n int, fn func(i int) error) []error {
	out := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			out[i] = fn(i)
		}()
	}
	close(start)
	wg.Wait()
	return out
}

func TestVerifyAttemptsUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	if err := env.Services.Auth.RequestCode(ctx, "ada@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	code := env.Notifier.Code("ada@example.com")
	wrong := func(i int) string {
		guess := fmt.Sprintf("%06d", i)
		if guess == code {
			guess = "999999x"
		}
		return guess
	}
	for i, err := range concurrently(50, func(i int) error {
		_, err := env.Services.Auth.Verify(ctx, "ada@example.com", wrong(i), "", "")
		return err
	}) {
		if !errs.Is(err, errs.KindUnauthorized) {
			t.Fatalf("guess %d: %v", i, err)
		}
	}
	if _, err := env.Services.Auth.Verify(ctx, "ada@example.com", code, "", ""); !errs.Is(err, errs.KindUnauthorized) {
		t.Fatalf("the right code must fail after too many attempts, got %v", err)
	}
	var attempts int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT attempts FROM login_challenges WHERE email = 'ada@example.com'`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 5 {
		t.Fatalf("attempts = %d, want 5", attempts)
	}
}

func TestCodeRedeemedOnce(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	if err := env.Services.Auth.RequestCode(ctx, "ada@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	code := env.Notifier.Code("ada@example.com")
	ok := 0
	for _, err := range concurrently(8, func(int) error {
		_, err := env.Services.Auth.Verify(ctx, "ada@example.com", code, "Ada", "test")
		return err
	}) {
		switch {
		case err == nil:
			ok++
		case !errs.Is(err, errs.KindUnauthorized):
			t.Fatalf("verify: %v", err)
		}
	}
	var sessions int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if ok != 1 || sessions != 1 {
		t.Fatalf("one code opened %d logins and %d sessions, want 1", ok, sessions)
	}
}

func TestCodeRequestsUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	limited := 0
	for _, err := range concurrently(20, func(int) error {
		return env.Services.Auth.RequestCode(ctx, "ada@example.com", "", "")
	}) {
		switch {
		case errs.Is(err, errs.KindRateLimited):
			limited++
		case err != nil:
			t.Fatalf("request: %v", err)
		}
	}
	var codes int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM login_challenges`).Scan(&codes); err != nil {
		t.Fatal(err)
	}
	if codes != 5 || limited != 15 {
		t.Fatalf("%d codes and %d refusals, want 5 and 15", codes, limited)
	}
}

// TestFirstAdminIsUnique verifies two first sign-ins at once on an empty
// closed server: one becomes the administrator, the other is refused.
func TestFirstAdminIsUnique(t *testing.T) {
	ctx := context.Background()
	cfg := bootstraptest.Defaults()
	cfg.SignupMode = auth.SignupClosed
	env := bootstraptest.Open(t, bootstraptest.Options{Config: cfg})
	for run := range 20 {
		if _, err := env.DB.Pool.Exec(ctx, `TRUNCATE users, login_challenges CASCADE`); err != nil {
			t.Fatal(err)
		}
		emails := []string{fmt.Sprintf("ada%d@example.com", run), fmt.Sprintf("ben%d@example.com", run)}
		for _, email := range emails {
			if err := env.Services.Auth.RequestCode(ctx, email, "", ""); err != nil {
				t.Fatal(err)
			}
			if env.Notifier.Code(email) == "" {
				t.Fatalf("run %d: no code for %s on an empty server", run, email)
			}
		}
		results := concurrently(2, func(i int) error {
			_, err := env.Services.Auth.Verify(ctx, emails[i], env.Notifier.Code(emails[i]), "", "test")
			return err
		})
		refused := 0
		for _, err := range results {
			switch {
			case errs.Is(err, errs.KindForbidden):
				refused++
			case err != nil:
				t.Fatalf("run %d: verify: %v", run, err)
			}
		}
		var users, admins int
		if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE role = 'admin') FROM users`).Scan(&users, &admins); err != nil {
			t.Fatal(err)
		}
		if users != 1 || admins != 1 || refused != 1 {
			t.Fatalf("run %d: %d accounts, %d administrators and %d refusals, want 1 each", run, users, admins, refused)
		}
	}
}
