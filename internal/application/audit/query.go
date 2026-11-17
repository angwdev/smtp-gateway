package audit

import (
	"context"
	"fmt"

	domain "github.com/angwdev/smtp-gateway/internal/domain/audit"
)

// Queries reads the audit log.
type Queries struct{ repo domain.Repository }

// NewQueries returns the read side of the audit log.
func NewQueries(repo domain.Repository) *Queries { return &Queries{repo: repo} }

// List returns one page and the cursor for the next, or nil at the end.
//
//nolint:gocritic // hugeParam: keep plan's value API
func (q *Queries) List(ctx context.Context, f domain.Filter) ([]*domain.Event, *int64, error) {
	f = f.Normalized()
	events, err := q.repo.List(ctx, f)
	if err != nil {
		return nil, nil, fmt.Errorf("list audit events: %w", err)
	}
	if len(events) < f.Limit {
		return events, nil, nil
	}
	next := events[len(events)-1].ID
	return events, &next, nil
}
