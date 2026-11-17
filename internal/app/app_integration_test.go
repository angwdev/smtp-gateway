//go:build integration

package app

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net"
	"net/url"
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

// TestRunCancelsWhenPanelFailsToBuild forces buildPanel to fail (a malformed
// WEB_TRUSTED_PROXIES entry, the one reachable way NewRouter errors; Run
// takes an already-built Config and does not revalidate it the way
// config.Load does) and checks that Run still stops the goroutines it had
// already scheduled (acmeMgr.Run, gateway.Run) instead of returning while
// they keep running against a database the deferred Close is about to shut.
func TestRunCancelsWhenPanelFailsToBuild(t *testing.T) {
	// Reserve a free port, then release it so Run can bind it; after Run
	// returns, binding it again proves the SMTP listener was closed.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	publicURL, err := url.Parse("http://panel.test")
	require.NoError(t, err)

	cfg := &config.Config{
		DataDir: t.TempDir(), Hostname: "gateway.test", SubmissionAddr: addr,
		TLSSource: "self-signed", MaxMessageBytes: 1 << 20, MaxRecipients: 10, MaxLineLength: 4096,
		AuthFailLimit: 3, AuthFailWindow: time.Minute, MessageContent: "headers", LogLevel: "error",
		LogRetention: time.Hour, Upstream: config.Upstream{Host: "127.0.0.1", Port: 1, TLSMode: "none", Timeout: time.Second},
		WebAddr: "127.0.0.1:0", PublicURL: publicURL, TrustedProxies: []string{"not-a-cidr-or-ip"},
	}

	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), cfg, "test") }()

	select {
	case err := <-done:
		require.Error(t, err, "Run must report buildPanel's error")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after buildPanel failed")
	}

	ln2, err := net.Listen("tcp", addr)
	require.NoError(t, err, "SMTP port was not released: Run left gateway.Run running")
	require.NoError(t, ln2.Close())
}

func TestAddCredentialCLI(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	require.Equal(t, 0, AddCredential(&out, io.Discard, dir, []string{"grafana", "alerts@example.com"}))
	assert.Regexp(t, `username: grafana\npassword: \S{32}\n`, out.String())
	assert.Equal(t, 3, AddCredential(io.Discard, io.Discard, dir, []string{"grafana"}), "duplicate username exits 3 as in v1")
	assert.Equal(t, 2, AddCredential(io.Discard, io.Discard, dir, nil))
}
