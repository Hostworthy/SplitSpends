package routes

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/Hostworthy/SplitSpends/packages/webpush"
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
