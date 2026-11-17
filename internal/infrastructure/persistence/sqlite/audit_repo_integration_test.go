//go:build integration

package sqlite

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

	t.Run("a multibyte detail is truncated without splitting a rune", func(t *testing.T) {
		// "本" is 3 bytes; 400 repeats is 1200 bytes, over MaxDetail (1000),
		// and 1000 is not a multiple of 3, so a byte-only cut would split
		// the last rune in two and produce invalid UTF-8.
		long := strings.Repeat("本", 400)
		e := record(t0, "cli", audit.ActorCLI, "upstream.test", audit.ResultOK, long, "", nil)
		got, err := repo.List(ctx, audit.Filter{BeforeID: e.ID + 1, Limit: 1})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.True(t, utf8.ValidString(got[0].Detail), "truncated detail must stay valid UTF-8")
		assert.LessOrEqual(t, len(got[0].Detail), audit.MaxDetail)
		assert.NotEmpty(t, got[0].Detail)
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

	t.Run("filter by target type, from and to", func(t *testing.T) {
		e := &audit.Event{CreatedAt: t0, Action: "credential.update", TargetType: "credential"}
		require.NoError(t, repo.Append(ctx, e))

		byTarget, err := repo.List(ctx, audit.Filter{TargetType: "credential", BeforeID: e.ID + 1, Limit: 1})
		require.NoError(t, err)
		require.Len(t, byTarget, 1)
		assert.Equal(t, e.ID, byTarget[0].ID)

		byFrom, err := repo.List(ctx, audit.Filter{From: t0, BeforeID: e.ID + 1, Limit: 1})
		require.NoError(t, err)
		require.Len(t, byFrom, 1)
		assert.Equal(t, e.ID, byFrom[0].ID)

		byTo, err := repo.List(ctx, audit.Filter{To: t0, BeforeID: e.ID + 1, Limit: 1})
		require.NoError(t, err)
		require.Len(t, byTo, 1)
		assert.Equal(t, e.ID, byTo[0].ID)

		missed, err := repo.List(ctx, audit.Filter{To: t0.Add(-time.Hour), BeforeID: e.ID + 1, Limit: 1})
		require.NoError(t, err)
		assert.Empty(t, missed)
	})
}

func TestAppendRejectsChangesThatDoNotMarshal(t *testing.T) {
	db := newTestDB(t)
	repo := NewAuditRepository(db)
	e := &audit.Event{CreatedAt: t0, Action: "credential.update",
		Changes: map[string]audit.Change{"rate_per_hour": {From: math.NaN(), To: 5}}}
	err := repo.Append(context.Background(), e)
	require.Error(t, err)
	assert.Zero(t, e.ID, "a failed append must not fill in an id")
}

func TestListFailsOnACorruptChangesColumn(t *testing.T) {
	// Append always writes valid JSON; a hand-crafted row simulates data
	// corrupted some other way, exercising toDomain's decode error path.
	db := newTestDB(t)
	repo := NewAuditRepository(db)
	require.NoError(t, db.Write.Exec(
		`INSERT INTO audit_events (created_at, actor, actor_type, action, target_type, target_label,
			result, ip, user_agent, request_id, detail, changes) VALUES
			(?, '', 'system', 'credential.update', '', '', 'ok', '', '', '', '', 'not json')`, t0.Unix()).Error)

	_, err := repo.List(context.Background(), audit.Filter{})
	require.Error(t, err)
}

func TestAuditRepositoryDatabaseFailures(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewAuditRepository(db)
	require.NoError(t, db.Close())

	err := repo.Append(ctx, &audit.Event{CreatedAt: t0, Action: "credential.update"})
	assert.Error(t, err)

	_, err = repo.List(ctx, audit.Filter{})
	assert.Error(t, err)

	_, err = repo.Prune(ctx, t0)
	assert.Error(t, err)
}
