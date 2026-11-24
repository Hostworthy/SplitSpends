package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type memRepo struct{ subs map[uuid.UUID]*Subscription }

func (m *memRepo) Save(_ context.Context, s *Subscription) (bool, error) {
	for _, old := range m.subs {
		if old.Endpoint == s.Endpoint {
			s.ID, s.CreatedAt = old.ID, old.CreatedAt
			*old = *s
			return false, nil
		}
	}
	c := *s
	m.subs[s.ID] = &c
	return true, nil
}

func (m *memRepo) List(_ context.Context, userID uuid.UUID) ([]*Subscription, error) {
	var out []*Subscription
	for _, s := range m.subs {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memRepo) Count(ctx context.Context, userID uuid.UUID) (int, error) {
	l, _ := m.List(ctx, userID)
	return len(l), nil
}

func (m *memRepo) Exists(_ context.Context, endpoint string) (bool, error) {
	for _, s := range m.subs {
		if s.Endpoint == endpoint {
			return true, nil
		}
	}
	return false, nil
}

func (m *memRepo) Delete(_ context.Context, userID, id uuid.UUID) error {
	if s, ok := m.subs[id]; !ok || s.UserID != userID {
		return errs.NotFound("Device")
	}
	delete(m.subs, id)
	return nil
}

func (m *memRepo) Remove(_ context.Context, id uuid.UUID) error { delete(m.subs, id); return nil }

func (m *memRepo) Succeeded(_ context.Context, id uuid.UUID, at time.Time) error {
	if s := m.subs[id]; s != nil {
		s.LastSuccessAt, s.Failures = &at, 0
	}
	return nil
}

func (m *memRepo) Failed(_ context.Context, id uuid.UUID) (int, error) {
	s := m.subs[id]
	if s == nil {
		return 0, nil
	}
	s.Failures++
	return s.Failures, nil
}

// fakeSender answers per endpoint and records payloads.
type fakeSender struct {
	answers  map[string]error
	payloads [][]byte
}

func (f *fakeSender) Send(_ context.Context, s *Subscription, payload []byte) error {
	f.payloads = append(f.payloads, payload)
	return f.answers[s.Endpoint]
}

var (
	testKey  = base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...))
	testAuth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
)

func input(endpoint string) Input {
	return Input{Endpoint: endpoint, P256dh: testKey, Auth: testAuth, Platform: "web", Label: "Firefox on Linux"}
}

func setup() (*Service, *memRepo, *fakeSender) {
	repo := &memRepo{subs: map[uuid.UUID]*Subscription{}}
	sender := &fakeSender{answers: map[string]error{}}
	svc := NewService(repo, sender, clock.Fixed{At: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)},
		Settings{PublicKey: "key", AppName: "SplitSpends", Link: "https://split.example.com"})
	return svc, repo, sender
}

func TestNewValidates(t *testing.T) {
	me := uuid.New()
	bad := []Input{
		{Endpoint: "http://push.example.com/x", P256dh: testKey, Auth: testAuth},
		{Endpoint: "not a url", P256dh: testKey, Auth: testAuth},
		{Endpoint: "https://push.example.com/x", P256dh: "short", Auth: testAuth},
		{Endpoint: "https://push.example.com/x", P256dh: testKey, Auth: "c2hvcnQ"},
		{Endpoint: "https://push.example.com/x", P256dh: testKey, Auth: testAuth, Platform: "apns"},
		{Endpoint: "https://push.example.com/" + strings.Repeat("a", 2100), P256dh: testKey, Auth: testAuth},
	}
	for _, in := range bad {
		if _, err := New(me, in, time.Now()); !errs.Is(err, errs.KindValidation) {
			t.Errorf("%+v: want a validation error, got %v", in, err)
		}
	}
	s, err := New(me, Input{Endpoint: " https://push.example.com/x ", P256dh: testKey + "=", Auth: testAuth, Label: "  My   phone "}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if s.Platform != PlatformWeb || s.Label != "My phone" || s.Endpoint != "https://push.example.com/x" {
		t.Fatalf("unexpected normalisation: %+v", s)
	}
}

func TestRegisterIsIdempotent(t *testing.T) {
	svc, repo, _ := setup()
	ctx := context.Background()
	me := uuid.New()
	first, created, err := svc.Register(ctx, me, input("https://push.example.com/a"))
	if err != nil || !created {
		t.Fatalf("first register: created=%v err=%v", created, err)
	}
	again, created, err := svc.Register(ctx, me, input("https://push.example.com/a"))
	if err != nil || created || again.ID != first.ID || len(repo.subs) != 1 {
		t.Fatalf("second register should refresh: created=%v id=%v rows=%d err=%v", created, again.ID, len(repo.subs), err)
	}
	// The same browser signed in as someone else moves to them.
	other := uuid.New()
	if _, _, err := svc.Register(ctx, other, input("https://push.example.com/a")); err != nil {
		t.Fatal(err)
	}
	if mine, _ := svc.List(ctx, me); len(mine) != 0 {
		t.Fatalf("endpoint should have moved, still listed for the old user")
	}
}

func TestRegisterCapsDevices(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()
	me := uuid.New()
	for i := 0; i < MaxDevices; i++ {
		if _, _, err := svc.Register(ctx, me, input("https://push.example.com/"+uuid.NewString())); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := svc.Register(ctx, me, input("https://push.example.com/one-more")); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("want conflict over the cap, got %v", err)
	}
}

func TestDisabled(t *testing.T) {
	svc := NewService(&memRepo{subs: map[uuid.UUID]*Subscription{}}, nil, clock.System{}, Settings{})
	if svc.Enabled() || svc.PublicKey() != "" {
		t.Fatal("a service without a sender must be disabled")
	}
	if _, _, err := svc.Register(context.Background(), uuid.New(), input("https://push.example.com/a")); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("register should fail when disabled, got %v", err)
	}
	if r, err := svc.Notify(context.Background(), ports.Notification{UserID: uuid.New(), Subject: "x"}); err != nil || r != (Report{}) {
		t.Fatalf("notify should do nothing when disabled: %+v %v", r, err)
	}
}

func TestNotifyRemovesGoneAndCountsFailures(t *testing.T) {
	svc, repo, sender := setup()
	ctx := context.Background()
	me := uuid.New()
	for _, e := range []string{"ok", "gone", "down"} {
		if _, _, err := svc.Register(ctx, me, input("https://push.example.com/"+e)); err != nil {
			t.Fatal(err)
		}
	}
	sender.answers["https://push.example.com/gone"] = ErrGone
	sender.answers["https://push.example.com/down"] = errors.New("webpush.send.rejected: status 503")

	r, err := svc.Notify(ctx, ports.Notification{UserID: me, Subject: "Ben added “Rent”", Lines: []string{"Ben added “Rent”.", "Your share: €500.00."}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Sent != 1 || r.Failed != 1 || r.Removed != 1 || r.Err == nil {
		t.Fatalf("unexpected report %+v", r)
	}
	if len(repo.subs) != 2 {
		t.Fatalf("the gone subscription should be removed, %d left", len(repo.subs))
	}
	for _, s := range repo.subs {
		switch s.Endpoint {
		case "https://push.example.com/ok":
			if s.LastSuccessAt == nil {
				t.Fatal("success not recorded")
			}
		case "https://push.example.com/down":
			if s.Failures != 1 {
				t.Fatalf("failures = %d, want 1", s.Failures)
			}
		}
	}
	var m Message
	if err := json.Unmarshal(sender.payloads[0], &m); err != nil {
		t.Fatal(err)
	}
	if m.Title != "Ben added “Rent”" || m.Body != "Ben added “Rent”. Your share: €500.00." || m.URL != "https://split.example.com" {
		t.Fatalf("unexpected payload %+v", m)
	}

	// A device that keeps failing is eventually dropped.
	for i := 1; i < MaxFailures; i++ {
		if _, err := svc.Notify(ctx, ports.Notification{UserID: me, Subject: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.subs) != 1 {
		t.Fatalf("a device failing %d times in a row should be removed", MaxFailures)
	}
}

func TestNotifySkipsWithoutRecipient(t *testing.T) {
	svc, _, sender := setup()
	if _, err := svc.Notify(context.Background(), ports.Notification{To: "ada@example.com", Subject: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(sender.payloads) != 0 {
		t.Fatal("email-only notifications must not be pushed")
	}
}

func TestTest(t *testing.T) {
	svc, _, sender := setup()
	ctx := context.Background()
	me := uuid.New()
	if _, err := svc.Test(ctx, me); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("testing without devices should explain, got %v", err)
	}
	if _, _, err := svc.Register(ctx, me, input("https://push.example.com/a")); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Test(ctx, me)
	if err != nil || r.Sent != 1 {
		t.Fatalf("test: %+v %v", r, err)
	}
	if !strings.Contains(string(sender.payloads[0]), "Test notification") {
		t.Fatalf("unexpected payload %s", sender.payloads[0])
	}
}

func TestPayloadStaysSmall(t *testing.T) {
	p := Message{Title: strings.Repeat("é", 500), Body: strings.Repeat("€", 5000), URL: "https://split.example.com"}.Payload()
	if len(p) > 3000 {
		t.Fatalf("payload too large: %d bytes", len(p))
	}
	var m Message
	if err := json.Unmarshal(p, &m); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(m.Body, "…") {
		t.Fatal("clipped text should end with an ellipsis")
	}
}
