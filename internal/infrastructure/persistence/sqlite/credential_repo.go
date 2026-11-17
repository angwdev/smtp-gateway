package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/angwdev/smtp-gateway/internal/domain/credential"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// CredentialRepository implements credential.Repository.
type CredentialRepository struct{ db *DB }

var _ credential.Repository = (*CredentialRepository)(nil)

// NewCredentialRepository returns a repository over db.
func NewCredentialRepository(db *DB) *CredentialRepository { return &CredentialRepository{db: db} }

func (r *CredentialRepository) one(q *gorm.DB, what string) (*credential.Credential, error) {
	var m credentialModel
	if err := q.Take(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%s: %w", what, shared.ErrNotFound)
		}
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return m.toDomain(), nil
}

// FindByID returns the credential with that id.
func (r *CredentialRepository) FindByID(ctx context.Context, id int64) (*credential.Credential, error) {
	return r.one(r.db.Read.WithContext(ctx).Where("id = ?", id), fmt.Sprintf("find credential %d", id))
}

// FindByUsername matches case-insensitively (the column is COLLATE NOCASE).
func (r *CredentialRepository) FindByUsername(ctx context.Context, username string) (*credential.Credential, error) {
	return r.one(r.db.Read.WithContext(ctx).Where("username = ?", username), "find credential by username")
}

// List returns every credential ordered by username.
func (r *CredentialRepository) List(ctx context.Context) ([]*credential.Credential, error) {
	var rows []credentialModel
	if err := r.db.Read.WithContext(ctx).Order("username COLLATE NOCASE").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	out := make([]*credential.Credential, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

// Create inserts c and fills c.ID.
func (r *CredentialRepository) Create(ctx context.Context, c *credential.Credential) error {
	m := credentialModelFrom(c)
	if err := r.db.Write.WithContext(ctx).Create(m).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create credential %q: %w", c.Username, shared.ErrConflict)
		}
		return fmt.Errorf("create credential %q: %w", c.Username, err)
	}
	c.ID = m.ID
	return nil
}

func (r *CredentialRepository) update(ctx context.Context, id int64, what string, values map[string]any) error {
	res := r.db.Write.WithContext(ctx).Model(&credentialModel{}).Where("id = ?", id).Updates(values)
	if res.Error != nil {
		return fmt.Errorf("%s %d: %w", what, id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%s %d: %w", what, id, shared.ErrNotFound)
	}
	return nil
}

// Update saves the editable fields.
func (r *CredentialRepository) Update(ctx context.Context, c *credential.Credential) error {
	return r.update(ctx, c.ID, "update credential", map[string]any{
		"description":     c.Description,
		"allowed_senders": strings.Join(c.AllowedSenders, "\n"),
		"rate_per_hour":   c.RatePerHour,
		"enabled":         c.Enabled,
		"updated_at":      c.UpdatedAt.Unix(),
	})
}

// SetSecretHash replaces the stored hash.
func (r *CredentialRepository) SetSecretHash(ctx context.Context, id int64, hash string, at time.Time) error {
	return r.update(ctx, id, "rotate credential", map[string]any{"secret_hash": hash, "updated_at": at.Unix()})
}

// Touch records the last successful login.
func (r *CredentialRepository) Touch(ctx context.Context, id int64, ip string, at time.Time) error {
	return r.update(ctx, id, "touch credential", map[string]any{"last_used_at": at.Unix(), "last_used_ip": ip})
}

// Delete removes the credential. Its messages stay (credential_id becomes NULL).
func (r *CredentialRepository) Delete(ctx context.Context, id int64) error {
	res := r.db.Write.WithContext(ctx).Where("id = ?", id).Delete(&credentialModel{})
	if res.Error != nil {
		return fmt.Errorf("delete credential %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("delete credential %d: %w", id, shared.ErrNotFound)
	}
	return nil
}
