//go:build integration

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/domain/session"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

func TestSessionRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewSessionRepository(newTestDB(t))

	s, _, err := session.New("ana@example.com", "Ana", "203.0.113.9", "Firefox", time.Hour, t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, s))

	t.Run("find by hash", func(t *testing.T) {
		got, err := repo.FindByHash(ctx, s.IDHash)
		require.NoError(t, err)
		assert.Equal(t, "ana@example.com", got.Email)
		assert.Equal(t, "Ana", got.Name)
	})

	t.Run("delete twice without error", func(t *testing.T) {
		require.NoError(t, repo.Delete(ctx, s.IDHash))
		require.NoError(t, repo.Delete(ctx, s.IDHash))
		_, err := repo.FindByHash(ctx, s.IDHash)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("delete expired removes only expired rows", func(t *testing.T) {
		expired, _, err := session.New("old@example.com", "", "", "", time.Hour, t0)
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, expired))

		fresh, _, err := session.New("fresh@example.com", "", "", "", time.Hour, t0.Add(time.Hour))
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, fresh))

		n, err := repo.DeleteExpired(ctx, t0.Add(time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)

		_, err = repo.FindByHash(ctx, expired.IDHash)
		assert.ErrorIs(t, err, shared.ErrNotFound)
		_, err = repo.FindByHash(ctx, fresh.IDHash)
		assert.NoError(t, err)
	})
}

func TestSessionRepositoryDatabaseFailures(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewSessionRepository(db)
	s, _, err := session.New("closed@example.com", "", "", "", time.Hour, t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, s))
	require.NoError(t, db.Close())

	fresh, _, err := session.New("another@example.com", "", "", "", time.Hour, t0)
	require.NoError(t, err)
	assert.Error(t, repo.Create(ctx, fresh))

	_, err = repo.FindByHash(ctx, s.IDHash)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, shared.ErrNotFound)

	assert.Error(t, repo.Delete(ctx, s.IDHash))

	_, err = repo.DeleteExpired(ctx, t0)
	assert.Error(t, err)
}
