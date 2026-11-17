package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"

	"github.com/angwdev/smtp-gateway/migrations"
)

// Migrate applies every pending migration. It is safe to call on each start
// and adopts a v1 database, whose tables already match the baseline. Applied
// migrations are reported through log, so callers that want silence (the
// add-credential CLI uses a discard logger) do not see goose's own output.
func Migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	for _, r := range results {
		log.Info("migration applied", "version", r.Source.Version, "duration", r.Duration)
	}
	return nil
}
