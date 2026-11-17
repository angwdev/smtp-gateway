// Package credential models an SMTP login and the rules that guard it.
package credential

import (
	"regexp"
	"time"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// Limits.
const (
	SecretLength   = 32
	MaxDescription = 200
	MaxRatePerHour = 1_000_000
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9._+@-]{1,64}$`)

// Credential is one SMTP login.
type Credential struct {
	ID             int64
	Username       string
	Description    string
	AllowedSenders SenderPolicy // empty = any sender
	RatePerHour    int          // 0 = unlimited
	Enabled        bool
	SecretHash     string
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      time.Time
	LastUsedAt     *time.Time
	LastUsedIP     string
	SentCount      int64
}

// New validates the input and returns a credential with a fresh secret. The
// plaintext secret is returned once and never stored.
func New(username, description string, senders []string, ratePerHour int, enabled bool, createdBy string, now time.Time) (*Credential, string, error) {
	if !usernameRe.MatchString(username) {
		return nil, "", shared.Invalid("username", "Use 1-64 letters, digits or . _ - + @")
	}
	c := &Credential{Username: username, Enabled: enabled, CreatedBy: createdBy, CreatedAt: now}
	if err := c.Apply(&description, &senders, &ratePerHour, nil, now); err != nil {
		return nil, "", err
	}
	secret, err := c.Rotate(now)
	if err != nil {
		return nil, "", err
	}
	return c, secret, nil
}

// Apply changes the editable fields. A nil argument leaves that field alone.
// Nothing is changed when any value is invalid.
func (c *Credential) Apply(description *string, senders *[]string, ratePerHour *int, enabled *bool, now time.Time) error {
	next := *c
	if description != nil {
		if len(*description) > MaxDescription {
			return shared.Invalid("description", "Keep it under 200 characters.")
		}
		next.Description = *description
	}
	if senders != nil {
		policy, err := NewSenderPolicy(*senders)
		if err != nil {
			return err
		}
		next.AllowedSenders = policy
	}
	if ratePerHour != nil {
		if *ratePerHour < 0 || *ratePerHour > MaxRatePerHour {
			return shared.Invalid("rate_per_hour", "Enter a whole number, or 0 for no limit.")
		}
		next.RatePerHour = *ratePerHour
	}
	if enabled != nil {
		next.Enabled = *enabled
	}
	next.UpdatedAt = now
	*c = next
	return nil
}

// Rotate replaces the secret and returns the new plaintext.
func (c *Credential) Rotate(now time.Time) (string, error) {
	secret, err := shared.RandomString(SecretLength, shared.SecretAlphabet)
	if err != nil {
		return "", err
	}
	c.SecretHash = shared.HashSecret(secret)
	c.UpdatedAt = now
	return secret, nil
}

// Verify reports whether password is right and the credential may be used.
// The comparison runs even for a disabled credential so timing does not
// reveal its state.
func (c *Credential) Verify(password string) bool {
	ok := shared.SecretMatches(c.SecretHash, password)
	return ok && c.Enabled
}
