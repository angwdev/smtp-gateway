// Package session holds the use cases for server-side panel sessions.
package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "github.com/angwdev/smtp-gateway/internal/domain/session"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// Handler implements the session use cases.
type Handler struct {
	repo domain.Repository
	ttl  time.Duration
	now  func() time.Time
}

// NewHandler returns the session use cases with a fixed lifetime.
func NewHandler(repo domain.Repository, ttl time.Duration) *Handler {
	return &Handler{repo: repo, ttl: ttl, now: time.Now}
}

// Create starts a session and returns the raw id for the cookie.
func (h *Handler) Create(ctx context.Context, email, name, ip, userAgent string) (string, *domain.Session, error) {
	s, raw, err := domain.New(email, name, ip, userAgent, h.ttl, h.now())
	if err != nil {
		return "", nil, fmt.Errorf("create session: %w", err)
	}
	if err := h.repo.Create(ctx, s); err != nil {
		return "", nil, fmt.Errorf("create session: %w", err)
	}
	return raw, s, nil
}

// Resolve returns the session for a cookie value, or shared.ErrUnauthorized
// when it is empty, unknown or expired. An expired session is deleted.
func (h *Handler) Resolve(ctx context.Context, rawID string) (*domain.Session, error) {
	if rawID == "" {
		return nil, shared.ErrUnauthorized
	}
	s, err := h.repo.FindByHash(ctx, domain.HashID(rawID))
	if errors.Is(err, shared.ErrNotFound) {
		return nil, shared.ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("resolve session: %w", err)
	}
	if s.Expired(h.now()) {
		if err := h.repo.Delete(ctx, s.IDHash); err != nil {
			return nil, fmt.Errorf("resolve session: %w", err)
		}
		return nil, shared.ErrUnauthorized
	}
	return s, nil
}

// Destroy ends a session. A cookie for a session that is already gone is
// not an error.
func (h *Handler) Destroy(ctx context.Context, rawID string) error {
	if err := h.repo.Delete(ctx, domain.HashID(rawID)); err != nil {
		return fmt.Errorf("destroy session: %w", err)
	}
	return nil
}

// TTL returns the configured session lifetime, for the cookie's max age.
func (h *Handler) TTL() time.Duration { return h.ttl }
