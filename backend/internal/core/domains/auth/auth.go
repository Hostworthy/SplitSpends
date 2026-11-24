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
	"strings"
	"time"

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

// Challenge is a pending one-time login code.
type Challenge struct {
	ID         uuid.UUID
	Email      string
	CodeHash   string
	Attempts   int
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// Usable reports whether the challenge can still be verified.
func (c *Challenge) Usable(now time.Time, maxAttempts int) bool {
	return c.ConsumedAt == nil && now.Before(c.ExpiresAt) && c.Attempts < maxAttempts
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
	// SignupDomain lets in invited users and emails from allowed domains.
	SignupDomain SignupMode = "domain"
)

// Policy configures login behaviour.
type Policy struct {
	Secret         []byte
	Mode           SignupMode
	AllowedDomains []string
	CodeTTL        time.Duration
	SessionTTL     time.Duration
	MaxAttempts    int
	MaxCodesHour   int
}

// DefaultPolicy returns sensible defaults; Secret must still be set.
func DefaultPolicy() Policy {
	return Policy{
		Mode:         SignupInvite,
		CodeTTL:      10 * time.Minute,
		SessionTTL:   90 * 24 * time.Hour,
		MaxAttempts:  5,
		MaxCodesHour: 5,
	}
}

func (p Policy) domainAllowed(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, d := range p.AllowedDomains {
		if strings.EqualFold(strings.TrimSpace(d), domain) {
			return true
		}
	}
	return false
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
