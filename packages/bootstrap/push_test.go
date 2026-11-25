package bootstrap

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/google/uuid"
)

// fakeNotifier records what the fanout hands to email. Tests in this package
// cannot use bootstraptest.Notifier: that package imports this one.
type fakeNotifier struct {
	codes map[string]string
	sent  []ports.Notification
}

func (f *fakeNotifier) LoginCode(_ context.Context, to, code string, _ time.Duration) error {
	f.codes[to] = code
	return nil
}
func (f *fakeNotifier) Invite(context.Context, string, string) error { return nil }
func (f *fakeNotifier) Notify(_ context.Context, n ports.Notification) {
	f.sent = append(f.sent, n)
}

type recordingPusher struct {
	mu   sync.Mutex
	sent []ports.Notification
	wait chan struct{} // when set, deliveries block until it is closed
}

func (r *recordingPusher) Notify(_ context.Context, n ports.Notification) (push.Report, error) {
	if r.wait != nil {
		<-r.wait
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, n)
	return push.Report{Sent: 1}, nil
}

func (r *recordingPusher) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

func TestFanoutChannels(t *testing.T) {
	ctx := context.Background()
	email := &fakeNotifier{codes: map[string]string{}}
	pushes := &recordingPusher{}
	f := newFanout(email, pushes, slog.New(slog.DiscardHandler), true)
	id := uuid.New()

	f.Notify(ctx, ports.Notification{To: "ada@example.com", UserID: id, Subject: "both"})
	f.Notify(ctx, ports.Notification{To: "ada@example.com", Subject: "email only"})
	f.Notify(ctx, ports.Notification{UserID: id, Subject: "push only"})

	if len(email.sent) != 2 || email.sent[0].Subject != "both" || email.sent[1].Subject != "email only" {
		t.Fatalf("email got %+v", email.sent)
	}
	if pushes.count() != 2 || pushes.sent[0].Subject != "both" || pushes.sent[1].Subject != "push only" {
		t.Fatalf("push got %+v", pushes.sent)
	}

	if err := f.LoginCode(ctx, "ada@example.com", "123456", time.Minute); err != nil || email.codes["ada@example.com"] != "123456" {
		t.Fatal("login codes go to email")
	}
}

func TestFanoutQueueDoesNotBlock(t *testing.T) {
	email := &fakeNotifier{codes: map[string]string{}}
	pushes := &recordingPusher{wait: make(chan struct{})}
	f := newFanout(email, pushes, slog.New(slog.DiscardHandler), false)

	done := make(chan struct{})
	go func() {
		// A slow push service must not hold up the caller.
		f.Notify(context.Background(), ports.Notification{To: "ada@example.com", UserID: uuid.New(), Subject: "queued"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify blocked on push delivery")
	}
	if len(email.sent) != 1 {
		t.Fatal("email should still be handed over")
	}
	close(pushes.wait)
	f.Close()
	if pushes.count() != 1 {
		t.Fatal("Close should drain the queue")
	}
}

// stallingPusher never finishes deliveries to slow and reports every
// other delivery on done.
type stallingPusher struct {
	slow    uuid.UUID
	release chan struct{}
	done    chan ports.Notification
}

func (p *stallingPusher) Notify(_ context.Context, n ports.Notification) (push.Report, error) {
	if n.UserID == p.slow {
		<-p.release
		return push.Report{}, nil
	}
	p.done <- n
	return push.Report{Sent: 1}, nil
}

func TestFanoutSlowRecipientDoesNotBlockOthers(t *testing.T) {
	pushes := &stallingPusher{slow: uuid.New(), release: make(chan struct{}), done: make(chan ports.Notification, 1)}
	f := newFanout(&fakeNotifier{codes: map[string]string{}}, pushes, slog.New(slog.DiscardHandler), false)
	defer f.Close()
	defer close(pushes.release)

	f.Notify(context.Background(), ports.Notification{UserID: pushes.slow, Subject: "stuck"})
	f.Notify(context.Background(), ports.Notification{UserID: uuid.New(), Subject: "next"})
	select {
	case n := <-pushes.done:
		if n.Subject != "next" {
			t.Fatalf("unexpected delivery %+v", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a recipient with stalled devices held up everyone else's push")
	}
}

func TestFanoutNotifyAfterClose(t *testing.T) {
	pushes := &recordingPusher{}
	f := newFanout(&fakeNotifier{codes: map[string]string{}}, pushes, slog.New(slog.DiscardHandler), false)
	f.Close()
	f.Notify(context.Background(), ports.Notification{UserID: uuid.New(), Subject: "late"})
	f.Close()
	if pushes.count() != 0 {
		t.Fatal("a notification after Close should be dropped")
	}
}

func TestFanoutCloseHasDeadline(t *testing.T) {
	pushes := &recordingPusher{wait: make(chan struct{})}
	defer close(pushes.wait)
	f := newFanout(&fakeNotifier{codes: map[string]string{}}, pushes, slog.New(slog.DiscardHandler), false)
	f.drain = 50 * time.Millisecond
	for range pushWorkers + 3 {
		f.Notify(context.Background(), ports.Notification{UserID: uuid.New(), Subject: "stuck"})
	}
	closed := make(chan struct{})
	go func() {
		f.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited for stalled deliveries past its deadline")
	}
}

func TestFanoutWithoutPush(t *testing.T) {
	email := &fakeNotifier{codes: map[string]string{}}
	f := newFanout(email, nil, slog.New(slog.DiscardHandler), false)
	f.Notify(context.Background(), ports.Notification{UserID: uuid.New(), Subject: "push only"})
	f.Close()
	if len(email.sent) != 0 {
		t.Fatal("nothing to send without an email address")
	}
}

func TestDefaultSubject(t *testing.T) {
	cases := map[[2]string]string{
		{"https://split.example.com", ""}:                        "https://split.example.com",
		{"http://localhost:8080", "SplitSpends <s@example.com>"}: "mailto:s@example.com",
		{"http://192.168.1.5:8080", ""}:                          "mailto:admin@192.168.1.5",
	}
	for in, want := range cases {
		if got := defaultSubject(in[0], in[1]); got != want {
			t.Errorf("defaultSubject(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
