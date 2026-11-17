//go:build integration

package sqlite

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDB is the helper every repository test uses.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "gateway.db"), slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpen(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Ping(context.Background()))

	var mode string
	require.NoError(t, db.Read.Raw("PRAGMA journal_mode").Scan(&mode).Error)
	assert.Equal(t, "wal", mode)

	err := db.Read.Exec("INSERT INTO settings (key, value) VALUES ('k', 'v')").Error
	require.Error(t, err, "the read pool must refuse writes")

	require.NoError(t, db.Write.Exec("INSERT INTO settings (key, value) VALUES ('k', 'v')").Error)
	var v string
	require.NoError(t, db.Read.Raw("SELECT value FROM settings WHERE key = 'k'").Scan(&v).Error)
	assert.Equal(t, "v", v)
}

func TestOpenRejectsAPathThatIsADirectory(t *testing.T) {
	// A directory is not a valid SQLite file: migration fails when it
	// actually touches the file, so Open must close the writer and report it.
	_, err := Open(context.Background(), t.TempDir(), slog.New(slog.DiscardHandler))
	require.Error(t, err)
}

func TestPingAfterCloseFails(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())
	assert.Error(t, db.Ping(context.Background()))
}
