package routes

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
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

// TestPushLimits checks that registering devices and testing push are
// counted per signed-in person: the 21st registration and the 6th test in a
// minute are refused, while someone else can still do both.
func TestPushLimits(t *testing.T) {
	cfg := bootstraptest.Defaults()
	cfg.Serverless, cfg.VAPIDPublicKey = true, "test-key"
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
