package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

// RequestID gives every request a random id, returned in X-Request-Id and
// attached to logs and audit events. A client-supplied id is ignored: it
// would let a caller forge log lines.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		b := make([]byte, 16)
		_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms (Go 1.24+)
		id := hex.EncodeToString(b)
		c.Set(keyRequestID, id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}

// RequestIDOf returns the id set by RequestID.
func RequestIDOf(c *gin.Context) string { return c.GetString(keyRequestID) }
