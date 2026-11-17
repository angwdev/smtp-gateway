//go:build integration

package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewSettingsRepository(newTestDB(t))

	t.Run("get or create calls gen once", func(t *testing.T) {
		calls := 0
		gen := func() (string, error) {
			calls++
			return "generated", nil
		}
		first, err := repo.GetOrCreate(ctx, "session_key", gen)
		require.NoError(t, err)
		assert.Equal(t, "generated", first)

		second, err := repo.GetOrCreate(ctx, "session_key", gen)
		require.NoError(t, err)
		assert.Equal(t, "generated", second)
		assert.Equal(t, 1, calls)
	})

	t.Run("a gen error is propagated", func(t *testing.T) {
		wantErr := errors.New("boom")
		_, err := repo.GetOrCreate(ctx, "other_key", func() (string, error) { return "", wantErr })
		require.ErrorIs(t, err, wantErr)
	})
}

func TestCheckpoint(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Checkpoint(context.Background()))
}
