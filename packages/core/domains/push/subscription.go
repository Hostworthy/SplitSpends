// Package push owns push subscriptions: the browsers and phones a user
// registered for Web Push (RFC 8030), and the rules for delivering to them.
package push

import (
	"encoding/base64"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// Platform is how a subscription reaches its device. Both speak standard
// Web Push; the value tells people which device is which.
type Platform string

const (
	// PlatformWeb is a browser's own push service.
	PlatformWeb Platform = "web"
	// PlatformUnifiedPush is an Android phone through a UnifiedPush
	// distributor such as ntfy.
	PlatformUnifiedPush Platform = "unifiedpush"
)

// Subscription is one device that receives push notifications.
type Subscription struct {
	ID     uuid.UUID
	UserID uuid.UUID
	// Endpoint is the push service URL. It is a capability: whoever knows it
	// can address the device, so it is only shown to its owner.
	Endpoint string
	// P256dh and Auth are the device's message encryption keys (RFC 8291),
	// base64url encoded.
	P256dh    string
	Auth      string
	Platform  Platform
	Label     string
	CreatedAt time.Time
	UpdatedAt time.Time
	// LastSuccessAt is the last time the push service accepted a message.
	LastSuccessAt *time.Time
	// Failures counts deliveries that failed since the last success.
	Failures int
}

// Input registers or refreshes a device.
type Input struct {
	Endpoint string
	P256dh   string
	Auth     string
	Platform string
	Label    string
}

const (
	maxEndpoint = 2048
	maxLabel    = 80
)

// New validates in and builds a subscription for userID.
func New(userID uuid.UUID, in Input, now time.Time) (*Subscription, error) {
	endpoint := strings.TrimSpace(in.Endpoint)
	u, err := url.Parse(endpoint)
	if endpoint == "" || err != nil || u.Scheme != "https" || u.Host == "" || len(endpoint) > maxEndpoint {
		return nil, errs.Validation("endpoint", "The push endpoint must be an https URL.")
	}
	p256dh, auth := strings.TrimRight(strings.TrimSpace(in.P256dh), "="), strings.TrimRight(strings.TrimSpace(in.Auth), "=")
	if key, err := decode(p256dh); err != nil || len(key) != 65 || key[0] != 4 {
		return nil, errs.Validation("keys.p256dh", "The p256dh key must be an uncompressed P-256 public key.")
	}
	if secret, err := decode(auth); err != nil || len(secret) != 16 {
		return nil, errs.Validation("keys.auth", "The auth secret must be 16 bytes.")
	}
	platform := Platform(strings.TrimSpace(in.Platform))
	switch platform {
	case "":
		platform = PlatformWeb
	case PlatformWeb, PlatformUnifiedPush:
	default:
		return nil, errs.Validation("platform", "Platform must be web or unifiedpush.")
	}
	label := strings.Join(strings.Fields(in.Label), " ")
	if utf8.RuneCountInString(label) > maxLabel {
		label = string([]rune(label)[:maxLabel])
	}
	return &Subscription{ID: uuid.New(), UserID: userID, Endpoint: endpoint, P256dh: p256dh, Auth: auth,
		Platform: platform, Label: label, CreatedAt: now, UpdatedAt: now}, nil
}

// Reachable reports whether the endpoint may be contacted under the
// address policy. Only addresses written into the URL are checked here;
// the sender checks every address a host name resolves to.
func (s *Subscription) Reachable(allowPrivate bool) bool {
	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip == nil || !Blocked(ip, allowPrivate)
}

// decode accepts base64url (what browsers and UnifiedPush send) and, to be
// lenient, standard base64.
func decode(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// Blocked reports whether a push endpoint may not reach ip: cloud metadata
// and other link-local addresses always, and unless allowPrivate (for a
// UnifiedPush server on the local network), loopback and private networks.
func Blocked(ip net.IP, allowPrivate bool) bool {
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if allowPrivate {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || cgnat.Contains(ip)
}

var cgnat = &net.IPNet{IP: net.IP{100, 64, 0, 0}, Mask: net.CIDRMask(10, 32)}
