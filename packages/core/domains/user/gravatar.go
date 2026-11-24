package user

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// Picture is the Gravatar image URL for u, or "" when u has not opted in or
// has no email. d=404 makes Gravatar answer 404 for people without a
// picture, so apps fall back to initials instead of a generic image.
func (u *User) Picture(size int) string {
	if !u.Gravatar || u.Email == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(u.Email))))
	return "https://gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?s=" + strconv.Itoa(size) + "&d=404"
}
