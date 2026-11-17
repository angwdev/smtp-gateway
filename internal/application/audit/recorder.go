// Package audit is the application service that records audit events.
package audit

import (
	"context"
	"log/slog"
	"reflect"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	domain "github.com/angwdev/smtp-gateway/internal/domain/audit"
)

// Flood control for events an outsider can trigger at will (failed logins,
// token calls): about one per second sustained.
const (
	noisyBurst   = 30
	writeTimeout = 5 * time.Second
)

// Meta identifies who is acting. Interfaces put it in the context once per
// request or SMTP session.
type Meta struct {
	Actor     string
	ActorType domain.ActorType
	IP        string
	UserAgent string
	RequestID string
}

type metaKey struct{}

// WithMeta returns a context carrying m.
//
//nolint:gocritic // hugeParam: keep plan's value API
func WithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

// MetaFrom returns the Meta in ctx, or the system actor when there is none.
func MetaFrom(ctx context.Context) Meta {
	if m, ok := ctx.Value(metaKey{}).(Meta); ok {
		return m
	}
	return Meta{Actor: "system", ActorType: domain.ActorSystem}
}

// Entry is what a use case reports. Result defaults to ok.
type Entry struct {
	Action      string
	TargetType  string
	TargetID    *int64
	TargetLabel string
	Result      domain.Result
	Detail      string
	Changes     map[string]domain.Change
}

// Recorder writes audit events.
type Recorder struct {
	repo    domain.Repository
	log     *slog.Logger
	now     func() time.Time
	noisy   *rate.Limiter
	dropped atomic.Int64
}

// NewRecorder returns a Recorder writing to repo.
func NewRecorder(repo domain.Repository, log *slog.Logger) *Recorder {
	return &Recorder{repo: repo, log: log, now: time.Now, noisy: rate.NewLimiter(rate.Every(time.Second), noisyBurst)}
}

// Record writes the event. The action it describes has already happened, so
// a failed write is logged, not returned: there is nothing the caller could
// undo. The write does not inherit the caller's cancellation, because a
// client that hangs up must not be able to erase its own trail.
//
//nolint:gocritic // hugeParam: keep plan's value API
func (r *Recorder) Record(ctx context.Context, e Entry) {
	meta := MetaFrom(ctx)
	if e.Result == "" {
		e.Result = domain.ResultOK
	}
	event := &domain.Event{
		CreatedAt: r.now(), Actor: meta.Actor, ActorType: meta.ActorType, Action: e.Action,
		TargetType: e.TargetType, TargetID: e.TargetID, TargetLabel: e.TargetLabel, Result: e.Result,
		IP: meta.IP, UserAgent: meta.UserAgent, RequestID: meta.RequestID, Detail: e.Detail, Changes: e.Changes,
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	if err := r.repo.Append(wctx, event); err != nil {
		r.log.Error("audit event not stored", "action", e.Action, "actor", meta.Actor, "err", err)
	}
}

// RecordNoisy is Record for events an attacker can generate in bulk. Above
// the rate limit events are counted instead of stored.
//
//nolint:gocritic // hugeParam: keep plan's value API
func (r *Recorder) RecordNoisy(ctx context.Context, e Entry) {
	if !r.noisy.Allow() {
		r.dropped.Add(1)
		return
	}
	r.Record(ctx, e)
}

// TakeDropped returns how many noisy events were dropped since the last call.
func (r *Recorder) TakeDropped() int64 { return r.dropped.Swap(0) }

// Diff lists the keys whose value differs between before and after.
func Diff(before, after map[string]any) map[string]domain.Change {
	out := map[string]domain.Change{}
	for key, to := range after {
		if from := before[key]; !reflect.DeepEqual(from, to) {
			out[key] = domain.Change{From: from, To: to}
		}
	}
	return out
}
