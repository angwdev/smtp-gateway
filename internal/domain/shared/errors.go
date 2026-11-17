// Package shared holds the error vocabulary and secret helpers used by every
// domain package. It imports only the standard library.
package shared

import "errors"

// Sentinel errors. Wrap them with fmt.Errorf("...: %w", err); callers test
// with errors.Is.
var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidLogin = errors.New("invalid credentials")
	ErrRateLimited  = errors.New("rate limited")
)

// ValidationError says which input field is wrong and why, in words fit to
// show to the user.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return "invalid " + e.Field + ": " + e.Message }

// Invalid builds a ValidationError.
func Invalid(field, message string) error { return &ValidationError{Field: field, Message: message} }

// RateLimitedError is ErrRateLimited plus how long the client should wait
// before trying again, when the limiter that rejected the request has a
// known, fixed window (e.g. the 10-per-minute login limiter). Callers that
// cannot cheaply learn their window keep returning the bare ErrRateLimited
// sentinel; the HTTP layer then falls back to a conservative default.
type RateLimitedError struct {
	RetrySeconds int
}

func (e *RateLimitedError) Error() string { return "rate limited" }

// Unwrap lets errors.Is(err, ErrRateLimited) keep working for callers that
// only care about the sentinel, not the retry hint.
func (e *RateLimitedError) Unwrap() error { return ErrRateLimited }

// RateLimited builds a RateLimitedError carrying the limiter's window.
func RateLimited(retrySeconds int) error { return &RateLimitedError{RetrySeconds: retrySeconds} }
