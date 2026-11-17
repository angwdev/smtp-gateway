package shared

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
)

// SecretAlphabet leaves out look-alike characters (0 O 1 l I).
const SecretAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// RandomString returns n characters drawn uniformly from alphabet. Random
// bytes that would skew the distribution are rejected rather than folded
// with a modulo.
func RandomString(n int, alphabet string) (string, error) {
	if n <= 0 || alphabet == "" || len(alphabet) > 256 {
		return "", errors.New("random string: need n > 0 and an alphabet of 1 to 256 characters")
	}
	limit := 256 - 256%len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n+n/2+8)
	for len(out) < n {
		_, _ = rand.Read(buf) // crypto/rand.Read never fails on supported platforms (Go 1.24+).
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, alphabet[int(b)%len(alphabet)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}

// HashSecret returns the hex SHA-256 of secret. Secrets are always
// server-generated and high-entropy, so a fast hash is enough (as with API
// tokens) and keeps SMTP AUTH cheap.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// SecretMatches compares in constant time.
func SecretMatches(hash, candidate string) bool {
	return subtle.ConstantTimeCompare([]byte(hash), []byte(HashSecret(candidate))) == 1
}
