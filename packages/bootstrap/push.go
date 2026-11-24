package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/Hostworthy/SplitSpends/packages/webpush"
	"github.com/google/uuid"
)

func (c *Config) loadPush() error {
	c.VAPIDPublicKey, c.VAPIDPrivateKey = env("VAPID_PUBLIC_KEY", ""), env("VAPID_PRIVATE_KEY", "")
	if (c.VAPIDPublicKey == "") != (c.VAPIDPrivateKey == "") {
		return fmt.Errorf("bootstrap.config.env.invalid.VAPID_PRIVATE_KEY: set both VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY, or neither")
	}
	if c.VAPIDPublicKey != "" {
		if err := webpush.Check(c.VAPIDPublicKey, c.VAPIDPrivateKey); err != nil {
			return fmt.Errorf("bootstrap.config.env.invalid.VAPID_PUBLIC_KEY: %w", err)
		}
	}
	c.VAPIDSubject = env("VAPID_SUBJECT", "")
	if c.VAPIDSubject == "" {
		c.VAPIDSubject = defaultSubject(c.BaseURL, c.SMTPFrom)
	}
	if !strings.HasPrefix(c.VAPIDSubject, "mailto:") && !strings.HasPrefix(c.VAPIDSubject, "https://") {
		return fmt.Errorf("bootstrap.config.env.invalid.VAPID_SUBJECT: use a mailto: address or an https:// URL")
	}
	c.PushAllowPrivate, _ = strconv.ParseBool(env("PUSH_ALLOW_PRIVATE", "false"))
	return nil
}

// defaultSubject is the contact push services see: the public https URL,
// else the email sender, else an address at the server's host.
func defaultSubject(baseURL, from string) string {
	if strings.HasPrefix(baseURL, "https://") {
		return baseURL
	}
	if addr, err := mail.ParseAddress(from); err == nil {
		return "mailto:" + addr.Address
	}
	host := "localhost"
	if u, err := url.Parse(baseURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	return "mailto:admin@" + host
}

// pushSender builds the Web Push sender. Keys come from the environment or,
// when unset, from the database, where the first instance to start stores
// a generated pair. Failing to set up push disables it instead of stopping
// the server: email keeps working.
func pushSender(ctx context.Context, db *database.DB, cfg *Config, log *slog.Logger) push.Sender {
	public, private := cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey
	if public == "" {
		candidatePublic, candidatePrivate, err := webpush.Keys()
		if err == nil {
			public, private, err = repositories.NewVAPIDKeys(db).Ensure(ctx, candidatePublic, candidatePrivate)
		}
		if err != nil {
			log.Error("bootstrap.push.keys.failed", "error", err)
			return nil
		}
	}
	sender, err := webpush.New(webpush.Config{PublicKey: public, PrivateKey: private, Subject: cfg.VAPIDSubject, AllowPrivate: cfg.PushAllowPrivate})
	if err != nil {
		log.Error("bootstrap.push.disabled", "error", err)
		return nil
	}
	cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey = public, private
	log.Info("bootstrap.push.enabled", "generated", env("VAPID_PUBLIC_KEY", "") == "")
	return sender
}

// pusher is the part of push.Service the fan-out needs.
type pusher interface {
	Notify(ctx context.Context, n ports.Notification) (push.Report, error)
}

// fanout is the ports.Notifier services use: it sends each notification by
// email and push, so services call Notify once. Push deliveries go through
// a bounded queue (like emails) so slow push services never hold up a
// request; with sync set (serverless) they finish before Notify returns.
type fanout struct {
	email ports.Notifier
	push  pusher
	log   *slog.Logger
	sync  bool
	queue chan ports.Notification
	done  sync.WaitGroup
}

func newFanout(email ports.Notifier, p pusher, log *slog.Logger, sync bool) *fanout {
	f := &fanout{email: email, push: p, log: log, sync: sync, queue: make(chan ports.Notification, 256)}
	if p != nil && !sync {
		f.done.Add(1)
		go f.work()
	}
	return f
}

func (f *fanout) LoginCode(ctx context.Context, to, code string, ttl time.Duration) error {
	return f.email.LoginCode(ctx, to, code, ttl)
}

func (f *fanout) Invite(ctx context.Context, to, inviter string) error {
	return f.email.Invite(ctx, to, inviter)
}

func (f *fanout) Notify(ctx context.Context, n ports.Notification) {
	if n.To != "" {
		f.email.Notify(ctx, n)
	}
	if f.push == nil || n.UserID == uuid.Nil {
		return
	}
	if f.sync {
		f.deliver(n)
		return
	}
	select {
	case f.queue <- n:
	default:
		f.log.Warn("push.queue.full", "kind", n.Kind)
	}
}

func (f *fanout) work() {
	defer f.done.Done()
	for n := range f.queue {
		f.deliver(n)
	}
}

func (f *fanout) deliver(n ports.Notification) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := f.push.Notify(ctx, n)
	if err != nil {
		f.log.Error("push.deliver.failed", "kind", n.Kind, "error", err)
		return
	}
	if r.Failed > 0 || r.Removed > 0 {
		f.log.Warn("push.deliver.partial", "kind", n.Kind, "sent", r.Sent, "failed", r.Failed, "removed", r.Removed, "error", r.Err)
	}
}

// Close stops the push worker after draining queued messages.
func (f *fanout) Close() {
	if f.push != nil && !f.sync {
		close(f.queue)
		f.done.Wait()
	}
}
