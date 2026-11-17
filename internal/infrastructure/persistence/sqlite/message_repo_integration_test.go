//go:build integration

package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

func TestMessageRepository(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	creds := NewCredentialRepository(db)
	repo := NewMessageRepository(db)
	c, _ := mustCredential(t, creds, "nextcloud")

	record := func(at time.Time, status message.Status, subject string, h *message.Headers) *message.Message {
		t.Helper()
		m := &message.Message{CreatedAt: at, CredentialID: &c.ID, Username: c.Username, RemoteIP: "203.0.113.9",
			MailFrom: "a@example.com", RcptTo: []string{"b@example.com", "c@example.com"}, Size: 10,
			Status: status, Detail: "2.0.0 Ok", Subject: subject}
		require.NoError(t, repo.Record(ctx, m, h))
		require.NotZero(t, m.ID)
		return m
	}

	sent := record(t0, message.StatusSent, "Weekly 100% report", &message.Headers{Raw: "Subject: Weekly 100% report\r\n\r\n"})
	failed := record(t0.Add(time.Minute), message.StatusFailed, "", nil)
	record(t0.Add(2*time.Hour), message.StatusRejected, "", nil)

	t.Run("sent bumps the counter, others do not", func(t *testing.T) {
		got, err := creds.FindByID(ctx, c.ID)
		require.NoError(t, err)
		assert.EqualValues(t, 1, got.SentCount)
	})

	t.Run("find with headers", func(t *testing.T) {
		got, err := repo.FindByID(ctx, sent.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{"b@example.com", "c@example.com"}, got.RcptTo)
		assert.Equal(t, "Weekly 100% report", got.Subject)
		h, err := repo.Headers(ctx, sent.ID)
		require.NoError(t, err)
		require.NotNil(t, h)
		assert.Contains(t, h.Raw, "Subject:")
		h, err = repo.Headers(ctx, failed.ID)
		require.NoError(t, err)
		assert.Nil(t, h)
		_, err = repo.FindByID(ctx, 9999)
		assert.ErrorIs(t, err, shared.ErrNotFound)
	})

	t.Run("list filters and paginates newest first", func(t *testing.T) {
		all, err := repo.List(ctx, message.Filter{})
		require.NoError(t, err)
		require.Len(t, all, 3)
		assert.Greater(t, all[0].ID, all[1].ID)

		page, err := repo.List(ctx, message.Filter{Limit: 2})
		require.NoError(t, err)
		require.Len(t, page, 2)
		rest, err := repo.List(ctx, message.Filter{Limit: 2, BeforeID: page[1].ID})
		require.NoError(t, err)
		require.Len(t, rest, 1)

		byStatus, err := repo.List(ctx, message.Filter{Status: message.StatusFailed})
		require.NoError(t, err)
		require.Len(t, byStatus, 1)

		// % is a LIKE wildcard: it must be matched literally.
		bySubject, err := repo.List(ctx, message.Filter{Query: "100% rep"})
		require.NoError(t, err)
		require.Len(t, bySubject, 1)
		none, err := repo.List(ctx, message.Filter{Query: "1%0"})
		require.NoError(t, err)
		assert.Empty(t, none)

		window, err := repo.List(ctx, message.Filter{From: t0.Add(30 * time.Second), To: t0.Add(time.Hour)})
		require.NoError(t, err)
		require.Len(t, window, 1)
		assert.Equal(t, failed.ID, window[0].ID)

		byCredential, err := repo.List(ctx, message.Filter{CredentialID: c.ID})
		require.NoError(t, err)
		assert.Len(t, byCredential, 3)
	})

	t.Run("counts and series", func(t *testing.T) {
		n, err := repo.CountSince(ctx, c.ID, t0.Add(-time.Hour))
		require.NoError(t, err)
		assert.Equal(t, 2, n, "rejected messages do not count toward the rate limit")

		counts, err := repo.StatusCounts(ctx, t0.Add(-time.Hour))
		require.NoError(t, err)
		assert.Equal(t, map[message.Status]int{message.StatusSent: 1, message.StatusFailed: 1, message.StatusRejected: 1}, counts)

		series, err := repo.HourlySeries(ctx, t0.Add(-time.Hour))
		require.NoError(t, err)
		require.Len(t, series, 2)
		assert.Equal(t, t0.Truncate(time.Hour).Unix(), series[0].Hour.Unix())
		assert.Equal(t, 1, series[0].Counts[message.StatusSent])
		assert.Equal(t, 1, series[1].Counts[message.StatusRejected])
	})

	t.Run("prune in batches cascades to headers", func(t *testing.T) {
		for i := range 7 {
			record(t0.Add(-48*time.Hour), message.StatusSent, fmt.Sprint(i), &message.Headers{Raw: "x"})
		}
		n, err := repo.Prune(ctx, t0.Add(-time.Hour), 3)
		require.NoError(t, err)
		assert.EqualValues(t, 7, n)
		var headers int64
		require.NoError(t, db.Read.Raw("SELECT COUNT(*) FROM message_headers").Scan(&headers).Error)
		assert.EqualValues(t, 1, headers, "only the header of the surviving message is left")
	})
}

func TestMessageRepositoryDatabaseFailures(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	creds := NewCredentialRepository(db)
	repo := NewMessageRepository(db)
	c, _ := mustCredential(t, creds, "closed")
	m := &message.Message{CreatedAt: t0, CredentialID: &c.ID, Username: c.Username,
		MailFrom: "a@example.com", RcptTo: []string{"b@example.com"}, Status: message.StatusSent}
	require.NoError(t, repo.Record(ctx, m, nil))
	require.NoError(t, db.Close())

	assert.Error(t, repo.Record(ctx, &message.Message{CreatedAt: t0, Status: message.StatusSent}, nil))

	_, err := repo.FindByID(ctx, m.ID)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, shared.ErrNotFound)

	_, err = repo.Headers(ctx, m.ID)
	assert.Error(t, err)

	_, err = repo.List(ctx, message.Filter{})
	assert.Error(t, err)

	_, err = repo.CountSince(ctx, c.ID, t0)
	assert.Error(t, err)

	_, err = repo.StatusCounts(ctx, t0)
	assert.Error(t, err)

	_, err = repo.HourlySeries(ctx, t0)
	assert.Error(t, err)

	_, err = repo.Prune(ctx, t0.Add(time.Hour), 10)
	assert.Error(t, err)
}
