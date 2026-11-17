//go:build integration

package message_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"strconv"
	"testing"

	appmsg "github.com/angwdev/smtp-gateway/internal/application/message"
	domain "github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/persistence/sqlite"
)

const seedRows = 10_000

// newBenchHandler opens a real SQLite file in b.TempDir, so the benchmarks
// exercise the same repository code production uses (skill rule
// testing-benchmark).
func newBenchHandler(b *testing.B) *appmsg.Handler {
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
	return appmsg.NewHandler(sqlite.NewMessageRepository(db), appmsg.ContentHeaders)
}

// headerBlock builds a raw header block at least size bytes long, the
// "2 KiB header block" the plan asks BenchmarkRecord to use.
func headerBlock(size int, tag string) []byte {
	h := make([]byte, 0, size+64)
	h = append(h, "From: a@example.com\r\nSubject: bench "+tag+"\r\n"...)
	for len(h) < size {
		h = append(h, 'x')
	}
	h = append(h, "\r\n\r\n"...)
	return h
}

func recordCommand(username, tag string, status domain.Status) appmsg.RecordCommand {
	return appmsg.RecordCommand{
		Username: username, RemoteIP: "203.0.113.9", MailFrom: "a@example.com",
		RcptTo: []string{"b@example.com"}, Size: 1024, Status: status,
		Detail: "queued", Header: headerBlock(2048, tag),
	}
}

// seed inserts n messages, nine sent for every one failed, so
// BenchmarkListFiltered has rows to find.
func seed(b *testing.B, h *appmsg.Handler, n int) {
	b.Helper()
	for i := range n {
		status := domain.StatusSent
		if i%10 == 0 {
			status = domain.StatusFailed
		}
		if err := h.Record(context.Background(), recordCommand("bench", strconv.Itoa(i), status)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRecord is the hot path of every accepted SMTP message: one
// transaction writing the message row, its header row and the credential's
// sent-count bump (section 2.5). Expected order of magnitude on the plan's
// host: under 2 milliseconds.
func BenchmarkRecord(b *testing.B) {
	h := newBenchHandler(b)
	cmd := recordCommand("bench", "record", domain.StatusSent)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := h.Record(context.Background(), cmd); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkList is the panel's default message page: 10,000 rows seeded,
// listing the newest 50 by keyset pagination with no filter. Expected order
// of magnitude on the plan's host: under 2 milliseconds.
func BenchmarkList(b *testing.B) {
	h := newBenchHandler(b)
	seed(b, h, seedRows)
	f := domain.Filter{Limit: 50}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := h.List(context.Background(), f); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListFiltered is the same page with a status and a text filter,
// the more expensive query shape the panel's Messages screen also sends.
func BenchmarkListFiltered(b *testing.B) {
	h := newBenchHandler(b)
	seed(b, h, seedRows)
	f := domain.Filter{Limit: 50, Status: domain.StatusFailed, Query: "bench"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := h.List(context.Background(), f); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListWhileWriting is the "panel query must not block SMTP" case
// from section 2.5: a goroutine records continuously on the single-writer
// pool while the benchmark lists on the four-connection read pool.
func BenchmarkListWhileWriting(b *testing.B) {
	h := newBenchHandler(b)
	seed(b, h, seedRows)
	f := domain.Filter{Limit: 50}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ctx.Err() == nil; i++ {
			_ = h.Record(context.Background(), recordCommand("writer", strconv.Itoa(i), domain.StatusSent)) //nolint:errcheck // best-effort background writer; the benchmark only checks List
		}
	}()
	b.Cleanup(func() {
		cancel()
		<-done
	})

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := h.List(context.Background(), f); err != nil {
			b.Fatal(err)
		}
	}
}
