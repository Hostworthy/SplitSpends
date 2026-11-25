// Package invite owns shareable group invite links. A link carries a random
// token; only its SHA-256 hash is stored, like session tokens.
package invite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

// TTL is how long a new invite link stays valid.
const TTL = 14 * 24 * time.Hour

// MaxActive is how many usable links one group can have at a time. Every
// member sees them all in the group's settings.
const MaxActive = 20

// Path is where invite links open in the web app, followed by the token.
const Path = "/join/"

// Invite is a link that lets anyone who has it join a group. It can be used
// many times until it expires or is revoked.
type Invite struct {
	ID        uuid.UUID
	GroupID   uuid.UUID
	CreatedBy uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// New creates an invite for groupID and returns it with its token, which is
// only available now.
func New(groupID, createdBy uuid.UUID, now time.Time) (*Invite, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	return &Invite{ID: uuid.New(), GroupID: groupID, CreatedBy: createdBy, TokenHash: Hash(token),
		ExpiresAt: now.Add(TTL), CreatedAt: now}, token, nil
}

// Expired reports whether the invite can no longer be used.
func (i *Invite) Expired(now time.Time) bool { return !now.Before(i.ExpiresAt) }

// Hash returns the storage hash of an invite token.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Repository persists invites.
type Repository interface {
	Create(ctx context.Context, i *Invite) error
	// FindByHash returns nil when no invite matches.
	FindByHash(ctx context.Context, hash string) (*Invite, error)
	// Active lists a group's invites that expire after now, newest first.
	Active(ctx context.Context, groupID uuid.UUID, now time.Time) ([]*Invite, error)
	// Delete removes one invite of a group and reports whether it existed.
	Delete(ctx context.Context, groupID, id uuid.UUID) (bool, error)
	DeleteExpired(ctx context.Context, now time.Time) error
}
