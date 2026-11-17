package message

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "github.com/angwdev/smtp-gateway/internal/domain/message"
)

const overviewHours = 24

// Handler implements the message-log use cases.
type Handler struct {
	repo   domain.Repository
	policy ContentPolicy
	now    func() time.Time
}

// NewHandler returns the use cases with the configured content policy.
func NewHandler(repo domain.Repository, policy ContentPolicy) *Handler {
	return &Handler{repo: repo, policy: policy, now: time.Now}
}

// Record stores one relay attempt.
//
//nolint:gocritic // hugeParam: keep plan's value API
func (h *Handler) Record(ctx context.Context, cmd RecordCommand) error {
	detail := cmd.Detail
	if len(cmd.Rejected) > 0 {
		detail += " (upstream refused: " + strings.Join(cmd.Rejected, ", ") + ")"
	}
	if len(detail) > domain.MaxDetail {
		detail = detail[:domain.MaxDetail]
	}
	m := &domain.Message{
		CreatedAt: h.now(), CredentialID: cmd.CredentialID, Username: cmd.Username, RemoteIP: cmd.RemoteIP,
		MailFrom: cmd.MailFrom, RcptTo: cmd.RcptTo, Size: cmd.Size, Status: cmd.Status, Detail: detail,
	}
	var headers *domain.Headers
	if len(cmd.Header) > 0 && h.policy != ContentNone {
		m.Subject = subjectOf(cmd.Header)
		if h.policy == ContentHeaders {
			headers = storedHeaders(cmd.Header)
		}
	}
	if err := h.repo.Record(ctx, m, headers); err != nil {
		return fmt.Errorf("record message: %w", err)
	}
	return nil
}

// CountSince is the number of messages a credential relayed since a time.
func (h *Handler) CountSince(ctx context.Context, credentialID int64, since time.Time) (int, error) {
	n, err := h.repo.CountSince(ctx, credentialID, since)
	if err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return n, nil
}

// List returns one page and the cursor for the next, or nil at the end.
//
//nolint:gocritic // hugeParam: keep plan's value API
func (h *Handler) List(ctx context.Context, f domain.Filter) ([]*domain.Message, *int64, error) {
	f = f.Normalized()
	list, err := h.repo.List(ctx, f)
	if err != nil {
		return nil, nil, fmt.Errorf("list messages: %w", err)
	}
	if len(list) < f.Limit {
		return list, nil, nil
	}
	next := list[len(list)-1].ID
	return list, &next, nil
}

// Get returns a message with its stored headers.
func (h *Handler) Get(ctx context.Context, id int64) (*Detail, error) {
	m, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	d := &Detail{Message: m}
	headers, err := h.repo.Headers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	if headers != nil {
		d.Raw, d.Truncated, d.Fields = headers.Raw, headers.Truncated, parseFields(headers.Raw)
	}
	return d, nil
}

// Overview returns the last 24 hours: totals and one zero-filled bar per hour.
func (h *Handler) Overview(ctx context.Context) (*Overview, error) {
	end := h.now().Truncate(time.Hour)
	start := end.Add(-(overviewHours - 1) * time.Hour)
	counts, err := h.repo.StatusCounts(ctx, h.now().Add(-overviewHours*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("overview: %w", err)
	}
	buckets, err := h.repo.HourlySeries(ctx, start)
	if err != nil {
		return nil, fmt.Errorf("overview: %w", err)
	}
	byHour := make(map[int64]domain.Bucket, len(buckets))
	for _, b := range buckets {
		byHour[b.Hour.Unix()] = b
	}
	series := make([]HourCount, 0, overviewHours)
	for i := range overviewHours {
		hour := start.Add(time.Duration(i) * time.Hour)
		b := byHour[hour.Unix()]
		series = append(series, HourCount{
			Hour: hour, Sent: b.Counts[domain.StatusSent], Failed: b.Counts[domain.StatusFailed], Rejected: b.Counts[domain.StatusRejected],
		})
	}
	return &Overview{Counts: counts, Series: series}, nil
}
