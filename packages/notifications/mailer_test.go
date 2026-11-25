package notifications

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
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
	n := &Notifier{sender: h, cfg: Config{AppName: "SplitSpends"}, log: slog.New(slog.DiscardHandler), drain: time.Minute}
	n.start(1)
	defer n.Close()
	if err := n.LoginCode(context.Background(), "ada@example.com", "123456", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	close(h.release)
	if to := <-h.sent; to != "ada@example.com" {
		t.Fatalf("sent to %q", to)
	}
}

// recording is a mail server that accepts every message and remembers it.
type recording struct {
	mu   sync.Mutex
	sent []string
}

func (r *recording) Send(_ context.Context, to, _, _, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, to)
	return nil
}

func TestCloseDeliversQueuedMessages(t *testing.T) {
	var out bytes.Buffer
	r := &recording{}
	n := &Notifier{sender: r, cfg: Config{AppName: "SplitSpends"}, log: slog.New(slog.NewTextHandler(&out, nil)), drain: time.Minute}
	n.start(8)
	for _, to := range []string{"ada@example.com", "ben@example.com", "cy@example.com"} {
		n.Notify(context.Background(), ports.Notification{To: to, Subject: "Rent"})
	}
	n.Close()
	r.mu.Lock()
	sent := len(r.sent)
	r.mu.Unlock()
	if sent != 3 {
		t.Fatalf("Close must deliver every queued message first, sent %d of 3", sent)
	}
	n.Notify(context.Background(), ports.Notification{To: "dee@example.com", Subject: "Late"})
	n.Close()
	if !strings.Contains(out.String(), "notifications.queue.closed") {
		t.Fatalf("a message after Close must be dropped and logged: %s", out.String())
	}
}

// stuck is a mail server that takes a message and never answers.
type stuck struct {
	taken   chan struct{}
	release chan struct{}
}

func (s stuck) Send(context.Context, string, string, string, string) error {
	s.taken <- struct{}{}
	<-s.release
	return nil
}

func TestCloseGivesUpOnAStuckMailServer(t *testing.T) {
	var out bytes.Buffer
	s := stuck{taken: make(chan struct{}), release: make(chan struct{})}
	defer close(s.release)
	n := &Notifier{sender: s, cfg: Config{AppName: "SplitSpends"}, log: slog.New(slog.NewTextHandler(&out, nil)), drain: time.Millisecond}
	n.start(2)
	n.Notify(context.Background(), ports.Notification{To: "ada@example.com", Subject: "Rent"})
	<-s.taken
	n.Close()
	if !strings.Contains(out.String(), "notifications.queue.drain.timeout") {
		t.Fatalf("Close must stop waiting and say so: %s", out.String())
	}
}
