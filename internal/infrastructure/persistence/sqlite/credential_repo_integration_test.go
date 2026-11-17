//go:build integration

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/domain/credential"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

var t0 = time.Unix(1_800_000_000, 0)

func mustCredential(t *testing.T, repo *CredentialRepository, username string) (c *credential.Credential, secret string) {
	t.Helper()
	c, secret, err := credential.New(username, "", nil, 0, true, "test", t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), c))
	return c, secret
}

func TestCredentialRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewCredentialRepository(newTestDB(t))

	c, _ := mustCredential(t, repo, "Nextcloud")
	require.NotZero(t, c.ID)

	t.Run("find by id and by username, case-insensitive", func(t *testing.T) {
		got, err := repo.FindByID(ctx, c.ID)
		require.NoError(t, err)
		assert.Equal(t, "Nextcloud", got.Username)
		assert.Equal(t, c.SecretHash, got.SecretHash)
		assert.True(t, got.Enabled)
		assert.Equal(t, t0.Unix(), got.CreatedAt.Unix())
		assert.Nil(t, got.LastUsedAt)

		got, err = repo.FindByUsername(ctx, "nextcloud")
		require.NoError(t, err)
		assert.Equal(t, c.ID, got.ID)
	})

	t.Run("duplicate username is a conflict", func(t *testing.T) {
		dup, _, err := credential.New("NEXTCLOUD", "", nil, 0, true, "", t0)
		require.NoError(t, err)
		assert.ErrorIs(t, repo.Create(ctx, dup), shared.ErrConflict)
	})

	t.Run("update, touch, rotate", func(t *testing.T) {
		desc, rate, off := "files", 7, false
		senders := []string{"@example.com", "a@b.c"}
		require.NoError(t, c.Apply(&desc, &senders, &rate, &off, t0.Add(time.Minute)))
		require.NoError(t, repo.Update(ctx, c))
		require.NoError(t, repo.Touch(ctx, c.ID, "203.0.113.9", t0.Add(2*time.Minute)))
		require.NoError(t, repo.SetSecretHash(ctx, c.ID, "newhash", t0.Add(3*time.Minute)))

		got, err := repo.FindByID(ctx, c.ID)
		require.NoError(t, err)
		assert.Equal(t, "files", got.Description)
		assert.Equal(t, credential.SenderPolicy{"@example.com", "a@b.c"}, got.AllowedSenders)
		assert.Equal(t, 7, got.RatePerHour)
		assert.False(t, got.Enabled)
		assert.Equal(t, "203.0.113.9", got.LastUsedIP)
		require.NotNil(t, got.LastUsedAt)
		assert.Equal(t, "newhash", got.SecretHash)
		assert.Equal(t, t0.Add(3*time.Minute).Unix(), got.UpdatedAt.Unix())
	})

	t.Run("list is ordered by username", func(t *testing.T) {
		mustCredential(t, repo, "alpha")
		list, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, list, 2)
		assert.Equal(t, "alpha", list[0].Username)
	})

	t.Run("missing rows", func(t *testing.T) {
		_, err := repo.FindByID(ctx, 9999)
		assert.ErrorIs(t, err, shared.ErrNotFound)
		_, err = repo.FindByUsername(ctx, "nobody")
		assert.ErrorIs(t, err, shared.ErrNotFound)
		assert.ErrorIs(t, repo.Delete(ctx, 9999), shared.ErrNotFound)
		assert.ErrorIs(t, repo.Update(ctx, &credential.Credential{ID: 9999}), shared.ErrNotFound)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.Delete(ctx, c.ID))
		_, err := repo.FindByID(ctx, c.ID)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})
}

func TestCredentialRepositoryDatabaseFailures(t *testing.T) {
	// A closed pool makes every call fail with a real database error
	// (not the not-found / conflict errors the happy-path tests already
	// cover), exercising each method's generic error-wrap branch.
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewCredentialRepository(db)
	c, _, err := credential.New("closed", "", nil, 0, true, "test", t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, c))
	require.NoError(t, db.Close())

	_, err = repo.FindByID(ctx, c.ID)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, shared.ErrNotFound)

	_, err = repo.List(ctx)
	assert.Error(t, err)

	other, _, err := credential.New("another", "", nil, 0, true, "test", t0)
	require.NoError(t, err)
	assert.Error(t, repo.Create(ctx, other))

	err = repo.Touch(ctx, c.ID, "203.0.113.9", t0)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, shared.ErrNotFound)

	assert.Error(t, repo.Delete(ctx, c.ID))
}
