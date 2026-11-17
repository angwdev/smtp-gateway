package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger writes one structured line per request. The query string is left
// out: it carries search terms such as e-mail addresses.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		level := slog.LevelInfo
		if c.Request.URL.Path == "/healthz" {
			level = slog.LevelDebug
		}
		attrs := []any{
			"method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(), "ip", c.ClientIP(), "request_id", RequestIDOf(c),
		}
		if p := PrincipalOf(c); p != nil {
			attrs = append(attrs, "principal", p.Name, "kind", p.Kind)
		}
		log.Log(c.Request.Context(), level, "http", attrs...)
	}
}
