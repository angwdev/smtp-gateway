package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// IPLimiter decides whether an address may proceed.
type IPLimiter interface {
	Allow(ip string) bool
}

// RateLimit refuses requests above the per-address limit.
func RateLimit(l IPLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP()) {
			Fail(c, shared.ErrRateLimited)
			return
		}
		c.Next()
	}
}
