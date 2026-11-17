package apitoken

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	domain "github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

const (
	targetType = "token"
	// touchInterval bounds how often a busy client causes a database write.
	touchInterval = time.Minute
)

// Auditor records audit entries. *audit.Recorder implements it.
type Auditor interface {
	Record(ctx context.Context, e appaudit.Entry)
}

// Handler implements the API token use cases.
type Handler struct {
	repo  domain.Repository
	audit Auditor
	now   func() time.Time
}

// NewHandler wires the use cases to a repository and an auditor.
func NewHandler(repo domain.Repository, auditor Auditor) *Handler {
	return &Handler{repo: repo, audit: auditor, now: time.Now}
}

// Create stores a new token and returns it with its one-time secret.
func (h *Handler) Create(ctx context.Context, cmd CreateCommand) (*domain.Token, string, error) {
	t, secret, err := domain.New(cmd.Name, cmd.Scope, cmd.ExpiresAt, cmd.CreatedBy, h.now())
	if err != nil {
		return nil, "", err
	}
	if err := h.repo.Create(ctx, t); err != nil {
		return nil, "", fmt.Errorf("create token: %w", err)
	}
	var expiresAt any
	if t.ExpiresAt != nil {
		expiresAt = t.ExpiresAt.UTC().Format(time.RFC3339)
	}
	id := t.ID
	h.audit.Record(ctx, appaudit.Entry{
		Action: "token.create", TargetType: targetType, TargetID: &id, TargetLabel: t.Name,
		Changes: appaudit.Diff(nil, map[string]any{"scope": string(t.Scope), "expires_at": expiresAt}),
	})
	return t, secret, nil
}

// List returns every token, newest first.
func (h *Handler) List(ctx context.Context) ([]*domain.Token, error) {
	list, err := h.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return list, nil
}

// Revoke disables a token. It stops working at once.
func (h *Handler) Revoke(ctx context.Context, id int64) error {
	list, err := h.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	var name string
	found := false
	for _, t := range list {
		if t.ID == id {
			name, found = t.Name, true
			break
		}
	}
	if !found {
		return fmt.Errorf("revoke token: %w", shared.ErrNotFound)
	}
	if err := h.repo.Revoke(ctx, id, h.now()); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	h.audit.Record(ctx, appaudit.Entry{Action: "token.revoke", TargetType: targetType, TargetID: &id, TargetLabel: name})
	return nil
}

// Authenticate checks a bearer token. An unrecognised prefix, an unknown
// hash, an expired or revoked token all return shared.ErrUnauthorized; any
// other error is an infrastructure failure.
func (h *Handler) Authenticate(ctx context.Context, raw, ip string) (*domain.Token, error) {
	if !strings.HasPrefix(raw, domain.Prefix) {
		return nil, shared.ErrUnauthorized
	}
	t, err := h.repo.FindByHash(ctx, shared.HashSecret(raw))
	if errors.Is(err, shared.ErrNotFound) {
		return nil, shared.ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate token: %w", err)
	}
	now := h.now()
	if !t.Active(now) {
		return nil, shared.ErrUnauthorized
	}
	if t.LastUsedAt == nil || t.LastUsedIP != ip || now.Sub(*t.LastUsedAt) >= touchInterval {
		if err := h.repo.Touch(ctx, t.ID, ip, now); err != nil {
			return nil, fmt.Errorf("authenticate token: %w", err)
		}
		t.LastUsedAt, t.LastUsedIP = &now, ip
	}
	return t, nil
}
