// Package response defines the JSON error body every API endpoint returns.
package response

// ErrorResponse is the unified error body.
type ErrorResponse struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error codes.
const (
	CodeValidation       = "VALIDATION_ERROR"
	CodeUnauthorized     = "UNAUTHORIZED"
	CodeForbidden        = "FORBIDDEN"
	CodeNotFound         = "NOT_FOUND"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeConflict         = "CONFLICT"
	CodeRateLimited      = "RATE_LIMITED"
	CodeInternal         = "INTERNAL_ERROR"
)
