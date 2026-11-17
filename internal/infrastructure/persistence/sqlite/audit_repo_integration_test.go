//go:build integration

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/domain/audit"
)

func TestAuditRepository(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewAuditRepository(db)

	record := func(at time.Time, actor string, actorType audit.ActorType, action string, result audit.Result,
		detail, ip string, changes map[string]audit.Change) *audit.Event {
		t.Helper()
		e := &audit.Event{CreatedAt: at, Actor: actor, ActorType: actorType, Action: action, Result: result,
			Detail: detail, IP: ip, Changes: changes}
		require.NoError(t, repo.Append(ctx, e))
		require.NotZero(t, e.ID)
		return e
	}

	t.Run("append then list round-trips changes", func(t *testing.T) {
		e := record(t0, "ana@example.com", audit.ActorUser, "credential.rotate", audit.ResultOK,
			"", "", map[string]audit.Change{"rate_per_hour": {From: 0, To: 5}})
		got, err := repo.List(ctx, audit.Filter{})
		require.NoError(t, err)
		require.NotEmpty(t, got)
		var found *audit.Event
		for _, ev := range got {
			if ev.ID == e.ID {
				found = ev
			}
		}
		require.NotNil(t, found)
		assert.Equal(t, map[string]audit.Change{"rate_per_hour": {From: float64(0), To: float64(5)}}, found.Changes)
	})

	t.Run("empty actor type and result default to system and ok", func(t *testing.T) {
		e := &audit.Event{CreatedAt: t0, Action: "certificate.renew"}
		require.NoError(t, repo.Append(ctx, e))
		got, err := repo.List(ctx, audit.Filter{Action: "certificate.renew"})
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.Equal(t, audit.ActorSystem, got[0].ActorType)
		assert.Equal(t, audit.ResultOK, got[0].Result)
	})

	t.Run("a long detail is truncated", func(t *testing.T) {
		long := make([]byte, 2000)
		for i := range long {
			long[i] = 'x'
		}
		e := record(t0, "cli", audit.ActorCLI, "upstream.test", audit.ResultOK, string(long), "", nil)
		got, err := repo.List(ctx, audit.Filter{BeforeID: e.ID + 1, Limit: 1})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Len(t, got[0].Detail, audit.MaxDetail)
	})

	t.Run("filter by action prefix and exact action", func(t *testing.T) {
		record(t0, "a", audit.ActorUser, "credential.create", audit.ResultOK, "", "", nil)
		record(t0, "a", audit.ActorUser, "credential.update", audit.ResultOK, "", "", nil)
		record(t0, "a", audit.ActorUser, "session.login", audit.ResultOK, "", "", nil)

		byPrefix, err := repo.List(ctx, audit.Filter{Action: "credential."})
		require.NoError(t, err)
		actions := make([]string, 0, len(byPrefix))
		for _, e := range byPrefix {
			actions = append(actions, e.Action)
		}
		assert.Contains(t, actions, "credential.create")
		assert.Contains(t, actions, "credential.update")
		assert.NotContains(t, actions, "session.login")

		exact, err := repo.List(ctx, audit.Filter{Action: "credential.create"})
		require.NoError(t, err)
		assert.Len(t, exact, 1)
	})

	t.Run("filter by actor, result, query and before id", func(t *testing.T) {
		record(t0, "unique-actor@example.com", audit.ActorUser, "session.login", audit.ResultOK, "", "", nil)
		byActor, err := repo.List(ctx, audit.Filter{Actor: "unique-actor@example.com"})
		require.NoError(t, err)
		require.Len(t, byActor, 1)

		denied := record(t0, "b", audit.ActorUser, "session.login_denied", audit.ResultDenied, "", "", nil)
		byResult, err := repo.List(ctx, audit.Filter{Result: audit.ResultDenied})
		require.NoError(t, err)
		require.Len(t, byResult, 1)
		assert.Equal(t, denied.ID, byResult[0].ID)

		record(t0, "c", audit.ActorUser, "token.auth_failed", audit.ResultDenied, "", "203.0.113.9", nil)
		byQuery, err := repo.List(ctx, audit.Filter{Query: "203.0.113"})
		require.NoError(t, err)
		require.Len(t, byQuery, 1)

		all, err := repo.List(ctx, audit.Filter{})
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(all), 2)
		page, err := repo.List(ctx, audit.Filter{BeforeID: all[0].ID, Limit: 1})
		require.NoError(t, err)
		require.Len(t, page, 1)
		assert.Equal(t, all[1].ID, page[0].ID)
	})

	t.Run("prune removes only rows older than the cut-off", func(t *testing.T) {
		old := record(t0.Add(-48*time.Hour), "d", audit.ActorUser, "session.login", audit.ResultOK, "", "", nil)
		recent := record(t0, "d", audit.ActorUser, "session.login", audit.ResultOK, "", "", nil)
		n, err := repo.Prune(ctx, t0.Add(-time.Hour))
		require.NoError(t, err)
		assert.EqualValues(t, 1, n)

		_, err = repo.List(ctx, audit.Filter{BeforeID: old.ID + 1, Limit: 1})
		require.NoError(t, err)
		got, err := repo.List(ctx, audit.Filter{BeforeID: recent.ID + 1, Limit: 1})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, recent.ID, got[0].ID)
	})
}
