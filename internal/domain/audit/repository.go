package audit

import (
	"context"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=repository.go -destination=mock_repository.go -package=audit

// Repository stores audit events. Events are append-only: there is no update
// and no delete other than retention pruning.
type Repository interface {
	Append(ctx context.Context, e *Event) error // fills e.ID
	List(ctx context.Context, f Filter) ([]*Event, error)
	Prune(ctx context.Context, before time.Time) (int64, error)
}
