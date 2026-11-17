package sqlite

import (
	"context"
	"fmt"
)

// Checkpoint folds the write-ahead log back into the database file and
// truncates it, then lets SQLite refresh its query-planner statistics.
func (d *DB) Checkpoint(ctx context.Context) error {
	if _, err := d.write.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint wal: %w", err)
	}
	if _, err := d.write.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return fmt.Errorf("optimize: %w", err)
	}
	return nil
}
