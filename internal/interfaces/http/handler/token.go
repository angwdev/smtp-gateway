package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apptoken "github.com/angwdev/smtp-gateway/internal/application/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

// TokenHandler serves /api/v1/tokens.
type TokenHandler struct{ uc *apptoken.Handler }

// NewTokenHandler returns the handler.
func NewTokenHandler(uc *apptoken.Handler) *TokenHandler { return &TokenHandler{uc: uc} }

// TokenCreateRequest is the body of POST /tokens.
type TokenCreateRequest struct {
	Name      string         `json:"name" binding:"required"`
	Scope     apitoken.Scope `json:"scope" binding:"required"`
	ExpiresAt *time.Time     `json:"expires_at"`
}

// TokenSecretResponse carries a one-time secret next to its token.
type TokenSecretResponse struct {
	Token  TokenResponse `json:"token"`
	Secret string        `json:"secret"`
}

// List handles GET /tokens.
func (h *TokenHandler) List(c *gin.Context) {
	list, err := h.uc.List(c.Request.Context())
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	items := make([]TokenResponse, 0, len(list))
	for _, t := range list {
		items = append(items, fromToken(t))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Create handles POST /tokens.
func (h *TokenHandler) Create(c *gin.Context) {
	var req TokenCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, err)
		return
	}
	t, secret, err := h.uc.Create(c.Request.Context(), apptoken.CreateCommand{
		Name: req.Name, Scope: req.Scope, ExpiresAt: req.ExpiresAt, CreatedBy: middleware.PrincipalOf(c).Email,
	})
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.MarkAudited(c)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, TokenSecretResponse{Token: fromToken(t), Secret: secret})
}

// Revoke handles DELETE /tokens/:id.
func (h *TokenHandler) Revoke(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	if err := h.uc.Revoke(c.Request.Context(), id); err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.MarkAudited(c)
	c.Status(http.StatusNoContent)
}
