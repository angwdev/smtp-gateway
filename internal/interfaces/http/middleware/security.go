package middleware

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders sets the response hardening headers. The CSP allows
// scripts only from this origin, and <style> elements only with the
// per-response nonce that the SPA hands to its CSS-in-JS cache (Material UI
// needs style-src-attr 'unsafe-inline' for dynamic inline styles).
func SecurityHeaders(https bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		nonce := base64.StdEncoding.EncodeToString(b)
		c.Set(keyNonce, nonce)

		h := c.Writer.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src-elem 'self' 'nonce-"+nonce+"'; "+
			"style-src-attr 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self'; "+
			"form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		if https {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		c.Next()
	}
}

// Nonce returns the CSP nonce of this response.
func Nonce(c *gin.Context) string { return c.GetString(keyNonce) }
