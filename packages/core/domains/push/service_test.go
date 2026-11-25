package push

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// memRepo is safe for the concurrent sends of one delivery.
type memRepo struct {
	mu   sync.Mutex
	subs map[uuid.UUID]*Subscription
	// removeErr, when set, is what Remove fails with.
	removeErr error
}

func (m *memRepo) Save(_ context.Context, s *Subscription, limit int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	others := 0
	for _, old := range m.subs {
		if old.UserID == s.UserID && old.Endpoint != s.Endpoint {
			others++
		}
	}
	if others >= limit {
		return false, ErrFull
	}
	for _, old := range m.subs {
		if old.Endpoint == s.Endpoint {
			s.ID, s.CreatedAt = old.ID, old.CreatedAt
			if old.P256dh == s.P256dh && old.Auth == s.Auth {
				s.Failures = old.Failures
			}
			*old = *s
			return false, nil
		}
	}
	c := *s
	m.subs[s.ID] = &c
	return true, nil
}

func (m *memRepo) List(_ context.Context, userID uuid.UUID) ([]*Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Subscription
	for _, s := range m.subs {
		if s.UserID == userID {
			c := *s
			out = append(out, &c)
		}
	}
	return out, nil
}

func (m *memRepo) Delete(_ context.Context, userID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.subs[id]; !ok || s.UserID != userID {
		return errs.NotFound("Device")
	}
	delete(m.subs, id)
	return nil
}

func (m *memRepo) Remove(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.removeErr != nil {
		return m.removeErr
	}
	delete(m.subs, id)
	return nil
}

func (m *memRepo) Succeeded(ctx context.Context, id uuid.UUID, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s := m.subs[id]; s != nil {
		s.LastSuccessAt, s.Failures = &at, 0
	}
	return nil
}

func (m *memRepo) Failed(ctx context.Context, id uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s := m.subs[id]
	if s == nil {
		return 0, nil
	}
	s.Failures++
	return s.Failures, nil
}

// fakeSender answers per endpoint and records payloads. Endpoints in hang
// never answer: Send waits until its context ends.
type fakeSender struct {
	mu       sync.Mutex
	answers  map[string]error
	hang     map[string]bool
	payloads [][]byte
}

func (f *fakeSender) Send(ctx context.Context, s *Subscription, payload []byte) error {
	f.mu.Lock()
	f.payloads = append(f.payloads, payload)
	hang, answer := f.hang[s.Endpoint], f.answers[s.Endpoint]
	f.mu.Unlock()
	if hang {
		<-ctx.Done()
		return ctx.Err()
	}
	return answer
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
	sender := &fakeSender{answers: map[string]error{}, hang: map[string]bool{}}
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
		if _, err := New(me, uuid.New(), in, time.Now()); !errs.Is(err, errs.KindValidation) {
			t.Errorf("%+v: want a validation error, got %v", in, err)
		}
	}
	s, err := New(me, uuid.New(), Input{Endpoint: " https://push.example.com/x ", P256dh: testKey + "=", Auth: testAuth, Label: "  My   phone "}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if s.Platform != PlatformWeb || s.Label != "My phone" || s.Endpoint != "https://push.example.com/x" {
		t.Fatalf("unexpected normalisation: %+v", s)
	}
}

func TestBlocked(t *testing.T) {
	type blockCase struct {
		ip           string
		allowPrivate bool
		blocked      bool
	}
	cases := []blockCase{
		{"142.250.74.106", false, false},
		{"2a00:1450:4001:80b::200a", false, false},
		{"127.0.0.1", false, true},
		{"::1", false, true},
		{"10.0.0.8", false, true},
		{"192.168.1.20", false, true},
		{"100.101.102.103", false, true},
		{"fd00::1", false, true},
		{"169.254.169.254", false, true},
		{"0.0.0.0", false, true},
		{"192.168.1.20", true, false},
		{"127.0.0.1", true, false},
		{"169.254.169.254", true, true},
		{"fe80::1", true, true},
		{"::ffff:10.0.0.8", false, true},
		{"::ffff:142.250.74.106", false, false},
		{"64:ff9b::8efa:4a6a", false, false},
		{"2002:8efa:4a6a::1", false, false},
		{"64:ff9b::a00:8", false, true},
		{"64:ff9b::a00:8", true, false},
		{"fe80::1%eth0", true, true},
		{"fd00::1%eth0", false, true},
	}
	// Reserved and cloud metadata ranges stay blocked whatever the policy.
	for _, ip := range []string{"192.0.0.192", "64:ff9b::a9fe:a9fe", "2002:a9fe:a9fe::1", "0.1.2.3", "240.0.0.1",
		"198.18.0.1", "255.255.255.255", "224.0.0.1", "ff02::1", "::", "fd00:ec2::254", "100.100.100.200"} {
		cases = append(cases, blockCase{ip, false, true}, blockCase{ip, true, true})
	}
	for _, tc := range cases {
		if got := Blocked(netip.MustParseAddr(tc.ip), tc.allowPrivate); got != tc.blocked {
			t.Errorf("Blocked(%s, %v) = %v, want %v", tc.ip, tc.allowPrivate, got, tc.blocked)
		}
	}
}

func TestRegisterRefusesBlockedAddresses(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()
	me := uuid.New()
	for _, endpoint := range []string{"https://127.0.0.1/x", "https://169.254.169.254/x", "https://[::1]:8443/x", "https://10.0.0.8/x"} {
		if _, _, err := svc.Register(ctx, me, uuid.New(), input(endpoint)); !errs.Is(err, errs.KindValidation) {
			t.Errorf("%s: want a validation error, got %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"https://push.example.com:8443/x", "https://142.250.74.106/x"} {
		if _, _, err := svc.Register(ctx, me, uuid.New(), input(endpoint)); err != nil {
			t.Errorf("%s: %v", endpoint, err)
		}
	}

	svc.settings.AllowPrivate = true
	if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://192.168.1.20:2586/up")); err != nil {
		t.Errorf("a LAN distributor should be allowed when private endpoints are: %v", err)
	}
	if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://169.254.169.254/x")); !errs.Is(err, errs.KindValidation) {
		t.Errorf("metadata addresses stay refused, got %v", err)
	}
}

func TestRegisterIsIdempotent(t *testing.T) {
	svc, repo, _ := setup()
	ctx := context.Background()
	me := uuid.New()
	first, created, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/a"))
	if err != nil || !created {
		t.Fatalf("first register: created=%v err=%v", created, err)
	}
	again, created, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/a"))
	if err != nil || created || again.ID != first.ID || len(repo.subs) != 1 {
		t.Fatalf("second register should refresh: created=%v id=%v rows=%d err=%v", created, again.ID, len(repo.subs), err)
	}
	// The same browser signed in as someone else moves to them.
	other := uuid.New()
	if _, _, err := svc.Register(ctx, other, uuid.New(), input("https://push.example.com/a")); err != nil {
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
		if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/"+uuid.NewString())); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/one-more")); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("want conflict over the cap, got %v", err)
	}
	// Taking over someone else's device counts too.
	other := uuid.New()
	if _, _, err := svc.Register(ctx, other, uuid.New(), input("https://push.example.com/theirs")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/theirs")); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("want conflict when moving a device over the cap, got %v", err)
	}
	// Refreshing a device already counted is fine.
	mine, _ := svc.List(ctx, me)
	if _, _, err := svc.Register(ctx, me, uuid.New(), input(mine[0].Endpoint)); err != nil {
		t.Fatalf("refreshing a device at the cap: %v", err)
	}
}

func TestDisabled(t *testing.T) {
	svc := NewService(&memRepo{subs: map[uuid.UUID]*Subscription{}}, nil, clock.System{}, Settings{})
	if svc.Enabled() || svc.PublicKey() != "" {
		t.Fatal("a service without a sender must be disabled")
	}
	if _, _, err := svc.Register(context.Background(), uuid.New(), uuid.New(), input("https://push.example.com/a")); !errs.Is(err, errs.KindConflict) {
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
		if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/"+e)); err != nil {
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

// TestSlowDevicesDoNotHoldUpOthers checks that devices are sent to at once,
// each within its own timeout: a user whose devices never answer costs one
// timeout, not one per device, and their working device still gets the
// message.
func TestSlowDevicesDoNotHoldUpOthers(t *testing.T) {
	svc, repo, sender := setup()
	svc.timeout = 200 * time.Millisecond
	ctx := context.Background()
	me := uuid.New()
	for i := range MaxDevices - 1 {
		endpoint := fmt.Sprintf("https://slow.example.com/%d", i)
		sender.hang[endpoint] = true
		if _, _, err := svc.Register(ctx, me, uuid.New(), input(endpoint)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/fast")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	start := time.Now()
	r, err := svc.Notify(ctx, ports.Notification{UserID: me, Subject: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 5*svc.timeout {
		t.Fatalf("delivery took %v; devices should time out together, not one after another", took)
	}
	if r.Sent != 1 || r.Failed != MaxDevices-1 || !errors.Is(r.Err, context.DeadlineExceeded) {
		t.Fatalf("unexpected report %+v", r)
	}
	for _, s := range repo.subs {
		if s.Endpoint == "https://push.example.com/fast" && s.LastSuccessAt == nil {
			t.Fatal("the working device should be marked delivered")
		}
	}
}

func TestNotifyReportsStoreErrors(t *testing.T) {
	svc, repo, sender := setup()
	ctx := context.Background()
	me := uuid.New()
	if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/gone")); err != nil {
		t.Fatal(err)
	}
	sender.answers["https://push.example.com/gone"] = ErrGone
	repo.removeErr = errors.New("database.unavailable")
	r, err := svc.Notify(ctx, ports.Notification{UserID: me, Subject: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(r.Store, repo.removeErr) {
		t.Fatalf("the failed removal should be reported, got %+v", r)
	}
}

// TestNotifyRecordsAfterDeadline checks that outcomes are recorded even
// when slow devices used up the delivery's time.
func TestNotifyRecordsAfterDeadline(t *testing.T) {
	svc, repo, sender := setup()
	me := uuid.New()
	if _, _, err := svc.Register(context.Background(), me, uuid.New(), input("https://push.example.com/slow")); err != nil {
		t.Fatal(err)
	}
	sender.hang["https://push.example.com/slow"] = true
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r, err := svc.Notify(ctx, ports.Notification{UserID: me, Subject: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Failed != 1 || r.Store != nil {
		t.Fatalf("unexpected report %+v", r)
	}
	for _, s := range repo.subs {
		if s.Failures != 1 {
			t.Fatalf("the timed-out delivery should be counted, failures = %d", s.Failures)
		}
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
	if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/a")); err != nil {
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

// TestTestHasDeadline checks that a test push waits for at most a few send
// timeouts, however many devices never answer.
func TestTestHasDeadline(t *testing.T) {
	svc, _, sender := setup()
	svc.timeout = 100 * time.Millisecond
	ctx := context.Background()
	me := uuid.New()
	for i := range MaxDevices {
		endpoint := fmt.Sprintf("https://slow.example.com/%d", i)
		sender.hang[endpoint] = true
		if _, _, err := svc.Register(ctx, me, uuid.New(), input(endpoint)); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	r, err := svc.Test(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 3*svc.timeout+time.Second {
		t.Fatalf("test push took %v", took)
	}
	if r.Failed != MaxDevices {
		t.Fatalf("unexpected report %+v", r)
	}
}

func TestPayloadStaysSmall(t *testing.T) {
	for _, m := range []Message{
		{Title: strings.Repeat("é", 500), Body: strings.Repeat("€", 5000)},
		{Title: strings.Repeat("<", 500), Body: strings.Repeat("&", 5000)},
		{Title: strings.Repeat("<", 500), Body: strings.Repeat("\u2028", 5000)},
		{Title: strings.Repeat("\x01", 500), Body: strings.Repeat("\x02", 5000)},
	} {
		m.URL = "https://split.example.com"
		p := m.Payload()
		if len(p) > MaxPayload {
			t.Errorf("payload of %.10q is %d bytes, more than %d", m.Body, len(p), MaxPayload)
		}
		var got Message
		if err := json.Unmarshal(p, &got); err != nil {
			t.Fatal(err)
		}
		if got.Body != "" && !strings.HasSuffix(got.Body, "…") || got.URL != m.URL {
			t.Errorf("clipped text should end with an ellipsis and keep the link: %+v", got)
		}
	}
	if p := (Message{Title: "Tom & Jerry <3"}).Payload(); !strings.Contains(string(p), "Tom & Jerry <3") {
		t.Errorf("the payload is not HTML, so it should not be escaped: %s", p)
	}
}

// TestOversizePayloadIsNotTheDevice checks that a message the sender
// cannot fit is reported but not counted against the device.
func TestOversizePayloadIsNotTheDevice(t *testing.T) {
	svc, repo, sender := setup()
	ctx := context.Background()
	me := uuid.New()
	if _, _, err := svc.Register(ctx, me, uuid.New(), input("https://push.example.com/a")); err != nil {
		t.Fatal(err)
	}
	sender.answers["https://push.example.com/a"] = fmt.Errorf("webpush.send.failed: %w", ErrTooLarge)
	r, err := svc.Notify(ctx, ports.Notification{UserID: me, Subject: "x"})
	if err != nil || r.Failed != 1 || r.Err == nil {
		t.Fatalf("unexpected report %+v %v", r, err)
	}
	for _, s := range repo.subs {
		if s.Failures != 0 {
			t.Fatalf("failures = %d, want 0", s.Failures)
		}
	}
}
