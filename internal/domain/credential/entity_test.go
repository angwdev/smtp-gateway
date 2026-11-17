package credential

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

var t0 = time.Unix(1_800_000_000, 0)

func TestNew(t *testing.T) {
	c, secret, err := New("nextcloud", "files", []string{"@Example.com"}, 10, true, "admin@example.com", t0)
	require.NoError(t, err)
	assert.Len(t, secret, SecretLength)
	assert.Equal(t, shared.HashSecret(secret), c.SecretHash)
	assert.Equal(t, SenderPolicy{"@example.com"}, c.AllowedSenders)
	assert.Equal(t, t0, c.CreatedAt)
	assert.Equal(t, t0, c.UpdatedAt)
	assert.True(t, c.Verify(secret))
	assert.False(t, c.Verify(secret+"x"))
}

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name, username, description string
		rate                        int
		field                       string
	}{
		{"empty username", "", "", 0, "username"},
		{"bad character", "next cloud", "", 0, "username"},
		{"65 characters", string(make([]byte, 65)), "", 0, "username"},
		{"long description", "ok", string(make([]byte, 201)), 0, "description"},
		{"negative rate", "ok", "", -1, "rate_per_hour"},
		{"huge rate", "ok", "", 1_000_001, "rate_per_hour"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := New(tt.username, tt.description, nil, tt.rate, true, "", t0)
			var ve *shared.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, tt.field, ve.Field)
		})
	}
}

func TestDisabledCredentialNeverVerifies(t *testing.T) {
	c, secret, err := New("a", "", nil, 0, false, "", t0)
	require.NoError(t, err)
	assert.False(t, c.Verify(secret))
}

func TestApplyAndRotate(t *testing.T) {
	c, secret, err := New("a", "", nil, 0, true, "", t0)
	require.NoError(t, err)
	later := t0.Add(time.Hour)

	desc, rate, off := "new", 5, false
	senders := []string{"x@y.z"}
	require.NoError(t, c.Apply(&desc, &senders, &rate, &off, later))
	assert.Equal(t, "new", c.Description)
	assert.Equal(t, SenderPolicy{"x@y.z"}, c.AllowedSenders)
	assert.Equal(t, 5, c.RatePerHour)
	assert.False(t, c.Enabled)
	assert.Equal(t, later, c.UpdatedAt)

	require.NoError(t, c.Apply(nil, nil, nil, nil, later), "nil fields change nothing")
	assert.Equal(t, "new", c.Description)

	bad := -1
	require.Error(t, c.Apply(nil, nil, &bad, nil, later))
	assert.Equal(t, 5, c.RatePerHour, "a failed Apply must not half-apply")

	next, err := c.Rotate(later)
	require.NoError(t, err)
	assert.NotEqual(t, secret, next)
	assert.Equal(t, shared.HashSecret(next), c.SecretHash)
}
