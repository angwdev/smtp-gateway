package apitoken

import (
	"context"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=repository.go -destination=mock_repository.go -package=apitoken

// Repository stores API tokens.
type Repository interface {
	Create(ctx context.Context, t *Token) error // fills t.ID; shared.ErrConflict on a duplicate name
	List(ctx context.Context) ([]*Token, error) // newest first, revoked included
	FindByHash(ctx context.Context, hash string) (*Token, error)
	Revoke(ctx context.Context, id int64, at time.Time) error // shared.ErrNotFound if missing or already revoked
	Touch(ctx context.Context, id int64, ip string, at time.Time) error
}
