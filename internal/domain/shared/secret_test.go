package shared

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomString(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		s, err := RandomString(32, SecretAlphabet)
		require.NoError(t, err)
		require.Len(t, s, 32)
		for _, r := range s {
			require.True(t, strings.ContainsRune(SecretAlphabet, r), "unexpected %q", r)
		}
		require.False(t, seen[s], "repeated secret")
		seen[s] = true
	}
	_, err := RandomString(0, SecretAlphabet)
	assert.Error(t, err)
	_, err = RandomString(8, "")
	assert.Error(t, err)
}

// The hash must stay hex SHA-256: v1 passwords in the live database were
// stored this way and have to keep working.
func TestHashSecretIsHexSHA256(t *testing.T) {
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", HashSecret("abc"))
	assert.True(t, SecretMatches(HashSecret("abc"), "abc"))
	assert.False(t, SecretMatches(HashSecret("abc"), "abd"))
}
