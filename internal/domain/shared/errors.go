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
