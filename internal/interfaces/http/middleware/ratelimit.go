package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// IPLimiter decides whether an address may proceed.
type IPLimiter interface {
	Allow(ip string) bool
}

// RateLimit refuses requests above the per-address limit. retryAfterSeconds
// is advertised to the client as the Retry-After hint for this limiter's
// window (e.g. 60 for a 10-per-minute limiter); pass the smallest sensible
// wait (1) when the limiter's own window is not known at the call site.
func RateLimit(l IPLimiter, retryAfterSeconds int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP()) {
			Fail(c, shared.RateLimited(retryAfterSeconds))
			return
		}
		c.Next()
	}
}
