// Package maintenance runs the periodic housekeeping: retention pruning,
// expired sessions, WAL checkpoint and the report of dropped audit events.
package maintenance

import (
	"context"
	"log/slog"
	"time"
)

const pruneBatch = 5000

// The service depends on behaviour, not on packages (small interfaces).
type (
	// MessagePruner deletes old message-log rows.
	MessagePruner interface {
		Prune(ctx context.Context, before time.Time, batch int) (int64, error)
	}
	// AuditPruner deletes old audit rows.
	AuditPruner interface {
		Prune(ctx context.Context, before time.Time) (int64, error)
	}
	// SessionPruner deletes expired sessions.
	SessionPruner interface {
		DeleteExpired(ctx context.Context, now time.Time) (int64, error)
	}
	// Checkpointer compacts the database log.
	Checkpointer interface {
		Checkpoint(ctx context.Context) error
	}
	// DropCounter reports audit events lost to flood control.
	DropCounter interface {
		TakeDropped() int64
	}
)

// Service is the housekeeping loop.
type Service struct {
	Messages  MessagePruner
	Audit     AuditPruner
	Sessions  SessionPruner
	DB        Checkpointer
	Drops     DropCounter
	Retention time.Duration // 0 keeps logs forever
	Log       *slog.Logger

	// Intervals; zero values get the defaults in Run. Tests shorten them.
	SweepEvery, ReportEvery time.Duration
}

// Run sweeps once at start, then on every tick, until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	sweepEvery, reportEvery := s.SweepEvery, s.ReportEvery
	if sweepEvery == 0 {
		sweepEvery = time.Hour
	}
	if reportEvery == 0 {
		reportEvery = time.Minute
	}
	sweep, report := time.NewTicker(sweepEvery), time.NewTicker(reportEvery)
	defer sweep.Stop()
	defer report.Stop()

	s.Sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			s.Sweep(ctx)
		case <-report.C:
			if n := s.Drops.TakeDropped(); n > 0 {
				s.Log.Warn("audit events dropped by flood control", "count", n)
			}
		}
	}
}

// Sweep runs every housekeeping step once. A failing step is logged and the
// others still run.
func (s *Service) Sweep(ctx context.Context) {
	now := time.Now()
	if s.Retention > 0 {
		before := now.Add(-s.Retention)
		if n, err := s.Messages.Prune(ctx, before, pruneBatch); err != nil {
			s.Log.Error("prune messages", "err", err)
		} else if n > 0 {
			s.Log.Info("pruned message log", "rows", n)
		}
		if n, err := s.Audit.Prune(ctx, before); err != nil {
			s.Log.Error("prune audit log", "err", err)
		} else if n > 0 {
			s.Log.Info("pruned audit log", "rows", n)
		}
	}
	if _, err := s.Sessions.DeleteExpired(ctx, now); err != nil {
		s.Log.Error("delete expired sessions", "err", err)
	}
	if err := s.DB.Checkpoint(ctx); err != nil {
		s.Log.Error("checkpoint", "err", err)
	}
}
