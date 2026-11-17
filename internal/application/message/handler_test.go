package message

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	domain "github.com/angwdev/smtp-gateway/internal/domain/message"
)

var fixedNow = time.Date(2026, 11, 17, 10, 30, 0, 0, time.UTC)

func newHandler(t *testing.T, policy ContentPolicy) (*Handler, *domain.MockRepository) {
	t.Helper()
	repo := domain.NewMockRepository(gomock.NewController(t))
	h := NewHandler(repo, policy)
	h.now = func() time.Time { return fixedNow }
	return h, repo
}

func headerBlock(subject string) []byte {
	return []byte("From: a@b.c\r\nSubject: " + subject + "\r\n\r\n")
}

func TestRecordPolicyHeaders(t *testing.T) {
	h, repo := newHandler(t, ContentHeaders)
	repo.EXPECT().Record(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m *domain.Message, headers *domain.Headers) error {
			assert.Equal(t, "Weekly report", m.Subject)
			require.NotNil(t, headers)
			assert.False(t, headers.Truncated)
			return nil
		})
	err := h.Record(context.Background(), RecordCommand{Header: headerBlock("Weekly report"), Status: domain.StatusSent})
	require.NoError(t, err)
}

func TestRecordPolicySubject(t *testing.T) {
	h, repo := newHandler(t, ContentSubject)
	repo.EXPECT().Record(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m *domain.Message, headers *domain.Headers) error {
			assert.Equal(t, "Weekly report", m.Subject)
			assert.Nil(t, headers)
			return nil
		})
	err := h.Record(context.Background(), RecordCommand{Header: headerBlock("Weekly report"), Status: domain.StatusSent})
	require.NoError(t, err)
}

func TestRecordPolicyNone(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	repo.EXPECT().Record(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m *domain.Message, headers *domain.Headers) error {
			assert.Empty(t, m.Subject)
			assert.Nil(t, headers)
			return nil
		})
	err := h.Record(context.Background(), RecordCommand{Header: headerBlock("Weekly report"), Status: domain.StatusSent})
	require.NoError(t, err)
}

func TestRecordNilHeaderUnderAnyPolicy(t *testing.T) {
	for _, policy := range []ContentPolicy{ContentHeaders, ContentSubject, ContentNone} {
		t.Run(string(policy), func(t *testing.T) {
			h, repo := newHandler(t, policy)
			repo.EXPECT().Record(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, m *domain.Message, headers *domain.Headers) error {
					assert.Empty(t, m.Subject)
					assert.Nil(t, headers)
					return nil
				})
			err := h.Record(context.Background(), RecordCommand{Status: domain.StatusSent})
			require.NoError(t, err)
		})
	}
}

func TestRecordRejectedAppendedToDetail(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	repo.EXPECT().Record(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m *domain.Message, _ *domain.Headers) error {
			assert.Equal(t, "ok (upstream refused: a@b.c, c@d.e)", m.Detail)
			return nil
		})
	err := h.Record(context.Background(), RecordCommand{Detail: "ok", Rejected: []string{"a@b.c", "c@d.e"}, Status: domain.StatusRejected})
	require.NoError(t, err)
}

func TestRecordDetailTruncatedTo500(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	long := strings.Repeat("x", 2000)
	repo.EXPECT().Record(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m *domain.Message, _ *domain.Headers) error {
			assert.Len(t, m.Detail, domain.MaxDetail)
			return nil
		})
	err := h.Record(context.Background(), RecordCommand{Detail: long, Status: domain.StatusFailed})
	require.NoError(t, err)
}

func TestRecordHeaderBlockTruncatedTo16KiB(t *testing.T) {
	h, repo := newHandler(t, ContentHeaders)
	big := []byte("Subject: x\r\n" + strings.Repeat("X-Pad: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\n", 500) + "\r\n")
	require.Greater(t, len(big), 20<<10)
	repo.EXPECT().Record(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *domain.Message, headers *domain.Headers) error {
			require.NotNil(t, headers)
			assert.True(t, headers.Truncated)
			assert.LessOrEqual(t, len(headers.Raw), domain.MaxStoredHeaderBytes)
			return nil
		})
	err := h.Record(context.Background(), RecordCommand{Header: big, Status: domain.StatusSent})
	require.NoError(t, err)
}

func TestRecordRepositoryErrorIsWrapped(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	repo.EXPECT().Record(gomock.Any(), gomock.Any(), gomock.Any()).Return(assert.AnError)
	err := h.Record(context.Background(), RecordCommand{Status: domain.StatusSent})
	require.ErrorIs(t, err, assert.AnError)
}

func TestCountSince(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	repo.EXPECT().CountSince(gomock.Any(), int64(3), fixedNow).Return(4, nil)
	n, err := h.CountSince(context.Background(), 3, fixedNow)
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	repo.EXPECT().CountSince(gomock.Any(), int64(3), fixedNow).Return(0, assert.AnError)
	_, err = h.CountSince(context.Background(), 3, fixedNow)
	require.ErrorIs(t, err, assert.AnError)
}

func TestList(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	full := make([]*domain.Message, domain.DefaultLimit)
	for i := range full {
		full[i] = &domain.Message{ID: int64(i + 1)}
	}
	repo.EXPECT().List(gomock.Any(), gomock.Any()).Return(full, nil)
	_, next, err := h.List(context.Background(), domain.Filter{})
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, full[len(full)-1].ID, *next)

	short := []*domain.Message{{ID: 1}}
	repo.EXPECT().List(gomock.Any(), gomock.Any()).Return(short, nil)
	list, next, err := h.List(context.Background(), domain.Filter{})
	require.NoError(t, err)
	assert.Nil(t, next)
	assert.Equal(t, short, list)

	repo.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, assert.AnError)
	_, _, err = h.List(context.Background(), domain.Filter{})
	require.ErrorIs(t, err, assert.AnError)
}

func TestGetWithHeaders(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	m := &domain.Message{ID: 9}
	repo.EXPECT().FindByID(gomock.Any(), int64(9)).Return(m, nil)
	repo.EXPECT().Headers(gomock.Any(), int64(9)).Return(&domain.Headers{Raw: "From: a@b.c\r\n\r\n"}, nil)
	d, err := h.Get(context.Background(), 9)
	require.NoError(t, err)
	assert.Equal(t, m, d.Message)
	assert.Equal(t, []HeaderField{{Name: "From", Value: "a@b.c"}}, d.Fields)
}

func TestGetWithoutHeaders(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	m := &domain.Message{ID: 9}
	repo.EXPECT().FindByID(gomock.Any(), int64(9)).Return(m, nil)
	repo.EXPECT().Headers(gomock.Any(), int64(9)).Return(nil, nil)
	d, err := h.Get(context.Background(), 9)
	require.NoError(t, err)
	assert.Empty(t, d.Fields)
	assert.Empty(t, d.Raw)
}

func TestGetNotFound(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	repo.EXPECT().FindByID(gomock.Any(), int64(99)).Return(nil, errors.New("missing"))
	_, err := h.Get(context.Background(), 99)
	require.Error(t, err)
}

func TestGetHeadersError(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	m := &domain.Message{ID: 9}
	repo.EXPECT().FindByID(gomock.Any(), int64(9)).Return(m, nil)
	repo.EXPECT().Headers(gomock.Any(), int64(9)).Return(nil, assert.AnError)
	_, err := h.Get(context.Background(), 9)
	require.ErrorIs(t, err, assert.AnError)
}

func TestOverview(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	end := fixedNow.Truncate(time.Hour)
	start := end.Add(-23 * time.Hour)
	wantBucketHour := start.Add(5 * time.Hour)
	repo.EXPECT().StatusCounts(gomock.Any(), gomock.Any()).Return(map[domain.Status]int{domain.StatusSent: 40}, nil)
	repo.EXPECT().HourlySeries(gomock.Any(), start).Return([]domain.Bucket{
		{Hour: wantBucketHour, Counts: map[domain.Status]int{domain.StatusSent: 3, domain.StatusFailed: 1}},
	}, nil)

	ov, err := h.Overview(context.Background())
	require.NoError(t, err)
	require.Len(t, ov.Series, overviewHours)
	assert.Equal(t, start, ov.Series[0].Hour)
	for i, bar := range ov.Series {
		if i == 5 {
			assert.Equal(t, 3, bar.Sent)
			assert.Equal(t, 1, bar.Failed)
			continue
		}
		assert.Zero(t, bar.Sent)
		assert.Zero(t, bar.Failed)
		assert.Zero(t, bar.Rejected)
	}
	assert.Equal(t, 40, ov.Counts[domain.StatusSent])
}

func TestOverviewStatusCountsError(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	repo.EXPECT().StatusCounts(gomock.Any(), gomock.Any()).Return(nil, assert.AnError)
	_, err := h.Overview(context.Background())
	require.ErrorIs(t, err, assert.AnError)
}

func TestOverviewHourlySeriesError(t *testing.T) {
	h, repo := newHandler(t, ContentNone)
	repo.EXPECT().StatusCounts(gomock.Any(), gomock.Any()).Return(map[domain.Status]int{}, nil)
	repo.EXPECT().HourlySeries(gomock.Any(), gomock.Any()).Return(nil, assert.AnError)
	_, err := h.Overview(context.Background())
	require.ErrorIs(t, err, assert.AnError)
}
