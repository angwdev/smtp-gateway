//go:build integration

package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openRaw(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db")+"?_pragma=foreign_keys(ON)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n))
	return n == 1
}

func TestMigrateFreshDatabase(t *testing.T) {
	db := openRaw(t)
	require.NoError(t, Migrate(context.Background(), db))
	for _, name := range []string{"credentials", "messages", "message_headers", "audit_events", "api_tokens", "sessions", "settings"} {
		assert.True(t, tableExists(t, db, name), name)
	}
	assert.False(t, tableExists(t, db, "audit"), "v1 audit table must be gone")
	require.NoError(t, Migrate(context.Background(), db), "second run must be a no-op")
}

func TestMigrateAdoptsV1Database(t *testing.T) {
	db := openRaw(t)
	legacy, err := os.ReadFile("testdata/legacy.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(legacy))
	require.NoError(t, err)

	require.NoError(t, Migrate(context.Background(), db))

	var hash, subject, key string
	require.NoError(t, db.QueryRow(`SELECT secret_hash FROM credentials WHERE username = 'nextcloud'`).Scan(&hash))
	assert.Equal(t, "aa11", hash, "credentials must survive untouched")
	require.NoError(t, db.QueryRow(`SELECT subject FROM messages WHERE id = 1`).Scan(&subject))
	assert.Empty(t, subject)
	require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE key = 'session_key'`).Scan(&key))
	assert.Equal(t, "abcd", key)

	rows, err := db.Query(`SELECT action, actor_type, target_type, target_label, detail FROM audit_events ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	var got [][5]string
	for rows.Next() {
		var r [5]string
		require.NoError(t, rows.Scan(&r[0], &r[1], &r[2], &r[3], &r[4]))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, [][5]string{
		{"session.login", "user", "session", "", ""},
		{"credential.create", "cli", "credential", "nextcloud", ""},
		{"credential.update", "user", "credential", "nextcloud", `nextcloud senders="" rate=5 enabled=true`},
		{"certificate.renew", "user", "certificate", "", "smtp.example.com"},
	}, got)
}
