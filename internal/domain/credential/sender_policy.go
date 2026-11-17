package credential

import (
	"strings"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

const maxSenderPatterns = 50

// SenderPolicy lists the addresses a credential may send as: "*" (any),
// "user@example.com" (exact), "@example.com" or "*@example.com" (a domain)
// and "@*.example.com" (any subdomain). Empty allows everything.
type SenderPolicy []string

// NewSenderPolicy normalises and validates patterns.
func NewSenderPolicy(patterns []string) (SenderPolicy, error) {
	if len(patterns) > maxSenderPatterns {
		return nil, shared.Invalid("allowed_senders", "List at most 50 senders.")
	}
	var out SenderPolicy
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if p != "*" && !strings.Contains(p, "@") {
			return nil, shared.Invalid("allowed_senders", `"`+p+`" is not a sender pattern. Use user@domain, @domain, @*.domain or *.`)
		}
		if len(p) > 255 {
			return nil, shared.Invalid("allowed_senders", "A sender pattern is too long.")
		}
		out = append(out, p)
	}
	return out, nil
}

// Allows reports whether addr matches the policy.
func (p SenderPolicy) Allows(addr string) bool {
	if len(p) == 0 {
		return true
	}
	addr = strings.ToLower(strings.TrimSpace(addr))
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 || at == len(addr)-1 {
		return false
	}
	domain := addr[at+1:]
	for _, pattern := range p {
		pattern = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(pattern)), "*")
		switch {
		case pattern == "": // the pattern was "*"
			return true
		case strings.HasPrefix(pattern, "@*."):
			if strings.HasSuffix(domain, pattern[2:]) {
				return true
			}
		case strings.HasPrefix(pattern, "@"):
			if domain == pattern[1:] {
				return true
			}
		case pattern == addr:
			return true
		}
	}
	return false
}
