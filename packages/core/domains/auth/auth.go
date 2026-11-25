// Package auth owns passwordless email login, sessions and API tokens.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// Kind distinguishes interactive sessions from personal API tokens.
type Kind string

const (
	KindSession Kind = "session"
	KindToken   Kind = "token"
)

// TokenPrefix marks personal API tokens so they are easy to spot in configs
// and secret scanners.
const TokenPrefix = "ss_"

// MaxTokens is how many API tokens one account may hold at a time.
const MaxTokens = 20

// Challenge is a pending one-time login code.
type Challenge struct {
	ID    uuid.UUID
	Email string
	// CodeHash is empty when no code was sent because the email may not
	// sign in; the request still counts toward the hourly limits.
	CodeHash   string
	Attempts   int
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
	// Client is a keyed hash of the network the code was requested from.
	Client string
	// Invite is the storage hash of the invite link the code was requested
	// with, or empty.
	Invite string
}

// Usable reports whether the challenge can still be verified.
func (c *Challenge) Usable(now time.Time, maxAttempts int) bool {
	return c.CodeHash != "" && c.ConsumedAt == nil && now.Before(c.ExpiresAt) && c.Attempts < maxAttempts
}

// Session is an authenticated credential, interactive or API token.
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Kind       Kind
	Name       string
	TokenHash  string
	UserAgent  string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// Expired reports whether the session is past its expiry.
func (s *Session) Expired(now time.Time) bool {
	return s.ExpiresAt != nil && !now.Before(*s.ExpiresAt)
}

// Principal is the authenticated caller.
type Principal struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Kind      Kind
}

// SignupMode controls who may create an account.
type SignupMode string

const (
	// SignupOpen lets anyone with an email sign in.
	SignupOpen SignupMode = "open"
	// SignupInvite lets in the first user and anyone an existing user added.
	SignupInvite SignupMode = "invite"
	// SignupDomain is the older name for invite mode plus an allowed list of
	// domains (SIGNUP_ALLOWED_DOMAINS); it is read as SignupInvite.
	SignupDomain SignupMode = "domain"
	// SignupApproval lets anyone ask to join; an administrator or manager
	// approves them before their first sign-in.
	SignupApproval SignupMode = "approval"
	// SignupClosed allows no new accounts at all. Existing accounts still
	// sign in.
	SignupClosed SignupMode = "closed"
)

// Modes lists the sign-up modes an administrator can choose.
var Modes = []SignupMode{SignupOpen, SignupInvite, SignupApproval, SignupClosed}

// Signup is who may create an account right now. Allowed holds email
// addresses and whole domains written as "@example.com"; in invite and
// approval modes they sign up without waiting.
type Signup struct {
	Mode    SignupMode
	Allowed []string
}

// MaxAllowed bounds the allowed list.
const MaxAllowed = 500

// Normalize checks the mode an administrator chose and cleans the allowed
// list: addresses are lowercased, "example.com" becomes "@example.com",
// duplicates go.
func Normalize(mode string, allowed []string) (Signup, error) {
	s := Signup{Mode: SignupMode(strings.ToLower(strings.TrimSpace(mode)))}
	if !slices.Contains(Modes, s.Mode) {
		return s, errs.Validation("mode", "Choose open, invite, approval or closed.")
	}
	if len(allowed) > MaxAllowed {
		return s, errs.Validation("allowed", fmt.Sprintf("The allowed list is limited to %d entries.", MaxAllowed))
	}
	seen := map[string]bool{}
	s.Allowed = []string{}
	for _, raw := range allowed {
		entry := strings.ToLower(strings.TrimSpace(raw))
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "@") || !strings.Contains(entry, "@") {
			domain, ok := Domain(entry)
			if !ok {
				return s, errs.Validation("allowed", "“"+raw+"” is not an email address or a domain like @example.com.")
			}
			entry = domain
		} else {
			email, err := user.NormalizeEmail(entry)
			if err != nil {
				return s, errs.Validation("allowed", "“"+raw+"” is not an email address or a domain like @example.com.")
			}
			entry = email
		}
		if !seen[entry] {
			seen[entry] = true
			s.Allowed = append(s.Allowed, entry)
		}
	}
	return s, nil
}

// Domain returns the allowed-list entry for a domain written with or
// without "@", such as "@example.com", and whether raw is a domain at all.
func Domain(raw string) (string, bool) {
	d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "@")
	if len(d) < 3 || len(d) > 253 || !strings.Contains(d, ".") || strings.HasPrefix(d, ".") || strings.HasSuffix(d, ".") {
		return "", false
	}
	for _, r := range d {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
		default:
			return "", false
		}
	}
	return "@" + d, true
}

// Allows reports whether email is on the allowed list.
func (s Signup) Allows(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	for _, entry := range s.Allowed {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == email || (at >= 0 && strings.HasPrefix(entry, "@") && entry == email[at:]) {
			return true
		}
	}
	return false
}

// Policy configures login behaviour.
type Policy struct {
	Secret         []byte
	Mode           SignupMode
	AllowedDomains []string
	CodeTTL        time.Duration
	SessionTTL     time.Duration
	MaxAttempts    int
	// MaxCodesHour is how many codes one network may request for an email
	// in an hour, and MaxCodesEmail how many all networks may together.
	// Someone who knows an address can use up only their own network's
	// share, so its owner can still get a code; the total keeps the number
	// of guesses per address bounded.
	MaxCodesHour  int
	MaxCodesEmail int
	// AdminEmail, when set, is the only address that may sign in while no
	// account is active, so nobody else can claim a new server first.
	AdminEmail string
}

// DefaultPolicy returns sensible defaults; Secret must still be set.
func DefaultPolicy() Policy {
	return Policy{
		Mode:          SignupInvite,
		CodeTTL:       10 * time.Minute,
		SessionTTL:    90 * 24 * time.Hour,
		MaxAttempts:   5,
		MaxCodesHour:  5,
		MaxCodesEmail: 10,
	}
}

// Signup is the starting configuration from the environment, used until an
// administrator changes it in the app.
func (p Policy) Signup() Signup {
	s := Signup{Mode: p.Mode}
	if s.Mode == SignupDomain {
		s.Mode = SignupInvite
	}
	for _, d := range p.AllowedDomains {
		// Loading the configuration refuses entries that are not domains,
		// so skipping them here never drops one silently.
		if entry, ok := Domain(d); ok && !slices.Contains(s.Allowed, entry) {
			s.Allowed = append(s.Allowed, entry)
		}
	}
	return s
}

// newCode returns a uniformly random 6 digit code.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// hashCode binds the code to the email and challenge with an HMAC so a leaked
// database row cannot be brute-forced offline without the server secret.
func hashCode(secret []byte, id uuid.UUID, email, code string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(id.String() + "|" + email + "|" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

// hashClient keys a client address with the server secret, so stored codes
// do not reveal where people sign in from.
func hashClient(secret []byte, client string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("client|" + client))
	return hex.EncodeToString(mac.Sum(nil))
}

// newToken returns a random bearer token and its storage hash.
func newToken(prefix string) (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token = prefix + base64.RawURLEncoding.EncodeToString(buf)
	return token, HashToken(token), nil
}

// HashToken returns the storage hash of a bearer token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
