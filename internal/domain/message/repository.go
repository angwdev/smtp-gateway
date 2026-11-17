package message

import (
	"context"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=repository.go -destination=mock_repository.go -package=message

// Repository stores the message log.
type Repository interface {
	// Record inserts m (filling m.ID) with its headers when h is not nil, and
	// bumps the credential's sent counter for a sent message, atomically.
	Record(ctx context.Context, m *Message, h *Headers) error
	FindByID(ctx context.Context, id int64) (*Message, error)
	// Headers returns nil, nil when none were stored.
	Headers(ctx context.Context, id int64) (*Headers, error)
	// List returns newest first.
	List(ctx context.Context, f Filter) ([]*Message, error)
	// CountSince counts sent and failed messages of a credential (rate limit).
	CountSince(ctx context.Context, credentialID int64, since time.Time) (int, error)
	StatusCounts(ctx context.Context, since time.Time) (map[Status]int, error)
	// HourlySeries returns only hours that have messages, oldest first.
	HourlySeries(ctx context.Context, since time.Time) ([]Bucket, error)
	// Prune deletes rows older than before in batches and returns the total.
	Prune(ctx context.Context, before time.Time, batch int) (int64, error)
}
