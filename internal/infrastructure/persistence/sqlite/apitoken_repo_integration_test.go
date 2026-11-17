//go:build integration

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

func mustToken(t *testing.T, repo *APITokenRepository, name string) (tok *apitoken.Token, secret string) {
	t.Helper()
	tok, secret, err := apitoken.New(name, apitoken.ScopeRead, nil, "test", t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), tok))
	return tok, secret
}

func TestAPITokenRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewAPITokenRepository(newTestDB(t))

	tok, _ := mustToken(t, repo, "grafana")
	require.NotZero(t, tok.ID)

	t.Run("duplicate name is a conflict", func(t *testing.T) {
		dup, _, err := apitoken.New("grafana", apitoken.ScopeWrite, nil, "", t0)
		require.NoError(t, err)
		assert.ErrorIs(t, repo.Create(ctx, dup), shared.ErrConflict)
	})

	t.Run("find by hash", func(t *testing.T) {
		got, err := repo.FindByHash(ctx, tok.Hash)
		require.NoError(t, err)
		assert.Equal(t, tok.ID, got.ID)
		assert.Equal(t, "grafana", got.Name)
	})

	t.Run("list is newest first", func(t *testing.T) {
		mustToken(t, repo, "another")
		list, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, list, 2)
		assert.Equal(t, "another", list[0].Name)
	})

	t.Run("touch", func(t *testing.T) {
		require.NoError(t, repo.Touch(ctx, tok.ID, "203.0.113.9", t0.Add(time.Minute)))
		got, err := repo.FindByHash(ctx, tok.Hash)
		require.NoError(t, err)
		assert.Equal(t, "203.0.113.9", got.LastUsedIP)
		require.NotNil(t, got.LastUsedAt)
	})

	t.Run("revoke once then not found", func(t *testing.T) {
		require.NoError(t, repo.Revoke(ctx, tok.ID, t0.Add(2*time.Minute)))
		got, err := repo.FindByHash(ctx, tok.Hash)
		require.NoError(t, err)
		require.NotNil(t, got.RevokedAt)

		assert.ErrorIs(t, repo.Revoke(ctx, tok.ID, t0), shared.ErrNotFound)
		assert.ErrorIs(t, repo.Revoke(ctx, 9999, t0), shared.ErrNotFound)
	})

	t.Run("touch a missing token is not found", func(t *testing.T) {
		assert.ErrorIs(t, repo.Touch(ctx, 9999, "203.0.113.9", t0), shared.ErrNotFound)
	})

	t.Run("an expiry date round-trips", func(t *testing.T) {
		future := t0.Add(time.Hour)
		withExpiry, _, err := apitoken.New("expiring", apitoken.ScopeRead, &future, "", t0)
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, withExpiry))
		got, err := repo.FindByHash(ctx, withExpiry.Hash)
		require.NoError(t, err)
		require.NotNil(t, got.ExpiresAt)
		assert.Equal(t, future.Unix(), got.ExpiresAt.Unix())
	})
}

func TestAPITokenRepositoryDatabaseFailures(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewAPITokenRepository(db)
	tok, _ := mustToken(t, repo, "closed")
	require.NoError(t, db.Close())

	_, err := repo.List(ctx)
	assert.Error(t, err)

	_, err = repo.FindByHash(ctx, tok.Hash)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, shared.ErrNotFound)

	other, _, err := apitoken.New("another", apitoken.ScopeRead, nil, "", t0)
	require.NoError(t, err)
	assert.Error(t, repo.Create(ctx, other))

	err = repo.Revoke(ctx, tok.ID, t0)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, shared.ErrNotFound)

	err = repo.Touch(ctx, tok.ID, "203.0.113.9", t0)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, shared.ErrNotFound)
}
