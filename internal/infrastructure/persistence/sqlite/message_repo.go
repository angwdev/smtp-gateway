package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// MessageRepository implements message.Repository.
type MessageRepository struct{ db *DB }

var _ message.Repository = (*MessageRepository)(nil)

// NewMessageRepository returns a repository over db.
func NewMessageRepository(db *DB) *MessageRepository { return &MessageRepository{db: db} }

// likeEscaper makes user input literal inside a LIKE pattern.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Record writes the message, its headers and the sent counter in one
// transaction so the log and the counter can never disagree.
func (r *MessageRepository) Record(ctx context.Context, m *message.Message, h *message.Headers) error {
	row := messageModel{
		CreatedAt: m.CreatedAt.Unix(), CredentialID: m.CredentialID, Username: m.Username,
		RemoteIP: m.RemoteIP, MailFrom: m.MailFrom, RcptTo: strings.Join(m.RcptTo, rcptSeparator),
		Size: m.Size, Status: string(m.Status), Detail: m.Detail, Subject: m.Subject,
	}
	err := r.db.Write.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
		if h != nil && h.Raw != "" {
			hdr := messageHeaderModel{MessageID: row.ID, Raw: h.Raw, Truncated: h.Truncated}
			if err := tx.Create(&hdr).Error; err != nil {
				return fmt.Errorf("insert message headers: %w", err)
			}
		}
		if m.Status == message.StatusSent && m.CredentialID != nil {
			err := tx.Model(&credentialModel{}).Where("id = ?", *m.CredentialID).
				UpdateColumn("sent_count", gorm.Expr("sent_count + 1")).Error
			if err != nil {
				return fmt.Errorf("bump sent count: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("record message: %w", err)
	}
	m.ID = row.ID
	return nil
}

// FindByID returns one message.
func (r *MessageRepository) FindByID(ctx context.Context, id int64) (*message.Message, error) {
	var row messageModel
	if err := r.db.Read.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("find message %d: %w", id, shared.ErrNotFound)
		}
		return nil, fmt.Errorf("find message %d: %w", id, err)
	}
	return row.toDomain(), nil
}

// Headers returns the stored header block, or nil when there is none.
func (r *MessageRepository) Headers(ctx context.Context, id int64) (*message.Headers, error) {
	var row messageHeaderModel
	if err := r.db.Read.WithContext(ctx).Where("message_id = ?", id).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // "no headers stored" is a normal answer, not an error
		}
		return nil, fmt.Errorf("find headers of message %d: %w", id, err)
	}
	return &message.Headers{Raw: row.Raw, Truncated: row.Truncated}, nil
}

// List returns messages newest first. Paging is by id (keyset), so the cost
// of a page does not grow with its position.
//
//nolint:gocritic // message.Filter is part of the public domain API and is passed by value.
func (r *MessageRepository) List(ctx context.Context, f message.Filter) ([]*message.Message, error) {
	f = f.Normalized()
	q := r.db.Read.WithContext(ctx).Model(&messageModel{})
	if f.CredentialID > 0 {
		q = q.Where("credential_id = ?", f.CredentialID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", string(f.Status))
	}
	if !f.From.IsZero() {
		q = q.Where("created_at >= ?", f.From.Unix())
	}
	if !f.To.IsZero() {
		q = q.Where("created_at <= ?", f.To.Unix())
	}
	if f.BeforeID > 0 {
		q = q.Where("id < ?", f.BeforeID)
	}
	if f.Query != "" {
		like := "%" + likeEscaper.Replace(f.Query) + "%"
		q = q.Where(`(mail_from LIKE ? ESCAPE '\' OR rcpt_to LIKE ? ESCAPE '\' OR username LIKE ? ESCAPE '\'`+
			` OR subject LIKE ? ESCAPE '\' OR detail LIKE ? ESCAPE '\')`, like, like, like, like, like)
	}
	var rows []messageModel
	if err := q.Order("id DESC").Limit(f.Limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	out := make([]*message.Message, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDomain())
	}
	return out, nil
}

// CountSince counts the messages that reached the upstream.
func (r *MessageRepository) CountSince(ctx context.Context, credentialID int64, since time.Time) (int, error) {
	var n int64
	err := r.db.Read.WithContext(ctx).Model(&messageModel{}).
		Where("credential_id = ? AND created_at >= ? AND status IN ?", credentialID, since.Unix(),
			[]string{string(message.StatusSent), string(message.StatusFailed)}).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("count messages of credential %d: %w", credentialID, err)
	}
	return int(n), nil
}

type statusCount struct {
	Hour   int64
	Status string
	N      int
}

// StatusCounts returns the number of messages per status since a time.
func (r *MessageRepository) StatusCounts(ctx context.Context, since time.Time) (map[message.Status]int, error) {
	var rows []statusCount
	err := r.db.Read.WithContext(ctx).
		Raw(`SELECT status, COUNT(*) AS n FROM messages WHERE created_at >= ? GROUP BY status`, since.Unix()).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count messages by status: %w", err)
	}
	out := make(map[message.Status]int, len(rows))
	for _, row := range rows {
		out[message.Status(row.Status)] = row.N
	}
	return out, nil
}

// HourlySeries groups messages by hour and status.
func (r *MessageRepository) HourlySeries(ctx context.Context, since time.Time) ([]message.Bucket, error) {
	var rows []statusCount
	err := r.db.Read.WithContext(ctx).
		Raw(`SELECT (created_at / 3600) * 3600 AS hour, status, COUNT(*) AS n FROM messages
			WHERE created_at >= ? GROUP BY hour, status ORDER BY hour`, since.Unix()).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("hourly message series: %w", err)
	}
	var out []message.Bucket
	for _, row := range rows {
		if len(out) == 0 || out[len(out)-1].Hour.Unix() != row.Hour {
			out = append(out, message.Bucket{Hour: time.Unix(row.Hour, 0), Counts: map[message.Status]int{}})
		}
		out[len(out)-1].Counts[message.Status(row.Status)] = row.N
	}
	return out, nil
}

// Prune deletes old messages a batch at a time so the single writer is never
// held for long. Headers go with their message (ON DELETE CASCADE).
func (r *MessageRepository) Prune(ctx context.Context, before time.Time, batch int) (int64, error) {
	var total int64
	for {
		res := r.db.Write.WithContext(ctx).Exec(
			`DELETE FROM messages WHERE id IN (SELECT id FROM messages WHERE created_at < ? LIMIT ?)`,
			before.Unix(), batch)
		if res.Error != nil {
			return total, fmt.Errorf("prune messages: %w", res.Error)
		}
		total += res.RowsAffected
		if res.RowsAffected < int64(batch) {
			return total, nil
		}
	}
}
