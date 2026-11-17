package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

// AuditHandler serves /api/v1/audit.
type AuditHandler struct{ uc *appaudit.Queries }

// NewAuditHandler returns the handler.
func NewAuditHandler(uc *appaudit.Queries) *AuditHandler { return &AuditHandler{uc: uc} }

// List handles GET /audit.
func (h *AuditHandler) List(c *gin.Context) {
	result := audit.Result(c.Query("result"))
	switch result {
	case "", audit.ResultOK, audit.ResultDenied, audit.ResultError:
	default:
		middleware.Fail(c, shared.Invalid("result", "Use ok, denied or error."))
		return
	}
	from, err := queryTime(c, "from")
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	to, err := queryTime(c, "to")
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	before, err := queryInt(c, "before")
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	limit, err := queryInt(c, "limit")
	if err != nil {
		middleware.Fail(c, err)
		return
	}

	list, next, err := h.uc.List(c.Request.Context(), audit.Filter{
		Actor: c.Query("actor"), Action: c.Query("action"), TargetType: c.Query("target_type"), Result: result,
		Query: c.Query("q"), From: from, To: to, BeforeID: before, Limit: int(limit),
	})
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	items := make([]AuditEventResponse, 0, len(list))
	for _, e := range list {
		items = append(items, fromAuditEvent(e))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "next_before": next})
}
