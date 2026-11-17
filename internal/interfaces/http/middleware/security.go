package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/pkg/response"
)

// SecurityHeaders sets the response hardening headers. The CSP allows
// scripts only from this origin, and <style> elements only with the
// per-response nonce that the SPA hands to its CSS-in-JS cache (Material UI
// needs style-src-attr 'unsafe-inline' for dynamic inline styles).
func SecurityHeaders(https bool) gin.HandlerFunc { return securityHeaders(https, rand.Reader) }

// securityHeaders is SecurityHeaders with an injectable randomness source,
// so the nonce-generation failure path below can be exercised in a test
// without crypto/rand itself ever actually failing.
func securityHeaders(https bool, source io.Reader) gin.HandlerFunc {
	return func(c *gin.Context) {
		b := make([]byte, 16)
		if _, err := io.ReadFull(source, b); err != nil {
			// Fail closed. An unchecked read failure would hand out an
			// all-zero, guessable nonce: an attacker-controlled <style>
			// could then satisfy the CSP. Abort instead of serving the
			// page with a weak nonce. Answered here directly (not via
			// Fail+ErrorHandler) because SecurityHeaders runs on every
			// route via r.Use, including ones such as /healthz that have
			// no ErrorHandler in their chain.
			slog.Default().Error("generate CSP nonce", "err", err, "request_id", RequestIDOf(c))
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				response.ErrorResponse{Code: response.CodeInternal, Message: "An internal error occurred."})
			return
		}
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
