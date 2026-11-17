// Package apitoken holds the use cases for bearer tokens.
package apitoken

import (
	"time"

	domain "github.com/angwdev/smtp-gateway/internal/domain/apitoken"
)

// CreateCommand creates an API token.
type CreateCommand struct {
	Name      string
	Scope     domain.Scope
	ExpiresAt *time.Time
	CreatedBy string
}
