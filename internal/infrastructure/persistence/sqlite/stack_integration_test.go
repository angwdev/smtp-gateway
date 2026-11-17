//go:build integration

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

// TestPureGoStack pins the three facts the persistence layer relies on:
// Goose migrates through the modernc driver, GORM runs on that same
// connection, and a UNIQUE violation is recognisable.
func TestPureGoStack(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "s.db")+"?_pragma=journal_mode(WAL)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	fsys := fstest.MapFS{"00001_t.sql": {Data: []byte(
		"-- +goose Up\nCREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE);\n-- +goose Down\nDROP TABLE t;\n")}}
	p, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	require.NoError(t, err)
	_, err = p.Up(context.Background())
	require.NoError(t, err)

	db, err := gorm.Open(gormsqlite.New(gormsqlite.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)

	type row struct {
		ID   int64
		Name string
	}
	first := row{Name: "a"}
	require.NoError(t, db.Table("t").Create(&first).Error)
	assert.NotZero(t, first.ID, "RETURNING must fill the id")

	err = db.Table("t").Create(&row{Name: "a"}).Error
	require.Error(t, err)
	assert.True(t, isUniqueViolation(err), "got %T: %v", err, err)
	assert.False(t, isUniqueViolation(nil))
}
