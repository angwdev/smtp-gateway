package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	appmsg "github.com/angwdev/smtp-gateway/internal/application/message"
	"github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

// MessageHandler serves /api/v1/messages.
type MessageHandler struct{ uc *appmsg.Handler }

// NewMessageHandler returns the handler.
func NewMessageHandler(uc *appmsg.Handler) *MessageHandler { return &MessageHandler{uc: uc} }

// List handles GET /messages.
func (h *MessageHandler) List(c *gin.Context) {
	credentialID, err := queryInt(c, "credential_id")
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	status := message.Status(c.Query("status"))
	if status != "" && !status.Valid() {
		middleware.Fail(c, shared.Invalid("status", "Use sent, failed or rejected."))
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

	list, next, err := h.uc.List(c.Request.Context(), message.Filter{
		CredentialID: credentialID, Status: status, Query: c.Query("q"), From: from, To: to,
		BeforeID: before, Limit: int(limit),
	})
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	items := make([]MessageResponse, 0, len(list))
	for _, m := range list {
		items = append(items, fromMessage(m))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "next_before": next})
}

// Get handles GET /messages/:id.
func (h *MessageHandler) Get(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	d, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, fromMessageDetail(d))
}
