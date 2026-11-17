package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	domain "github.com/angwdev/smtp-gateway/internal/domain/session"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

var now = time.Unix(1_800_000_000, 0)

func setup(t *testing.T) (*Handler, *domain.MockRepository) {
	t.Helper()
	repo := domain.NewMockRepository(gomock.NewController(t))
	h := NewHandler(repo, time.Hour)
	h.now = func() time.Time { return now }
	return h, repo
}

func TestCreate(t *testing.T) {
	h, repo := setup(t)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, s *domain.Session) error {
		assert.Equal(t, "ana@example.com", s.Email)
		assert.Equal(t, now.Add(time.Hour), s.ExpiresAt)
		return nil
	})
	raw, s, err := h.Create(context.Background(), "ana@example.com", "Ana", "203.0.113.9", "Firefox")
	require.NoError(t, err)
	assert.NotEmpty(t, raw)
	assert.Equal(t, domain.HashID(raw), s.IDHash)
}

func TestCreateRepositoryError(t *testing.T) {
	h, repo := setup(t)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(assert.AnError)
	_, _, err := h.Create(context.Background(), "ana@example.com", "Ana", "203.0.113.9", "Firefox")
	require.ErrorIs(t, err, assert.AnError)
}

func TestResolveEmptyID(t *testing.T) {
	h, _ := setup(t)
	_, err := h.Resolve(context.Background(), "")
	require.ErrorIs(t, err, shared.ErrUnauthorized)
}

func TestResolveUnknown(t *testing.T) {
	h, repo := setup(t)
	repo.EXPECT().FindByHash(gomock.Any(), domain.HashID("raw")).Return(nil, shared.ErrNotFound)
	_, err := h.Resolve(context.Background(), "raw")
	require.ErrorIs(t, err, shared.ErrUnauthorized)
}

func TestResolveRepositoryError(t *testing.T) {
	h, repo := setup(t)
	repo.EXPECT().FindByHash(gomock.Any(), domain.HashID("raw")).Return(nil, assert.AnError)
	_, err := h.Resolve(context.Background(), "raw")
	require.ErrorIs(t, err, assert.AnError)
}

func TestResolveValid(t *testing.T) {
	h, repo := setup(t)
	s := &domain.Session{IDHash: domain.HashID("raw"), Email: "ana@example.com", ExpiresAt: now.Add(time.Minute)}
	repo.EXPECT().FindByHash(gomock.Any(), domain.HashID("raw")).Return(s, nil)
	got, err := h.Resolve(context.Background(), "raw")
	require.NoError(t, err)
	assert.Equal(t, s, got)
}

func TestResolveExpiredIsDeleted(t *testing.T) {
	h, repo := setup(t)
	s := &domain.Session{IDHash: domain.HashID("raw"), ExpiresAt: now.Add(-time.Minute)}
	repo.EXPECT().FindByHash(gomock.Any(), domain.HashID("raw")).Return(s, nil)
	repo.EXPECT().Delete(gomock.Any(), s.IDHash).Return(nil)
	_, err := h.Resolve(context.Background(), "raw")
	require.ErrorIs(t, err, shared.ErrUnauthorized)
}

func TestResolveExpiredDeleteError(t *testing.T) {
	h, repo := setup(t)
	s := &domain.Session{IDHash: domain.HashID("raw"), ExpiresAt: now.Add(-time.Minute)}
	repo.EXPECT().FindByHash(gomock.Any(), domain.HashID("raw")).Return(s, nil)
	repo.EXPECT().Delete(gomock.Any(), s.IDHash).Return(assert.AnError)
	_, err := h.Resolve(context.Background(), "raw")
	require.ErrorIs(t, err, assert.AnError)
}

func TestDestroy(t *testing.T) {
	h, repo := setup(t)
	repo.EXPECT().Delete(gomock.Any(), domain.HashID("raw")).Return(nil)
	err := h.Destroy(context.Background(), "raw")
	require.NoError(t, err)

	repo.EXPECT().Delete(gomock.Any(), domain.HashID("raw")).Return(assert.AnError)
	err = h.Destroy(context.Background(), "raw")
	require.ErrorIs(t, err, assert.AnError)
}

func TestTTL(t *testing.T) {
	h, _ := setup(t)
	assert.Equal(t, time.Hour, h.TTL())
}
