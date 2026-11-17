package audit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	domain "github.com/angwdev/smtp-gateway/internal/domain/audit"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRecordTakesMetaFromContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domain.NewMockRepository(ctrl)
	var got *domain.Event
	repo.EXPECT().Append(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, e *domain.Event) error {
		got = e
		return nil
	})

	ctx := WithMeta(context.Background(), Meta{Actor: "ana@example.com", ActorType: domain.ActorUser,
		IP: "203.0.113.9", UserAgent: "Firefox", RequestID: "req-1"})
	id := int64(3)
	NewRecorder(repo, quiet()).Record(ctx, Entry{Action: "credential.rotate", TargetType: "credential", TargetID: &id, TargetLabel: "nextcloud"})

	require.NotNil(t, got)
	assert.Equal(t, "ana@example.com", got.Actor)
	assert.Equal(t, domain.ActorUser, got.ActorType)
	assert.Equal(t, "203.0.113.9", got.IP)
	assert.Equal(t, "Firefox", got.UserAgent)
	assert.Equal(t, "req-1", got.RequestID)
	assert.Equal(t, domain.ResultOK, got.Result)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestRecordWithoutMetaIsSystem(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domain.NewMockRepository(ctrl)
	repo.EXPECT().Append(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, e *domain.Event) error {
		assert.Equal(t, "system", e.Actor)
		assert.Equal(t, domain.ActorSystem, e.ActorType)
		return nil
	})
	NewRecorder(repo, quiet()).Record(context.Background(), Entry{Action: "certificate.renew"})
}

func TestRecordSurvivesCancelledContextAndRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domain.NewMockRepository(ctrl)
	repo.EXPECT().Append(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ *domain.Event) error {
		assert.NoError(t, ctx.Err(), "the audit write must not inherit the request's cancellation")
		return errors.New("disk full")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	NewRecorder(repo, quiet()).Record(ctx, Entry{Action: "session.logout"}) // must not panic
}

func TestRecordNoisyDropsAboveTheBurst(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domain.NewMockRepository(ctrl)
	repo.EXPECT().Append(gomock.Any(), gomock.Any()).Return(nil).Times(noisyBurst)
	r := NewRecorder(repo, quiet())
	for range noisyBurst + 20 {
		r.RecordNoisy(context.Background(), Entry{Action: "smtp.auth_failed"})
	}
	assert.EqualValues(t, 20, r.TakeDropped())
	assert.Zero(t, r.TakeDropped())
}

func TestDiff(t *testing.T) {
	before := map[string]any{"rate_per_hour": 0, "enabled": true, "description": "x"}
	after := map[string]any{"rate_per_hour": 5, "enabled": true, "description": "x"}
	assert.Equal(t, map[string]domain.Change{"rate_per_hour": {From: 0, To: 5}}, Diff(before, after))
	assert.Empty(t, Diff(after, after))
	assert.Equal(t, map[string]domain.Change{"enabled": {From: nil, To: true}}, Diff(nil, map[string]any{"enabled": true}))
}

func TestQueriesList(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := domain.NewMockRepository(ctrl)
	q := NewQueries(repo)

	full := make([]*domain.Event, domain.DefaultLimit)
	for i := range full {
		full[i] = &domain.Event{ID: int64(i + 1)}
	}
	repo.EXPECT().List(gomock.Any(), gomock.Any()).Return(full, nil)
	events, next, err := q.List(context.Background(), domain.Filter{})
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, full[len(full)-1].ID, *next)
	assert.Len(t, events, domain.DefaultLimit)

	short := []*domain.Event{{ID: 1}}
	repo.EXPECT().List(gomock.Any(), gomock.Any()).Return(short, nil)
	events, next, err = q.List(context.Background(), domain.Filter{})
	require.NoError(t, err)
	assert.Nil(t, next)
	assert.Equal(t, short, events)

	repo.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))
	_, _, err = q.List(context.Background(), domain.Filter{})
	require.Error(t, err)
}
