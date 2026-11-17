package credential

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	domain "github.com/angwdev/smtp-gateway/internal/domain/credential"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

const (
	targetType = "credential"
	// touchInterval bounds how often a busy client causes a database write.
	touchInterval = time.Minute
)

// Auditor records audit entries. *audit.Recorder implements it.
type Auditor interface {
	Record(ctx context.Context, e appaudit.Entry)
}

// Handler implements the credential use cases.
type Handler struct {
	repo  domain.Repository
	audit Auditor
	now   func() time.Time
}

// NewHandler wires the use cases to a repository and an auditor.
func NewHandler(repo domain.Repository, auditor Auditor) *Handler {
	return &Handler{repo: repo, audit: auditor, now: time.Now}
}

// dummyHash keeps the cost of a wrong username equal to a wrong password.
var dummyHash = shared.HashSecret("dummy-secret-for-timing")

// snapshot is the audited view of a credential. It never includes the secret.
func snapshot(c *domain.Credential) map[string]any {
	senders := []string(c.AllowedSenders)
	if senders == nil {
		senders = []string{}
	}
	return map[string]any{
		"description":     c.Description,
		"allowed_senders": senders,
		"rate_per_hour":   c.RatePerHour,
		"enabled":         c.Enabled,
	}
}

func (h *Handler) record(ctx context.Context, action string, c *domain.Credential, before, after map[string]any) {
	id := c.ID
	h.audit.Record(ctx, appaudit.Entry{
		Action: action, TargetType: targetType, TargetID: &id, TargetLabel: c.Username,
		Changes: appaudit.Diff(before, after),
	})
}

// Create stores a new credential and returns it with its one-time secret.
//
//nolint:gocritic // hugeParam: keep plan's value API
func (h *Handler) Create(ctx context.Context, cmd CreateCommand) (*domain.Credential, string, error) {
	c, secret, err := domain.New(cmd.Username, cmd.Description, cmd.AllowedSenders, cmd.RatePerHour, cmd.Enabled, cmd.CreatedBy, h.now())
	if err != nil {
		return nil, "", err
	}
	if err := h.repo.Create(ctx, c); err != nil {
		return nil, "", fmt.Errorf("create credential: %w", err)
	}
	h.record(ctx, "credential.create", c, nil, snapshot(c))
	return c, secret, nil
}

// Get returns one credential.
func (h *Handler) Get(ctx context.Context, id int64) (*domain.Credential, error) {
	c, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get credential: %w", err)
	}
	return c, nil
}

// List returns every credential.
func (h *Handler) List(ctx context.Context) ([]*domain.Credential, error) {
	list, err := h.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	return list, nil
}

// Update applies the command. A change to Enabled alone is audited as
// credential.enable or credential.disable; anything else as credential.update.
// When nothing changes, nothing is written.
func (h *Handler) Update(ctx context.Context, cmd UpdateCommand) (*domain.Credential, error) {
	c, err := h.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("update credential: %w", err)
	}
	before := snapshot(c)
	if err := c.Apply(cmd.Description, cmd.AllowedSenders, cmd.RatePerHour, cmd.Enabled, h.now()); err != nil {
		return nil, err
	}
	after := snapshot(c)
	changes := appaudit.Diff(before, after)
	if len(changes) == 0 {
		return c, nil
	}
	if err := h.repo.Update(ctx, c); err != nil {
		return nil, fmt.Errorf("update credential: %w", err)
	}
	action := "credential.update"
	if _, only := changes["enabled"]; only && len(changes) == 1 {
		action = "credential.disable"
		if c.Enabled {
			action = "credential.enable"
		}
	}
	h.record(ctx, action, c, before, after)
	return c, nil
}

// Rotate issues a new secret. The old one stops working at once.
func (h *Handler) Rotate(ctx context.Context, id int64) (*domain.Credential, string, error) {
	c, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return nil, "", fmt.Errorf("rotate credential: %w", err)
	}
	secret, err := c.Rotate(h.now())
	if err != nil {
		return nil, "", fmt.Errorf("rotate credential: %w", err)
	}
	if err := h.repo.SetSecretHash(ctx, c.ID, c.SecretHash, c.UpdatedAt); err != nil {
		return nil, "", fmt.Errorf("rotate credential: %w", err)
	}
	h.record(ctx, "credential.rotate", c, nil, nil)
	return c, secret, nil
}

// Delete removes a credential. Its messages stay in the log.
func (h *Handler) Delete(ctx context.Context, id int64) error {
	c, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	if err := h.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	h.record(ctx, "credential.delete", c, nil, nil)
	return nil
}

// Authenticate checks an SMTP login. Unknown user, wrong password and
// disabled credential all return shared.ErrInvalidLogin; any other error is
// an infrastructure failure the caller should answer with a temporary error.
func (h *Handler) Authenticate(ctx context.Context, cmd AuthenticateCommand) (*domain.Credential, error) {
	c, err := h.repo.FindByUsername(ctx, cmd.Username)
	if errors.Is(err, shared.ErrNotFound) {
		shared.SecretMatches(dummyHash, cmd.Password)
		return nil, shared.ErrInvalidLogin
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	if !c.Verify(cmd.Password) {
		return nil, shared.ErrInvalidLogin
	}
	now := h.now()
	if c.LastUsedAt == nil || c.LastUsedIP != cmd.IP || now.Sub(*c.LastUsedAt) >= touchInterval {
		if err := h.repo.Touch(ctx, c.ID, cmd.IP, now); err != nil {
			return nil, fmt.Errorf("authenticate: %w", err)
		}
		c.LastUsedAt, c.LastUsedIP = &now, cmd.IP
	}
	return c, nil
}
