package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
)

// Principal kinds.
const (
	KindUser  = "user"
	KindToken = "token"
)

// Principal is who is making the request.
type Principal struct {
	Kind      string
	Name      string // display name of the user, or the token's name
	Email     string // users only
	Scope     apitoken.Scope
	SessionID string    // raw cookie value, users only (needed to sign out)
	ExpiresAt time.Time // users only
}

// PrincipalOf returns the authenticated principal, or nil.
func PrincipalOf(c *gin.Context) *Principal {
	p, _ := c.Get(keyPrincipal)
	principal, _ := p.(*Principal)
	return principal
}
