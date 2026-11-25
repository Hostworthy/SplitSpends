// Package webpush implements push.Sender with standard Web Push: message
// encryption (RFC 8291, aes128gcm) and VAPID (RFC 8292). The same requests
// reach browsers through their push services and Android phones through
// UnifiedPush distributors, so no vendor SDK or account is needed.
package webpush

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	wp "github.com/SherClockHolmes/webpush-go"
	"golang.org/x/net/http/httpproxy"
)

// TTL is how long push services keep a message for an offline device.
const TTL = 24 * time.Hour

// Config holds the VAPID identity and network policy.
type Config struct {
	PublicKey  string
	PrivateKey string
	// Subject is a mailto: or https: contact for push service operators.
	Subject string
	// AllowPrivate lets endpoints resolve to private or loopback addresses,
	// for a UnifiedPush server (such as ntfy) on the local network.
	AllowPrivate bool
	// Client replaces the HTTP client (tests). Without it the sender uses
	// a client that refuses private addresses unless AllowPrivate is set.
	Client *http.Client
}

// Sender delivers Web Push messages.
type Sender struct {
	cfg    Config
	client *http.Client
}

// New validates the keys and builds a Sender.
func New(cfg Config) (*Sender, error) {
	if err := Check(cfg.PublicKey, cfg.PrivateKey); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(cfg.Subject, "mailto:") && !strings.HasPrefix(cfg.Subject, "https://") {
		return nil, fmt.Errorf("webpush.subject.invalid: use a mailto: or https:// URL")
	}
	client := cfg.Client
	if client == nil {
		client = guarded(cfg.AllowPrivate)
	}
	return &Sender{cfg: cfg, client: client}, nil
}

// Keys generates a VAPID key pair (base64url, unpadded).
func Keys() (public, private string, err error) {
	private, public, err = wp.GenerateVAPIDKeys()
	if err != nil {
		return "", "", fmt.Errorf("webpush.keys.generate.failed: %w", err)
	}
	return public, private, nil
}

// Check verifies that public and private form a P-256 key pair.
func Check(public, private string) error {
	pub, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(public, "="))
	if err != nil || len(pub) != 65 {
		return fmt.Errorf("webpush.keys.public.invalid: expected a base64url uncompressed P-256 point")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(private, "="))
	if err != nil || len(raw) != 32 {
		return fmt.Errorf("webpush.keys.private.invalid: expected 32 base64url bytes")
	}
	key, err := ecdh.P256().NewPrivateKey(raw)
	if err != nil {
		return fmt.Errorf("webpush.keys.private.invalid: %w", err)
	}
	if string(key.PublicKey().Bytes()) != string(pub) {
		return fmt.Errorf("webpush.keys.mismatch: the public key does not belong to the private key")
	}
	return nil
}

// PublicKey is the VAPID key devices subscribe with.
func (w *Sender) PublicKey() string { return w.cfg.PublicKey }

// Send encrypts payload for s and posts it to the push service. A 404 or
// 410 answer returns push.ErrGone, and a payload larger than
// push.MaxPayload returns push.ErrTooLarge without contacting anyone.
func (w *Sender) Send(ctx context.Context, s *push.Subscription, payload []byte) error {
	if len(payload) > push.MaxPayload {
		return fmt.Errorf("webpush.send.failed: %w", push.ErrTooLarge)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := wp.SendNotificationWithContext(ctx, payload, &wp.Subscription{
		Endpoint: s.Endpoint, Keys: wp.Keys{P256dh: s.P256dh, Auth: s.Auth},
	}, &wp.Options{
		HTTPClient: w.client,
		// The library prefixes "mailto:" itself unless the subject is an
		// https URL.
		Subscriber:      strings.TrimPrefix(w.cfg.Subject, "mailto:"),
		VAPIDPublicKey:  w.cfg.PublicKey,
		VAPIDPrivateKey: w.cfg.PrivateKey,
		TTL:             int(TTL.Seconds()),
		Urgency:         wp.UrgencyNormal,
		RecordSize:      recordSize(len(payload)),
	})
	if err != nil {
		return fmt.Errorf("webpush.send.failed: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return push.ErrGone
	case res.StatusCode >= http.StatusOK && res.StatusCode < http.StatusMultipleChoices:
		return nil
	default:
		return fmt.Errorf("webpush.send.rejected: status %d", res.StatusCode)
	}
}

// recordSize picks the smallest 256-byte bucket that fits the payload, so
// messages are padded (hiding their exact length) without always growing
// to the 4 KB maximum. The library pads to the record size minus the
// 16-byte tag and 86-byte header, and needs one delimiter byte.
func recordSize(n int) uint32 {
	need := n + 1 + 16 + 86
	size := (need/256 + 1) * 256
	if size > int(wp.MaxRecordSize) {
		size = int(wp.MaxRecordSize)
	}
	return uint32(size) //nolint:gosec // between 256 and MaxRecordSize
}

// guarded returns a client that does not follow redirects and refuses to
// connect to addresses a user-supplied endpoint should never reach (cloud
// metadata, and unless allowPrivate, loopback and private networks).
// Connections to a configured HTTP(S) proxy are allowed; the proxy connects
// to the endpoint itself, so before handing a request to it the endpoint's
// host is checked as far as this server can resolve it.
func guarded(allowPrivate bool) *http.Client {
	proxies := proxyAddrs()
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip, err := netip.ParseAddr(host); err != nil || push.Blocked(ip, allowPrivate) {
			return fmt.Errorf("webpush.endpoint.blocked: %s", host)
		}
		return nil
	}}
	direct := &net.Dialer{Timeout: 10 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxy := httpproxy.FromEnvironment().ProxyFunc()
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		via, err := proxy(req.URL)
		if err != nil || via == nil {
			return via, err
		}
		if err := screen(req.Context(), req.URL.Hostname(), allowPrivate, net.DefaultResolver.LookupNetIP); err != nil {
			return nil, err
		}
		return via, nil
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if proxies[addr] {
			return direct.DialContext(ctx, network, addr)
		}
		return dialer.DialContext(ctx, network, addr)
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// screen refuses a host that is, or resolves to, an address Blocked
// rejects. Any blocked address is enough, since the proxy may pick it. A
// host this server cannot resolve is left to the proxy: networks that only
// reach the internet through a proxy often have no public DNS.
func screen(ctx context.Context, host string, allowPrivate bool, lookup func(context.Context, string, string) ([]netip.Addr, error)) error {
	addrs := []netip.Addr{}
	if ip, err := netip.ParseAddr(host); err == nil {
		addrs = append(addrs, ip)
	} else if found, err := lookup(ctx, "ip", host); err == nil {
		addrs = found
	}
	for _, ip := range addrs {
		if push.Blocked(ip, allowPrivate) {
			return fmt.Errorf("webpush.endpoint.blocked: %s", host)
		}
	}
	return nil
}

// proxyAddrs lists host:port of the proxies in the environment, which the
// transport dials instead of the endpoint.
func proxyAddrs() map[string]bool {
	out := map[string]bool{}
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		v := os.Getenv(key)
		if v == "" {
			continue
		}
		if !strings.Contains(v, "://") {
			v = "http://" + v
		}
		u, err := url.Parse(v)
		if err != nil || u.Hostname() == "" {
			continue
		}
		port := u.Port()
		if port == "" {
			port = map[string]string{"https": "443", "socks5": "1080"}[u.Scheme]
			if port == "" {
				port = "80"
			}
		}
		out[net.JoinHostPort(u.Hostname(), port)] = true
	}
	return out
}
