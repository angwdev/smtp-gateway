package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
)

// MarkAudited tells AuditCalls that the handler's use case already wrote a
// specific audit event for this request.
func MarkAudited(c *gin.Context) { c.Set(keyAudited, true) }

// AuditCalls records every API-token request that did not produce a more
// specific audit event, so that each call made with a token leaves a trace.
// It must be registered after Authenticate (it reads the principal that
// Authenticate sets) and before any handler whose RequireScope/RequireSession
// guard might abort the request, so even rejected calls get one audit row.
func AuditCalls(auditor NoisyAuditor) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		p := PrincipalOf(c)
		if p == nil || p.Kind != KindToken || c.GetBool(keyAudited) {
			return
		}
		status := c.Writer.Status()
		if len(c.Errors) > 0 { // ErrorHandler has not written the response yet
			status, _ = Translate(c.Errors.Last().Err)
		}
		result := audit.ResultOK
		switch {
		case status >= http.StatusInternalServerError:
			result = audit.ResultError
		case status >= http.StatusBadRequest:
			result = audit.ResultDenied
		}
		auditor.RecordNoisy(c.Request.Context(), appaudit.Entry{
			Action: "api.call", Result: result, Detail: c.Request.Method + " " + c.Request.URL.RequestURI()})
	}
}
