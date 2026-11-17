package apitoken

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

var t0 = time.Unix(1_800_000_000, 0)

func TestNew(t *testing.T) {
	tok, secret, err := New("grafana", ScopeRead, nil, "ana@example.com", t0)
	require.NoError(t, err)
	assert.Len(t, secret, len(Prefix)+SecretLength)
	assert.True(t, len(secret) > len(Prefix) && secret[:len(Prefix)] == Prefix)
	assert.Equal(t, shared.HashSecret(secret), tok.Hash)
	assert.Equal(t, secret[len(secret)-hintLength:], tok.Hint)
	assert.Equal(t, ScopeRead, tok.Scope)
	assert.Equal(t, t0, tok.CreatedAt)
	assert.Equal(t, "ana@example.com", tok.CreatedBy)
}

func TestNewValidation(t *testing.T) {
	future := t0.Add(time.Hour)
	past := t0.Add(-time.Hour)
	tests := []struct {
		name      string
		tokenName string
		scope     Scope
		expiresAt *time.Time
		field     string
	}{
		{"empty name", "", ScopeRead, nil, "name"},
		{"invalid character", "bad!name", ScopeRead, nil, "name"},
		{"65 characters", strings.Repeat("a", 65), ScopeRead, nil, "name"},
		{"bad scope", "ok", Scope("admin"), nil, "scope"},
		{"expiry in the past", "ok", ScopeRead, &past, "expires_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := New(tt.tokenName, tt.scope, tt.expiresAt, "", t0)
			var ve *shared.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, tt.field, ve.Field)
		})
	}
	_, _, err := New("ok", ScopeWrite, &future, "", t0)
	require.NoError(t, err)
}

func TestScopeAllows(t *testing.T) {
	tests := []struct {
		have, need Scope
		want       bool
	}{
		{ScopeWrite, ScopeWrite, true},
		{ScopeWrite, ScopeRead, true},
		{ScopeRead, ScopeRead, true},
		{ScopeRead, ScopeWrite, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.have.Allows(tt.need), "%s.Allows(%s)", tt.have, tt.need)
	}
}

func TestActive(t *testing.T) {
	revokedAt := t0
	future := t0.Add(time.Hour)
	past := t0.Add(-time.Hour)
	tests := []struct {
		name string
		tok  Token
		want bool
	}{
		{"revoked", Token{RevokedAt: &revokedAt}, false},
		{"expired", Token{ExpiresAt: &past}, false},
		{"future expiry", Token{ExpiresAt: &future}, true},
		{"no expiry", Token{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tok.Active(t0))
		})
	}
}
