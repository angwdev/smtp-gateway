package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// APITokenRepository implements apitoken.Repository.
type APITokenRepository struct{ db *DB }

var _ apitoken.Repository = (*APITokenRepository)(nil)

// NewAPITokenRepository returns a repository over db.
func NewAPITokenRepository(db *DB) *APITokenRepository { return &APITokenRepository{db: db} }

// Create inserts t and fills t.ID.
func (r *APITokenRepository) Create(ctx context.Context, t *apitoken.Token) error {
	m := apiTokenModelFrom(t)
	if err := r.db.Write.WithContext(ctx).Create(m).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create token %q: %w", t.Name, shared.ErrConflict)
		}
		return fmt.Errorf("create token %q: %w", t.Name, err)
	}
	t.ID = m.ID
	return nil
}

// List returns every token, newest first, revoked included.
func (r *APITokenRepository) List(ctx context.Context) ([]*apitoken.Token, error) {
	var rows []apiTokenModel
	if err := r.db.Read.WithContext(ctx).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	out := make([]*apitoken.Token, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

// FindByHash returns the token matching hash.
func (r *APITokenRepository) FindByHash(ctx context.Context, hash string) (*apitoken.Token, error) {
	var m apiTokenModel
	if err := r.db.Read.WithContext(ctx).Where("token_hash = ?", hash).Take(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("find token by hash: %w", shared.ErrNotFound)
		}
		return nil, fmt.Errorf("find token by hash: %w", err)
	}
	return m.toDomain(), nil
}

// Revoke marks the token revoked. It fails when the token is missing or
// already revoked, so a caller can tell the two apart from a double click.
func (r *APITokenRepository) Revoke(ctx context.Context, id int64, at time.Time) error {
	res := r.db.Write.WithContext(ctx).Model(&apiTokenModel{}).
		Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", at.Unix())
	if res.Error != nil {
		return fmt.Errorf("revoke token %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("revoke token %d: %w", id, shared.ErrNotFound)
	}
	return nil
}

// Touch records the last successful use.
func (r *APITokenRepository) Touch(ctx context.Context, id int64, ip string, at time.Time) error {
	res := r.db.Write.WithContext(ctx).Model(&apiTokenModel{}).Where("id = ?", id).
		Updates(map[string]any{"last_used_at": at.Unix(), "last_used_ip": ip})
	if res.Error != nil {
		return fmt.Errorf("touch token %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("touch token %d: %w", id, shared.ErrNotFound)
	}
	return nil
}
