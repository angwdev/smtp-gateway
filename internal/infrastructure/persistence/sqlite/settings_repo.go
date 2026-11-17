package sqlite

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// SettingsRepository stores small key/value settings, such as the session
// signing key. It has no domain interface: only internal/app uses it.
type SettingsRepository struct{ db *DB }

// NewSettingsRepository returns a repository over db.
func NewSettingsRepository(db *DB) *SettingsRepository { return &SettingsRepository{db: db} }

// GetOrCreate returns the stored value for key, creating it with gen on first use.
func (r *SettingsRepository) GetOrCreate(ctx context.Context, key string, gen func() (string, error)) (string, error) {
	var row settingModel
	err := r.db.Read.WithContext(ctx).Where("key = ?", key).Take(&row).Error
	if err == nil {
		return row.Value, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("read setting %q: %w", key, err)
	}
	value, err := gen()
	if err != nil {
		return "", fmt.Errorf("generate setting %q: %w", key, err)
	}
	if err := r.db.Write.WithContext(ctx).Create(&settingModel{Key: key, Value: value}).Error; err != nil {
		return "", fmt.Errorf("store setting %q: %w", key, err)
	}
	return value, nil
}
