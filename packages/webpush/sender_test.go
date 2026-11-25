package webpush

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/google/uuid"
)

var b64 = base64.RawURLEncoding

// device is a fake browser: the subscriber key pair and auth secret that
// PushManager.subscribe would create.
type device struct {
	key  *ecdh.PrivateKey
	auth []byte
}

func newDevice(t *testing.T) device {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	return device{key: key, auth: auth}
}

func (d device) subscription(endpoint string) *push.Subscription {
	return &push.Subscription{ID: uuid.New(), Endpoint: endpoint, P256dh: b64.EncodeToString(d.key.PublicKey().Bytes()), Auth: b64.EncodeToString(d.auth)}
}

// decrypt reverses RFC 8291 / RFC 8188 (aes128gcm) as a user agent does.
func (d device) decrypt(t *testing.T, body []byte) []byte {
	t.Helper()
	if len(body) < 21 {
		t.Fatalf("body too short: %d", len(body))
	}
	salt, rs, idlen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	if idlen != 65 {
		t.Fatalf("key id length %d, want 65", idlen)
	}
	serverPublic, ciphertext := body[21:21+idlen], body[21+idlen:]
	if len(ciphertext) > int(rs) {
		t.Fatalf("record larger than rs: %d > %d", len(ciphertext), rs)
	}
	as, err := ecdh.P256().NewPublicKey(serverPublic)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := d.key.ECDH(as)
	if err != nil {
		t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), d.key.PublicKey().Bytes()...), serverPublic...)
	prkKey, _ := hkdf.Extract(sha256.New, secret, d.auth)
	ikm, _ := hkdf.Expand(sha256.New, prkKey, string(info), 32)
	prk, _ := hkdf.Extract(sha256.New, ikm, salt)
	cek, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	end := len(plain) - 1
	for end >= 0 && plain[end] == 0 {
		end--
	}
	if end < 0 || plain[end] != 2 {
		t.Fatal("missing last-record delimiter")
	}
	return plain[:end]
}

type received struct {
	header http.Header
	body   []byte
	host   string
}

// pushService is a fake push service (the role of FCM, Mozilla autopush or
// an ntfy server): it records requests and answers by path.
func pushService(t *testing.T) (*httptest.Server, func() []received) {
	var mu sync.Mutex
	var got []received
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{header: r.Header.Clone(), body: body, host: r.Host})
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/gone"):
			w.WriteHeader(http.StatusGone)
		case strings.HasSuffix(r.URL.Path, "/missing"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/busy"):
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []received { mu.Lock(); defer mu.Unlock(); return append([]received(nil), got...) }
}

// verifyVAPID checks the Authorization header (RFC 8292) and returns the
// JWT claims.
func verifyVAPID(t *testing.T, header, public string) map[string]any {
	t.Helper()
	if !strings.HasPrefix(header, "vapid ") {
		t.Fatalf("authorization scheme: %q", header)
	}
	params := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(header, "vapid "), ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		params[k] = v
	}
	if params["k"] != public {
		t.Fatalf("k = %q, want the server public key", params["k"])
	}
	segments := strings.Split(params["t"], ".")
	if len(segments) != 3 {
		t.Fatalf("malformed JWT %q", params["t"])
	}
	var head map[string]any
	raw, _ := b64.DecodeString(segments[0])
	_ = json.Unmarshal(raw, &head)
	if head["alg"] != "ES256" {
		t.Fatalf("alg = %v", head["alg"])
	}
	keyBytes, _ := b64.DecodeString(public)
	x, y := elliptic.Unmarshal(elliptic.P256(), keyBytes) //nolint:staticcheck // test-only parsing of a raw point
	sig, _ := b64.DecodeString(segments[2])
	if len(sig) != 64 {
		t.Fatalf("signature length %d", len(sig))
	}
	digest := sha256.Sum256([]byte(segments[0] + "." + segments[1]))
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("VAPID signature does not verify with the server public key")
	}
	var claims map[string]any
	raw, _ = b64.DecodeString(segments[1])
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestSendEncryptsAndSigns(t *testing.T) {
	srv, got := pushService(t)
	public, private, err := Keys()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := New(Config{PublicKey: public, PrivateKey: private, Subject: "mailto:admin@split.example.com", Client: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	dev := newDevice(t)
	payload := push.Message{Title: "Ben added “Rent”", Body: "Your share: €500.00.", URL: "https://split.example.com"}.Payload()
	if err := sender.Send(context.Background(), dev.subscription(srv.URL+"/push/abc"), payload); err != nil {
		t.Fatal(err)
	}
	reqs := got()
	if len(reqs) != 1 {
		t.Fatalf("want one request, got %d", len(reqs))
	}
	r := reqs[0]
	if r.header.Get("Content-Encoding") != "aes128gcm" || r.header.Get("TTL") != strconv.Itoa(int(TTL.Seconds())) || r.header.Get("Urgency") != "normal" {
		t.Fatalf("unexpected headers %v", r.header)
	}
	claims := verifyVAPID(t, r.header.Get("Authorization"), public)
	if claims["aud"] != "https://"+r.host {
		t.Fatalf("aud = %v, want the push service origin", claims["aud"])
	}
	if claims["sub"] != "mailto:admin@split.example.com" {
		t.Fatalf("sub = %v (the mailto: prefix must not be doubled)", claims["sub"])
	}
	exp := time.Unix(int64(claims["exp"].(float64)), 0)
	if until := time.Until(exp); until <= 0 || until > 24*time.Hour {
		t.Fatalf("exp must be within 24 hours, is %v away", until)
	}
	if plain := dev.decrypt(t, r.body); string(plain) != string(payload) {
		t.Fatalf("decrypted %q, want %q", plain, payload)
	}
	if len(r.body) > 1024 {
		t.Fatalf("small payloads should not be padded to 4 KB: %d bytes", len(r.body))
	}
}

func TestSendStatuses(t *testing.T) {
	srv, _ := pushService(t)
	public, private, _ := Keys()
	sender, err := New(Config{PublicKey: public, PrivateKey: private, Subject: "https://split.example.com", Client: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	dev := newDevice(t)
	for path, want := range map[string]error{"/gone": push.ErrGone, "/missing": push.ErrGone} {
		if err := sender.Send(context.Background(), dev.subscription(srv.URL+path), []byte(`{}`)); !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", path, err, want)
		}
	}
	if err := sender.Send(context.Background(), dev.subscription(srv.URL+"/busy"), []byte(`{}`)); err == nil || errors.Is(err, push.ErrGone) {
		t.Errorf("429 should be a plain failure, got %v", err)
	}
}

func TestDefaultClientRefusesLoopback(t *testing.T) {
	srv, got := pushService(t)
	public, private, _ := Keys()
	sender, err := New(Config{PublicKey: public, PrivateKey: private, Subject: "mailto:a@b.c"})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(context.Background(), newDevice(t).subscription(srv.URL+"/push/abc"), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "webpush.endpoint.blocked") {
		t.Fatalf("want a blocked error, got %v", err)
	}
	if len(got()) != 0 {
		t.Fatal("the request must not reach a loopback endpoint")
	}
}

func TestCheckKeys(t *testing.T) {
	public, private, err := Keys()
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(public, private); err != nil {
		t.Fatal(err)
	}
	otherPublic, _, _ := Keys()
	if err := Check(otherPublic, private); err == nil {
		t.Fatal("mismatched keys must be rejected")
	}
	if err := Check("nope", private); err == nil {
		t.Fatal("garbage must be rejected")
	}
	if _, err := New(Config{PublicKey: public, PrivateKey: private, Subject: "admin@example.com"}); err == nil {
		t.Fatal("a subject without mailto: or https:// must be rejected")
	}
}
