package routes

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/Hostworthy/SplitSpends/packages/webpush"
	"github.com/google/uuid"
)

// TestSignOutRemovesDevice checks that signing out on a device removes it
// from the account, so it stops getting notifications, while devices of
// the account's other sessions stay.
func TestSignOutRemovesDevice(t *testing.T) {
	cfg := bootstraptest.Defaults()
	cfg.Serverless = true
	s := newServer(t, bootstraptest.Options{Config: cfg, Sender: accepting{}})
	laptop := s.signIn(t, "ada@example.com", "Ada").Token
	phone := s.signIn(t, "ada@example.com", "Ada").Token
	public, _, err := webpush.Keys()
	if err != nil {
		t.Fatal(err)
	}
	device := func(endpoint string) platform.PushSubscriptionCreate {
		return platform.PushSubscriptionCreate{Endpoint: endpoint,
			Keys: platform.PushKeys{P256dh: public, Auth: base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))}}
	}
	devices := platform.Prefix + "/me/push-subscriptions"
	s.call(t, http.MethodPost, devices, laptop, device("https://push.example.com/laptop")).want(t, http.StatusCreated)
	s.call(t, http.MethodPost, devices, phone, device("https://push.example.com/phone")).want(t, http.StatusCreated)

	s.call(t, http.MethodPost, platform.Prefix+"/auth/logout", laptop, nil).want(t, http.StatusNoContent)

	left := items[platform.PushSubscription](t, s.call(t, http.MethodGet, devices, phone, nil).want(t, http.StatusOK))
	if left.Total != 1 || left.Items[0].Endpoint != "https://push.example.com/phone" {
		t.Fatalf("only the phone should be left, got %+v", left.Items)
	}
}

// unrecorded is a store of push devices that cannot record deliveries.
type unrecorded struct{ push.Repository }

func (unrecorded) Succeeded(context.Context, uuid.UUID, time.Time) error {
	return errors.New("database.push.succeeded: connection refused")
}

// TestPushTestLogsStoreFailure checks that a test notification that was
// delivered but could not be recorded still answers, and the failure is
// logged as it is for other notifications.
func TestPushTestLogsStoreFailure(t *testing.T) {
	cfg := bootstraptest.Defaults()
	cfg.Serverless = true
	s := newServer(t, bootstraptest.Options{Config: cfg, Sender: accepting{},
		Subscriptions: func(r push.Repository) push.Repository { return unrecorded{r} }})
	ada := s.signIn(t, "ada@example.com", "Ada").Token
	public, _, err := webpush.Keys()
	if err != nil {
		t.Fatal(err)
	}
	s.call(t, http.MethodPost, platform.Prefix+"/me/push-subscriptions", ada, platform.PushSubscriptionCreate{
		Endpoint: "https://push.example.com/laptop",
		Keys:     platform.PushKeys{P256dh: public, Auth: base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))},
	}).want(t, http.StatusCreated)

	got := data[platform.PushTest](t, s.call(t, http.MethodPost, platform.Prefix+"/me/push-subscriptions/test", ada, nil).want(t, http.StatusOK))
	if got.Sent != 1 {
		t.Fatalf("want the notification sent, got %+v", got)
	}
	if !s.logs.has("push.test.store-failed") {
		t.Fatal("the failure to record the delivery was not logged")
	}
}
