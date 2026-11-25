package push

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

const (
	// MaxDevices caps subscriptions per user; every notification fans out
	// to all of them.
	MaxDevices = 25
	// MaxFailures removes a subscription after this many failed deliveries
	// in a row (for example after the server's VAPID key changed).
	MaxFailures = 25
	// SendTimeout bounds one delivery to one device. Endpoints are chosen
	// by users, so a slow or silent one must not hold up the others.
	SendTimeout = 5 * time.Second

	// parallel is how many devices are sent to at once: all of a user's
	// devices up to the cap, so one notification costs about one
	// SendTimeout however its devices answer.
	parallel = MaxDevices

	maxTitle = 120
	maxBody  = 600
)

// Settings describe this server to devices.
type Settings struct {
	// PublicKey is the VAPID application server key (base64url) that
	// browsers and distributors subscribe with.
	PublicKey string
	AppName   string
	// Link is where a notification without its own link opens.
	Link string
}

// Service registers devices and delivers push notifications to them.
type Service struct {
	repo     Repository
	sender   Sender
	clock    clock.Clock
	settings Settings
	timeout  time.Duration
}

// NewService builds a Service. A nil sender disables push: devices cannot
// register and nothing is sent.
func NewService(repo Repository, sender Sender, c clock.Clock, settings Settings) *Service {
	return &Service{repo: repo, sender: sender, clock: c, settings: settings, timeout: SendTimeout}
}

// Enabled reports whether this server sends push notifications.
func (s *Service) Enabled() bool { return s != nil && s.sender != nil && s.settings.PublicKey != "" }

// PublicKey is the VAPID key devices subscribe with; empty when disabled.
func (s *Service) PublicKey() string {
	if !s.Enabled() {
		return ""
	}
	return s.settings.PublicKey
}

func (s *Service) available() error {
	if !s.Enabled() {
		return errs.Conflict("Push notifications are not enabled on this server.")
	}
	return nil
}

// Register adds the actor's device or refreshes it when the endpoint is
// already known. Registering is idempotent on the endpoint; an endpoint
// registered by someone else moves to the actor, because a browser or phone
// belongs to whoever is signed in on it now.
func (s *Service) Register(ctx context.Context, actor uuid.UUID, in Input) (*Subscription, bool, error) {
	if err := s.available(); err != nil {
		return nil, false, err
	}
	sub, err := New(actor, in, s.clock.Now())
	if err != nil {
		return nil, false, err
	}
	known, err := s.repo.Exists(ctx, sub.Endpoint)
	if err != nil {
		return nil, false, err
	}
	if !known {
		n, err := s.repo.Count(ctx, actor)
		if err != nil {
			return nil, false, err
		}
		if n >= MaxDevices {
			return nil, false, errs.Conflict("You have too many devices registered for push notifications. Remove one first.")
		}
	}
	created, err := s.repo.Save(ctx, sub)
	if err != nil {
		return nil, false, err
	}
	return sub, created, nil
}

// List returns the actor's devices.
func (s *Service) List(ctx context.Context, actor uuid.UUID) ([]*Subscription, error) {
	return s.repo.List(ctx, actor)
}

// Remove deletes one of the actor's devices.
func (s *Service) Remove(ctx context.Context, actor, id uuid.UUID) error {
	return s.repo.Delete(ctx, actor, id)
}

// Report counts what happened to one delivery.
type Report struct {
	Sent    int
	Failed  int
	Removed int
	// Err is one of the delivery failures (the last device's), for logs.
	Err error
}

// Message is the payload devices receive: small JSON that the web app's
// service worker and the Android app show as a notification.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// Payload encodes m, trimming long text so it stays well under the 4 KB
// Web Push limit.
func (m Message) Payload() []byte {
	m.Title, m.Body = clip(m.Title, maxTitle), clip(m.Body, maxBody)
	b, _ := json.Marshal(m)
	return b
}

func clip(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max-1]) + "…"
}

// Test sends a test notification to every device of the actor, ignoring
// preferences, and waits for the result.
func (s *Service) Test(ctx context.Context, actor uuid.UUID) (Report, error) {
	if err := s.available(); err != nil {
		return Report{}, err
	}
	subs, err := s.repo.List(ctx, actor)
	if err != nil {
		return Report{}, err
	}
	if len(subs) == 0 {
		return Report{}, errs.Conflict("No devices are registered for push notifications yet.")
	}
	return s.deliver(ctx, subs, Message{Title: "Test notification", Body: "Push notifications from " + s.settings.AppName + " work on this device.",
		URL: s.settings.Link}), nil
}

// Notify delivers n to the push devices of n.UserID. It does nothing when
// push is disabled or n has no push recipient. Callers decide channels (see
// user.User.Notice); Notify does not look at preferences.
func (s *Service) Notify(ctx context.Context, n ports.Notification) (Report, error) {
	if !s.Enabled() || n.UserID == uuid.Nil {
		return Report{}, nil
	}
	subs, err := s.repo.List(ctx, n.UserID)
	if err != nil {
		return Report{}, err
	}
	link := n.Link
	if link == "" {
		link = s.settings.Link
	}
	body := strings.Join(n.Lines, " ")
	if body == "" {
		body = n.Subject
	}
	return s.deliver(ctx, subs, Message{Title: n.Subject, Body: body, URL: link}), nil
}

// deliver sends to the devices concurrently, each within its own timeout.
// Devices the push service forgot are removed; other failures are counted,
// and a device that keeps failing is eventually removed too.
func (s *Service) deliver(ctx context.Context, subs []*Subscription, m Message) Report {
	payload := m.Payload()
	results := make([]Report, len(subs))
	slots := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, sub := range subs {
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			results[i] = s.send(ctx, sub, payload)
		}()
	}
	wg.Wait()
	var r Report
	for _, one := range results {
		r.Sent, r.Failed, r.Removed = r.Sent+one.Sent, r.Failed+one.Failed, r.Removed+one.Removed
		if one.Err != nil {
			r.Err = one.Err
		}
	}
	return r
}

// send delivers payload to one device and records the outcome.
func (s *Service) send(ctx context.Context, sub *Subscription, payload []byte) Report {
	sendCtx, cancel := context.WithTimeout(ctx, s.timeout)
	err := s.sender.Send(sendCtx, sub, payload)
	cancel()
	switch {
	case err == nil:
		_ = s.repo.Succeeded(ctx, sub.ID, s.clock.Now())
		return Report{Sent: 1}
	case errors.Is(err, ErrGone):
		_ = s.repo.Remove(ctx, sub.ID)
		return Report{Removed: 1}
	default:
		r := Report{Failed: 1, Err: err}
		if n, ferr := s.repo.Failed(ctx, sub.ID); ferr == nil && n >= MaxFailures {
			r.Removed = 1
			_ = s.repo.Remove(ctx, sub.ID)
		}
		return r
	}
}
