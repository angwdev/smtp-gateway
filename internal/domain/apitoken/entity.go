// Package apitoken models bearer tokens for the HTTP API.
package apitoken

import (
	"regexp"
	"strings"
	"time"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// Scope is what a token may do.
type Scope string

// Scopes. Write includes read.
const (
	ScopeRead  Scope = "read"
	ScopeWrite Scope = "write"
)

// Allows reports whether a principal holding s may do something that needs need.
func (s Scope) Allows(need Scope) bool {
	return s == ScopeWrite || (s == ScopeRead && need == ScopeRead)
}

// Token format: Prefix followed by SecretLength random characters.
const (
	Prefix       = "sgw_"
	SecretLength = 40
	hintLength   = 4
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)

// Token is one API token. Only the hash of the secret is kept.
type Token struct {
	ID         int64
	Name       string
	Hash       string
	Hint       string // last characters of the secret, to tell tokens apart
	Scope      Scope
	CreatedAt  time.Time
	CreatedBy  string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	LastUsedIP string
	RevokedAt  *time.Time
}

// New validates the input and returns the token with its plaintext secret,
// which is shown once and never stored.
func New(name string, scope Scope, expiresAt *time.Time, createdBy string, now time.Time) (*Token, string, error) {
	name = strings.TrimSpace(name)
	if !nameRe.MatchString(name) {
		return nil, "", shared.Invalid("name", "Use 1-64 letters, digits, spaces or . _ -")
	}
	if scope != ScopeRead && scope != ScopeWrite {
		return nil, "", shared.Invalid("scope", "Choose read or write.")
	}
	if expiresAt != nil && !expiresAt.After(now) {
		return nil, "", shared.Invalid("expires_at", "Pick a date in the future.")
	}
	random, err := shared.RandomString(SecretLength, shared.SecretAlphabet)
	if err != nil {
		return nil, "", err
	}
	secret := Prefix + random
	return &Token{
		Name: name, Hash: shared.HashSecret(secret), Hint: random[SecretLength-hintLength:],
		Scope: scope, CreatedAt: now, CreatedBy: createdBy, ExpiresAt: expiresAt,
	}, secret, nil
}

// Active reports whether the token may be used at now.
func (t *Token) Active(now time.Time) bool {
	return t.RevokedAt == nil && (t.ExpiresAt == nil || now.Before(*t.ExpiresAt))
}
