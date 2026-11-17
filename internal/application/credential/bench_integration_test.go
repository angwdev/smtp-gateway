//go:build integration

package credential_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	appcred "github.com/angwdev/smtp-gateway/internal/application/credential"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/persistence/sqlite"
)

// newBenchHandler opens a real SQLite file in b.TempDir and seeds one
// credential "bench", so the benchmarks exercise the same repository code
// path production does (skill rule testing-benchmark).
func newBenchHandler(b *testing.B) (h *appcred.Handler, secret string) {
	b.Helper()
	db, err := sqlite.Open(context.Background(), filepath.Join(b.TempDir(), "bench.db"), slog.New(slog.DiscardHandler))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	})
	recorder := appaudit.NewRecorder(sqlite.NewAuditRepository(db), slog.New(slog.DiscardHandler))
	h = appcred.NewHandler(sqlite.NewCredentialRepository(db), recorder)
	_, secret, err = h.Create(context.Background(), appcred.CreateCommand{Username: "bench", Enabled: true, CreatedBy: "bench"})
	if err != nil {
		b.Fatal(err)
	}
	return h, secret
}

// BenchmarkAuthenticate is the hot path of every SMTP AUTH command.
// Expected order of magnitude on the plan's host: under 100 microseconds,
// since it is a read and the LastUsedAt touch write happens at most once a
// minute per credential (section 2.5).
func BenchmarkAuthenticate(b *testing.B) {
	h, secret := newBenchHandler(b)
	cmd := appcred.AuthenticateCommand{Username: "bench", Password: secret, IP: "203.0.113.9"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := h.Authenticate(context.Background(), cmd); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAuthenticateParallel is the same path under concurrent SMTP
// sessions, which is how it actually runs in production.
func BenchmarkAuthenticateParallel(b *testing.B) {
	h, secret := newBenchHandler(b)
	cmd := appcred.AuthenticateCommand{Username: "bench", Password: secret, IP: "203.0.113.9"}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := h.Authenticate(context.Background(), cmd); err != nil {
				b.Fatal(err)
			}
		}
	})
}
