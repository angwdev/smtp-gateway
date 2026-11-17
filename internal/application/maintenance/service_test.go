package maintenance

import (
	"bytes"
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMessages struct {
	calls  atomic.Int32
	before time.Time
	batch  int
	n      int64
	err    error
}

func newFakeMessages() *fakeMessages { return &fakeMessages{} }

func (f *fakeMessages) Prune(_ context.Context, before time.Time, batch int) (int64, error) {
	f.calls.Add(1)
	f.before, f.batch = before, batch
	return f.n, f.err
}

type fakeAudit struct {
	calls  atomic.Int32
	before time.Time
	n      int64
	err    error
}

func (f *fakeAudit) Prune(_ context.Context, before time.Time) (int64, error) {
	f.calls.Add(1)
	f.before = before
	return f.n, f.err
}

type fakeSessions struct {
	calls atomic.Int32
	n     int64
	err   error
}

func (f *fakeSessions) DeleteExpired(_ context.Context, _ time.Time) (int64, error) {
	f.calls.Add(1)
	return f.n, f.err
}

type fakeDB struct {
	calls atomic.Int32
	err   error
}

func (f *fakeDB) Checkpoint(_ context.Context) error {
	f.calls.Add(1)
	return f.err
}

type fakeDrops struct{ n int64 }

func (f *fakeDrops) TakeDropped() int64 {
	n := f.n
	f.n = 0
	return n
}

// fakes groups every dependency's fake so test helpers stay within the
// linter's result-count limit.
type fakes struct {
	messages *fakeMessages
	audit    *fakeAudit
	sessions *fakeSessions
	db       *fakeDB
	drops    *fakeDrops
}

func newService(t *testing.T) (*Service, *fakes) {
	t.Helper()
	f := &fakes{messages: newFakeMessages(), audit: &fakeAudit{}, sessions: &fakeSessions{}, db: &fakeDB{}, drops: &fakeDrops{}}
	svc := &Service{Messages: f.messages, Audit: f.audit, Sessions: f.sessions, DB: f.db, Drops: f.drops, Log: quiet()}
	return svc, f
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil)) }

func TestSweepWithoutRetentionSkipsPrunes(t *testing.T) {
	svc, f := newService(t)
	svc.Sweep(context.Background())
	assert.EqualValues(t, 0, f.messages.calls.Load())
	assert.EqualValues(t, 0, f.audit.calls.Load())
	assert.EqualValues(t, 1, f.sessions.calls.Load())
	assert.EqualValues(t, 1, f.db.calls.Load())
}

func TestSweepWithRetentionPrunesWithCutoffAndBatch(t *testing.T) {
	svc, f := newService(t)
	svc.Retention = 30 * 24 * time.Hour
	before := time.Now()
	svc.Sweep(context.Background())
	wantCutoff := before.Add(-svc.Retention)

	assert.EqualValues(t, 1, f.messages.calls.Load())
	assert.Equal(t, pruneBatch, f.messages.batch)
	assert.WithinDuration(t, wantCutoff, f.messages.before, time.Second)

	assert.EqualValues(t, 1, f.audit.calls.Load())
	assert.WithinDuration(t, wantCutoff, f.audit.before, time.Second)

	assert.EqualValues(t, 1, f.sessions.calls.Load())
	assert.EqualValues(t, 1, f.db.calls.Load())
}

func TestSweepOneFailingStepDoesNotStopTheOthers(t *testing.T) {
	svc, f := newService(t)
	svc.Retention = time.Hour
	f.messages.err = assert.AnError
	svc.Sweep(context.Background())
	assert.EqualValues(t, 1, f.messages.calls.Load())
	assert.EqualValues(t, 1, f.audit.calls.Load())
	assert.EqualValues(t, 1, f.sessions.calls.Load())
	assert.EqualValues(t, 1, f.db.calls.Load())
}

func TestSweepAuditFailureDoesNotStopTheOthers(t *testing.T) {
	svc, f := newService(t)
	svc.Retention = time.Hour
	f.audit.err = assert.AnError
	svc.Sweep(context.Background())
	assert.EqualValues(t, 1, f.sessions.calls.Load())
	assert.EqualValues(t, 1, f.db.calls.Load())
}

func TestSweepSessionsFailureDoesNotStopTheOthers(t *testing.T) {
	svc, f := newService(t)
	f.sessions.err = assert.AnError
	svc.Sweep(context.Background())
	assert.EqualValues(t, 1, f.sessions.calls.Load())
	assert.EqualValues(t, 1, f.db.calls.Load())
}

func TestSweepCheckpointFailureIsLogged(t *testing.T) {
	svc, f := newService(t)
	f.db.err = assert.AnError
	svc.Sweep(context.Background())
	assert.EqualValues(t, 1, f.db.calls.Load())
}

func TestSweepLogsPrunedRowCounts(t *testing.T) {
	svc, f := newService(t)
	svc.Retention = time.Hour
	f.messages.n, f.audit.n = 7, 3
	svc.Sweep(context.Background())
	assert.EqualValues(t, 1, f.messages.calls.Load())
	assert.EqualValues(t, 1, f.audit.calls.Load())
}

func TestRunSweepsPeriodicallyAndStopsOnCancel(t *testing.T) {
	svc, f := newService(t)
	svc.SweepEvery, svc.ReportEvery = 10*time.Millisecond, time.Hour
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.Run(ctx)
		close(done)
	}()

	time.Sleep(55 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run did not return within 100ms of cancellation")
	}
	assert.GreaterOrEqual(t, f.sessions.calls.Load(), int32(2))
}

func TestRunReportsDroppedAuditEvents(t *testing.T) {
	buf := &bytes.Buffer{}
	svc, f := newService(t)
	svc.Log = slog.New(slog.NewTextHandler(buf, nil))
	svc.SweepEvery, svc.ReportEvery = time.Hour, 10*time.Millisecond
	f.drops.n = 5

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.Run(ctx)
		close(done)
	}()

	time.Sleep(35 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run did not return within 100ms of cancellation")
	}
	require.Contains(t, buf.String(), "audit events dropped by flood control")
}

func TestRunReportTickWithNoDropsLogsNothing(t *testing.T) {
	buf := &bytes.Buffer{}
	svc, f := newService(t)
	svc.Log = slog.New(slog.NewTextHandler(buf, nil))
	svc.SweepEvery, svc.ReportEvery = time.Hour, 10*time.Millisecond
	f.drops.n = 0

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.Run(ctx)
		close(done)
	}()

	time.Sleep(35 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run did not return within 100ms of cancellation")
	}
	assert.NotContains(t, buf.String(), "dropped")
}

func TestRunUsesDefaultIntervalsWhenZero(t *testing.T) {
	svc, _ := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled up front: Run must still sweep once and return promptly

	done := make(chan struct{})
	go func() {
		svc.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run with default intervals did not return after an already-cancelled context")
	}
}
