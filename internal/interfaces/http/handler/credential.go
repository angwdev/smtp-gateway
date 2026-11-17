package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	appcred "github.com/angwdev/smtp-gateway/internal/application/credential"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

// CredentialHandler serves /api/v1/credentials.
type CredentialHandler struct{ uc *appcred.Handler }

// NewCredentialHandler returns the handler.
func NewCredentialHandler(uc *appcred.Handler) *CredentialHandler { return &CredentialHandler{uc: uc} }

// CredentialCreateRequest is the body of POST /credentials.
type CredentialCreateRequest struct {
	Username       string   `json:"username" binding:"required"`
	Description    string   `json:"description"`
	AllowedSenders []string `json:"allowed_senders"`
	RatePerHour    int      `json:"rate_per_hour"`
	Enabled        *bool    `json:"enabled"` // default true
}

// CredentialPatchRequest is the body of PATCH /credentials/:id.
type CredentialPatchRequest struct {
	Description    *string   `json:"description"`
	AllowedSenders *[]string `json:"allowed_senders"`
	RatePerHour    *int      `json:"rate_per_hour"`
	Enabled        *bool     `json:"enabled"`
}

// SecretResponse carries a one-time secret next to its credential.
type SecretResponse struct {
	Credential CredentialResponse `json:"credential"`
	Secret     string             `json:"secret"`
}

// List handles GET /credentials.
func (h *CredentialHandler) List(c *gin.Context) {
	list, err := h.uc.List(c.Request.Context())
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	items := make([]CredentialResponse, 0, len(list))
	for _, cred := range list {
		items = append(items, fromCredential(cred))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Create handles POST /credentials.
func (h *CredentialHandler) Create(c *gin.Context) {
	var req CredentialCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, err)
		return
	}
	enabled := req.Enabled == nil || *req.Enabled
	cred, secret, err := h.uc.Create(c.Request.Context(), appcred.CreateCommand{
		Username: req.Username, Description: req.Description, AllowedSenders: req.AllowedSenders,
		RatePerHour: req.RatePerHour, Enabled: enabled, CreatedBy: middleware.PrincipalOf(c).Name,
	})
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.MarkAudited(c)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, SecretResponse{Credential: fromCredential(cred), Secret: secret})
}

// Get handles GET /credentials/:id.
func (h *CredentialHandler) Get(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	cred, err := h.uc.Get(c.Request.Context(), id)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, fromCredential(cred))
}

// Patch handles PATCH /credentials/:id.
func (h *CredentialHandler) Patch(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	var req CredentialPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.Fail(c, err)
		return
	}
	cred, err := h.uc.Update(c.Request.Context(), appcred.UpdateCommand{
		ID: id, Description: req.Description, AllowedSenders: req.AllowedSenders, RatePerHour: req.RatePerHour, Enabled: req.Enabled,
	})
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.MarkAudited(c)
	c.JSON(http.StatusOK, fromCredential(cred))
}

// Rotate handles POST /credentials/:id/rotate.
func (h *CredentialHandler) Rotate(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	cred, secret, err := h.uc.Rotate(c.Request.Context(), id)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.MarkAudited(c)
	c.JSON(http.StatusOK, SecretResponse{Credential: fromCredential(cred), Secret: secret})
}

// Delete handles DELETE /credentials/:id.
func (h *CredentialHandler) Delete(c *gin.Context) {
	id, err := pathID(c)
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	if err := h.uc.Delete(c.Request.Context(), id); err != nil {
		middleware.Fail(c, err)
		return
	}
	middleware.MarkAudited(c)
	c.Status(http.StatusNoContent)
}
