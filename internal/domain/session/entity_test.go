package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Unix(1_800_000_000, 0)

func TestNew(t *testing.T) {
	s, id, err := New("ana@example.com", "Ana", "203.0.113.9", "Firefox", time.Hour, t0)
	require.NoError(t, err)
	assert.Len(t, id, 43)
	assert.Equal(t, HashID(id), s.IDHash)
	assert.Equal(t, t0.Add(time.Hour), s.ExpiresAt)
	assert.Equal(t, "ana@example.com", s.Email)
	assert.Equal(t, "Ana", s.Name)
}

func TestExpired(t *testing.T) {
	s, _, err := New("a@b.c", "", "", "", time.Hour, t0)
	require.NoError(t, err)
	assert.True(t, s.Expired(s.ExpiresAt))
	assert.False(t, s.Expired(s.ExpiresAt.Add(-time.Second)))
}

func TestDisplay(t *testing.T) {
	assert.Equal(t, "Ana", (&Session{Email: "a@b.c", Name: "Ana"}).Display())
	assert.Equal(t, "a@b.c", (&Session{Email: "a@b.c"}).Display())
}
