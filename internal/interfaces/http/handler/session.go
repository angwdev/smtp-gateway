package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

// SessionHandler serves /api/v1/session.
type SessionHandler struct{}

// NewSessionHandler returns the handler.
func NewSessionHandler() *SessionHandler { return &SessionHandler{} }

// SessionUserResponse is the signed-in user, omitted for a token.
type SessionUserResponse struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// SessionResponse is the API view of the current principal.
type SessionResponse struct {
	Auth      string               `json:"auth"`
	User      *SessionUserResponse `json:"user"`
	TokenName string               `json:"token_name,omitempty"`
	Scope     string               `json:"scope"`
	ExpiresAt *time.Time           `json:"expires_at"`
}

// Get handles GET /session. Authenticate has already resolved the principal.
func (h *SessionHandler) Get(c *gin.Context) {
	p := middleware.PrincipalOf(c)
	resp := SessionResponse{Scope: string(p.Scope)}
	if p.Kind == middleware.KindUser {
		resp.Auth = "session"
		resp.User = &SessionUserResponse{Email: p.Email, Name: p.Name}
		resp.ExpiresAt = utcPtr(&p.ExpiresAt)
	} else {
		resp.Auth = "token"
		resp.TokenName = p.Name
	}
	c.JSON(http.StatusOK, resp)
}
