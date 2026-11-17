// Package session models a signed-in panel session.
package session

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// Session is one panel sign-in. The cookie carries a random id; only its
// hash is stored, so a database leak does not yield usable cookies.
type Session struct {
	IDHash    string
	Email     string
	Name      string
	CreatedAt time.Time
	ExpiresAt time.Time
	IP        string
	UserAgent string
}

// New creates a session and returns the raw id for the cookie. The error
// return exists for interface symmetry with other domain constructors; it
// is always nil, since crypto/rand.Read never fails on supported platforms.
func New(email, name, ip, userAgent string, ttl time.Duration, now time.Time) (*Session, string, error) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw) // crypto/rand.Read never fails on supported platforms (Go 1.24+).
	id := base64.RawURLEncoding.EncodeToString(raw)
	return &Session{
		IDHash: HashID(id), Email: email, Name: name, CreatedAt: now, ExpiresAt: now.Add(ttl),
		IP: ip, UserAgent: userAgent,
	}, id, nil
}

// HashID maps a cookie value to the stored key.
func HashID(id string) string { return shared.HashSecret(id) }

// Expired reports whether the session is past its lifetime at now.
func (s *Session) Expired(now time.Time) bool { return !now.Before(s.ExpiresAt) }

// Display is the name to show for the user.
func (s *Session) Display() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Email
}
