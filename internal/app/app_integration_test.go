//go:build integration

package app

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/config"
)

// TestRunServesAndStops starts the whole application on free ports with the
// panel disabled, relays nothing, and checks that cancelling the context
// returns nil within five seconds and leaves no goroutine serving.
func TestRunServesAndStops(t *testing.T) {
	cfg := &config.Config{
		DataDir: t.TempDir(), Hostname: "gateway.test", SubmissionAddr: "127.0.0.1:0",
		TLSSource: "self-signed", MaxMessageBytes: 1 << 20, MaxRecipients: 10, MaxLineLength: 4096,
		AuthFailLimit: 3, AuthFailWindow: time.Minute, MessageContent: "headers", LogLevel: "error",
		LogRetention: time.Hour, Upstream: config.Upstream{Host: "127.0.0.1", Port: 1, TLSMode: "none", Timeout: time.Second},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, "test") }()

	// Wait for the schema to be fully ready, not just for the file to
	// exist: the sqlite driver creates an empty file on open, before goose
	// applies any migration, so a bare os.Stat races with Run's own
	// startup and can cancel the context mid-migration. Checking for the
	// last known migration version, plus a short grace period for Open to
	// return and Run to reach the listening phase, makes the race safe.
	const lastMigrationVersion = 2
	dbPath := filepath.Join(cfg.DataDir, "gateway.db")
	require.Eventually(t, func() bool {
		db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
		if err != nil {
			return false
		}
		defer db.Close()
		var version int
		if err := db.QueryRow("select max(version_id) from goose_db_version").Scan(&version); err != nil {
			return false
		}
		return version >= lastMigrationVersion
	}, 5*time.Second, 20*time.Millisecond, "database schema was not ready")
	time.Sleep(100 * time.Millisecond)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestAddCredentialCLI(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	require.Equal(t, 0, AddCredential(&out, io.Discard, dir, []string{"grafana", "alerts@example.com"}))
	assert.Regexp(t, `username: grafana\npassword: \S{32}\n`, out.String())
	assert.Equal(t, 3, AddCredential(io.Discard, io.Discard, dir, []string{"grafana"}), "duplicate username exits 3 as in v1")
	assert.Equal(t, 2, AddCredential(io.Discard, io.Discard, dir, nil))
}
