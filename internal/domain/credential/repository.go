package credential

import (
	"context"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=repository.go -destination=mock_repository.go -package=credential

// Repository stores credentials. Implementations return shared.ErrNotFound
// and shared.ErrConflict (wrapped) for missing rows and duplicate usernames.
type Repository interface {
	FindByID(ctx context.Context, id int64) (*Credential, error)
	FindByUsername(ctx context.Context, username string) (*Credential, error)
	List(ctx context.Context) ([]*Credential, error)
	Create(ctx context.Context, c *Credential) error // fills c.ID
	Update(ctx context.Context, c *Credential) error // description, senders, rate, enabled, updated_at
	SetSecretHash(ctx context.Context, id int64, hash string, at time.Time) error
	Delete(ctx context.Context, id int64) error
	Touch(ctx context.Context, id int64, ip string, at time.Time) error
}
