package routes

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/Hostworthy/SplitSpends/packages/webpush"
	"github.com/google/uuid"
)

// TestImportLimit checks that imports are counted per signed-in person: the
// eleventh in an hour is refused, while someone else can still import.
func TestImportLimit(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada").Token
	ben := s.signIn(t, "ben@example.com", "Ben").Token
	path := platform.Prefix + "/import"
	bundle := platform.ImportRequest{MeID: uuid.NewString()}

	for range 10 {
		s.call(t, http.MethodPost, path, ada, bundle).want(t, http.StatusOK)
	}
	if e := s.call(t, http.MethodPost, path, ada, bundle).want(t, http.StatusTooManyRequests).failure(t); e.Code != "RATE_LIMITED" {
		t.Fatalf("unexpected error %+v", e)
	}
	s.call(t, http.MethodPost, path, ben, bundle).want(t, http.StatusOK)
}

// TestSocialLimit checks that adding people and reminding them share one
// budget per signed-in person: the 21st such request in a minute is refused,
// while someone else can still add people.
func TestSocialLimit(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada").Token
	ben := s.signIn(t, "ben@example.com", "Ben").Token
	friends := platform.Prefix + "/friends"
	for i := range 19 {
		s.call(t, http.MethodPost, friends, ada, platform.Invitee{Name: fmt.Sprintf("Person %d", i)}).want(t, http.StatusCreated)
	}
	name := "Trip"
	s.call(t, http.MethodPost, platform.Prefix+"/groups", ada, platform.GroupCreate{GroupPatch: platform.GroupPatch{Name: &name}}).want(t, http.StatusCreated)
	if e := s.call(t, http.MethodPost, friends, ada, platform.Invitee{Name: "One more"}).want(t, http.StatusTooManyRequests).failure(t); e.Code != "RATE_LIMITED" {
		t.Fatalf("unexpected error %+v", e)
	}
	s.call(t, http.MethodPost, friends, ben, platform.Invitee{Name: "Sam"}).want(t, http.StatusCreated)
}

// accepting is a push service that takes every message.
type accepting struct{}

func (accepting) Send(context.Context, *push.Subscription, []byte) error { return nil }
func (accepting) PublicKey() string                                      { return "test-key" }

// TestPushLimits checks that registering devices and testing push are
// counted per signed-in person: the 21st registration and the 6th test in a
// minute are refused, while someone else can still do both.
func TestPushLimits(t *testing.T) {
	cfg := bootstraptest.Defaults()
	cfg.Serverless = true
	s := newServer(t, bootstraptest.Options{Config: cfg, Sender: accepting{}})
	ada := s.signIn(t, "ada@example.com", "Ada").Token
	ben := s.signIn(t, "ben@example.com", "Ben").Token
	public, _, err := webpush.Keys()
	if err != nil {
		t.Fatal(err)
	}
	device := func(i int) platform.PushSubscriptionCreate {
		return platform.PushSubscriptionCreate{Endpoint: fmt.Sprintf("https://push.example.com/%d", i),
			Keys: platform.PushKeys{P256dh: public, Auth: base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))}}
	}
	devices, test := platform.Prefix+"/me/push-subscriptions", platform.Prefix+"/me/push-subscriptions/test"

	for i := range 20 {
		s.call(t, http.MethodPost, devices, ada, device(i)).want(t, http.StatusCreated)
	}
	if e := s.call(t, http.MethodPost, devices, ada, device(20)).want(t, http.StatusTooManyRequests).failure(t); e.Code != "RATE_LIMITED" {
		t.Fatalf("unexpected error %+v", e)
	}
	for range 5 {
		s.call(t, http.MethodPost, test, ada, nil).want(t, http.StatusOK)
	}
	if e := s.call(t, http.MethodPost, test, ada, nil).want(t, http.StatusTooManyRequests).failure(t); e.Code != "RATE_LIMITED" {
		t.Fatalf("unexpected error %+v", e)
	}

	s.call(t, http.MethodPost, devices, ben, device(100)).want(t, http.StatusCreated)
	s.call(t, http.MethodPost, test, ben, nil).want(t, http.StatusOK)
}

// counting is an AI reader that answers every request with an empty reading
// and counts the requests it gets.
type counting struct{ calls atomic.Int32 }

func (c *counting) ReadReceipt(context.Context, ports.ReceiptInput) (ports.ReceiptExtract, error) {
	c.calls.Add(1)
	return ports.ReceiptExtract{Currency: "USD", Total: "12.50"}, nil
}

func (c *counting) DraftExpense(context.Context, ports.DraftInput) (ports.DraftExtract, error) {
	c.calls.Add(1)
	return ports.DraftExtract{Description: "Lunch", Amount: "12.50", Currency: "USD"}, nil
}

// TestAILimit checks that receipt scans and expense drafts share one hourly
// budget per signed-in person: the request after the budget is refused
// without reaching the AI provider, while someone else can still use AI.
func TestAILimit(t *testing.T) {
	cfg := bootstraptest.Defaults()
	cfg.AIRateLimit = 3
	reader := &counting{}
	s := newServer(t, bootstraptest.Options{Config: cfg, Reader: reader})
	ada := s.signIn(t, "ada@example.com", "Ada").Token
	ben := s.signIn(t, "ben@example.com", "Ben").Token
	rc := data[platform.Receipt](t, s.upload(t, platform.Prefix+"/receipts", ada, pixel(t)).want(t, http.StatusCreated))
	scan := platform.Prefix + "/receipts/" + rc.ID + "/scan"
	draft := platform.Prefix + "/ai/expense-draft"

	s.call(t, http.MethodPost, scan, ada, nil).want(t, http.StatusOK)
	s.call(t, http.MethodPost, scan, ada, nil).want(t, http.StatusOK)
	s.call(t, http.MethodPost, draft, ada, platform.DraftRequest{Text: "Lunch 12.50"}).want(t, http.StatusOK)
	if e := s.call(t, http.MethodPost, scan, ada, nil).want(t, http.StatusTooManyRequests).failure(t); e.Code != "RATE_LIMITED" {
		t.Fatalf("unexpected error %+v", e)
	}
	if e := s.call(t, http.MethodPost, draft, ada, platform.DraftRequest{Text: "Lunch 12.50"}).want(t, http.StatusTooManyRequests).failure(t); e.Code != "RATE_LIMITED" {
		t.Fatalf("unexpected error %+v", e)
	}
	if n := reader.calls.Load(); n != 3 {
		t.Fatalf("want 3 requests to the AI provider, got %d", n)
	}
	s.call(t, http.MethodPost, draft, ben, platform.DraftRequest{Text: "Lunch 12.50"}).want(t, http.StatusOK)
}
