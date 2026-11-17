package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/angwdev/smtp-gateway/internal/domain/session"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// SessionRepository implements session.Repository.
type SessionRepository struct{ db *DB }

var _ session.Repository = (*SessionRepository)(nil)

// NewSessionRepository returns a repository over db.
func NewSessionRepository(db *DB) *SessionRepository { return &SessionRepository{db: db} }

// Create inserts s.
func (r *SessionRepository) Create(ctx context.Context, s *session.Session) error {
	if err := r.db.Write.WithContext(ctx).Create(sessionModelFrom(s)).Error; err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// FindByHash returns the session with that id hash.
func (r *SessionRepository) FindByHash(ctx context.Context, idHash string) (*session.Session, error) {
	var m sessionModel
	if err := r.db.Read.WithContext(ctx).Where("id_hash = ?", idHash).Take(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("find session: %w", shared.ErrNotFound)
		}
		return nil, fmt.Errorf("find session: %w", err)
	}
	return m.toDomain(), nil
}

// Delete removes the session. Deleting an already-gone session is not an
// error: logout must be idempotent.
func (r *SessionRepository) Delete(ctx context.Context, idHash string) error {
	if err := r.db.Write.WithContext(ctx).Where("id_hash = ?", idHash).Delete(&sessionModel{}).Error; err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpired removes sessions whose lifetime ended at or before now.
func (r *SessionRepository) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res := r.db.Write.WithContext(ctx).Where("expires_at <= ?", now.Unix()).Delete(&sessionModel{})
	if res.Error != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}
