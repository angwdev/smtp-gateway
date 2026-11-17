package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/pkg/response"
)

// Recovery turns a panic in a handler into a logged 500 (skill rule
// error-panic-recover: recover only at the edge, never to hide errors).
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			log.Error("panic in handler", "panic", r, "request_id", RequestIDOf(c), "path", c.Request.URL.Path, "stack", string(debug.Stack()))
			if c.Writer.Written() {
				c.Abort()
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				response.ErrorResponse{Code: response.CodeInternal, Message: "An internal error occurred."})
		}()
		c.Next()
	}
}
