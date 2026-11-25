package notifications

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
)

// failing is a mail server that refuses every message.
type failing struct{}

func (failing) Send(context.Context, string, string, string, string) error {
	return errors.New("smtp: 554 refused")
}

// held is a mail server that waits for release before accepting a message.
type held struct {
	release chan struct{}
	sent    chan string
}

func (h held) Send(_ context.Context, to, _, _, _ string) error {
	<-h.release
	h.sent <- to
	return nil
}

func TestLoginCodeFailureIsLoggedNotReturned(t *testing.T) {
	var out bytes.Buffer
	n := &Notifier{sender: failing{}, cfg: Config{AppName: "SplitSpends", Sync: true}, log: slog.New(slog.NewTextHandler(&out, nil))}
	if err := n.LoginCode(context.Background(), "ada@example.com", "123456", 10*time.Minute); err != nil {
		t.Fatalf("a mail failure must not reach the caller: %v", err)
	}
	logged := out.String()
	if !strings.Contains(logged, "notifications.send.failed") {
		t.Fatalf("the failure must be logged: %s", logged)
	}
	if strings.Contains(logged, "123456") {
		t.Fatalf("the code must not be logged: %s", logged)
	}
}

func TestLoginCodeDoesNotWaitForTheMailServer(t *testing.T) {
	h := held{release: make(chan struct{}), sent: make(chan string, 1)}
	n := &Notifier{sender: h, cfg: Config{AppName: "SplitSpends"}, log: slog.New(slog.DiscardHandler), queue: make(chan ports.Notification, 1)}
	go n.work()
	defer n.Close()
	if err := n.LoginCode(context.Background(), "ada@example.com", "123456", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	close(h.release)
	if to := <-h.sent; to != "ada@example.com" {
		t.Fatalf("sent to %q", to)
	}
}
