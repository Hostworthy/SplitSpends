// Package push owns push subscriptions: the browsers and phones a user
// registered for Web Push (RFC 8030), and the rules for delivering to them.
package push

import (
	"encoding/base64"
	"net/netip"
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
	// SessionID is the session or API token that registered the device.
	// The device is removed with it, so it stops getting the account's
	// notifications once that sign-in ends. Nil for devices registered
	// before devices were tied to sessions.
	SessionID uuid.UUID
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

// New validates in and builds a subscription for userID, registered by
// sessionID.
func New(userID, sessionID uuid.UUID, in Input, now time.Time) (*Subscription, error) {
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
	return &Subscription{ID: uuid.New(), UserID: userID, SessionID: sessionID, Endpoint: endpoint, P256dh: p256dh, Auth: auth,
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
	ip, err := netip.ParseAddr(u.Hostname())
	return err != nil || !Blocked(ip, allowPrivate)
}

// decode accepts base64url (what browsers and UnifiedPush send) and, to be
// lenient, standard base64.
func decode(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// Blocked reports whether a push endpoint may not reach ip: reserved
// ranges and cloud metadata services always, and unless allowPrivate (for a
// UnifiedPush server on the local network), loopback and private networks.
// IPv6 addresses that carry an IPv4 address (mapped, NAT64, 6to4) are judged
// by that IPv4 address.
func Blocked(ip netip.Addr, allowPrivate bool) bool {
	// A prefix never contains an address with a zone, so drop it.
	ip = ip.Unmap().WithZone("")
	if !ip.IsValid() {
		return true
	}
	if v4, ok := embedded(ip); ok {
		return Blocked(v4, allowPrivate)
	}
	for _, p := range reserved {
		if p.Contains(ip) {
			return true
		}
	}
	if allowPrivate {
		return false
	}
	for _, p := range private {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// embedded returns the IPv4 address a NAT64 (64:ff9b::/96) or 6to4
// (2002::/16) address forwards to.
func embedded(ip netip.Addr) (netip.Addr, bool) {
	b := ip.As16()
	switch {
	case nat64.Contains(ip):
		return netip.AddrFrom4([4]byte(b[12:16])), true
	case sixToFour.Contains(ip):
		return netip.AddrFrom4([4]byte(b[2:6])), true
	}
	return netip.Addr{}, false
}

var (
	nat64     = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour = netip.MustParsePrefix("2002::/16")

	// reserved is never a push service: this network, link-local (where
	// most clouds serve instance metadata), IETF protocol assignments
	// (Oracle Cloud's legacy metadata), benchmarking, multicast, the
	// reserved block and broadcast, local-use NAT64, and the metadata
	// services of AWS (IPv6) and Alibaba Cloud, which sit inside private
	// ranges and so must stay blocked when those are allowed.
	reserved = prefixes("0.0.0.0/8", "169.254.0.0/16", "192.0.0.0/24", "198.18.0.0/15", "224.0.0.0/3",
		"::/128", "fe80::/10", "ff00::/8", "64:ff9b:1::/48", "fd00:ec2::254/128", "100.100.100.200/32")

	// private is loopback, private networks and carrier-grade NAT.
	private = prefixes("127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10",
		"::1/128", "fc00::/7")
)

func prefixes(list ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(list))
	for i, p := range list {
		out[i] = netip.MustParsePrefix(p)
	}
	return out
}
