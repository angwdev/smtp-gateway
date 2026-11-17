package session

import (
	"context"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=repository.go -destination=mock_repository.go -package=session

// Repository stores panel sessions.
type Repository interface {
	Create(ctx context.Context, s *Session) error
	FindByHash(ctx context.Context, idHash string) (*Session, error)
	Delete(ctx context.Context, idHash string) error // no error when already gone
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}
