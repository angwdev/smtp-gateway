package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/angwdev/smtp-gateway/internal/domain/audit"
)

// AuditRepository implements audit.Repository.
type AuditRepository struct{ db *DB }

var _ audit.Repository = (*AuditRepository)(nil)

// NewAuditRepository returns a repository over db.
func NewAuditRepository(db *DB) *AuditRepository { return &AuditRepository{db: db} }

func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}

// Append writes e and fills e.ID. Oversized fields are truncated rather than
// rejected, since an audit write must never block the action it records.
func (r *AuditRepository) Append(ctx context.Context, e *audit.Event) error {
	actorType := e.ActorType
	if actorType == "" {
		actorType = audit.ActorSystem
	}
	result := e.Result
	if result == "" {
		result = audit.ResultOK
	}
	var changes string
	if len(e.Changes) > 0 {
		b, err := json.Marshal(e.Changes)
		if err != nil {
			return fmt.Errorf("append audit event: %w", err)
		}
		changes = string(b)
	}
	m := auditEventModel{
		CreatedAt: e.CreatedAt.Unix(), Actor: truncate(e.Actor, audit.MaxActor), ActorType: string(actorType),
		Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, TargetLabel: e.TargetLabel,
		Result: string(result), IP: e.IP, UserAgent: truncate(e.UserAgent, audit.MaxUserAgent),
		RequestID: e.RequestID, Detail: truncate(e.Detail, audit.MaxDetail), Changes: changes,
	}
	if err := r.db.Write.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	e.ID = m.ID
	return nil
}

// List returns events newest first.
//
//nolint:gocritic // audit.Filter is part of the public domain API and is passed by value.
func (r *AuditRepository) List(ctx context.Context, f audit.Filter) ([]*audit.Event, error) {
	f = f.Normalized()
	q := r.db.Read.WithContext(ctx).Model(&auditEventModel{})
	if f.Actor != "" {
		q = q.Where("actor = ?", f.Actor)
	}
	switch {
	case f.Action == "":
		// no constraint
	case strings.HasSuffix(f.Action, "."):
		q = q.Where("action LIKE ? ESCAPE '\\'", likeEscaper.Replace(f.Action)+"%")
	default:
		q = q.Where("action = ?", f.Action)
	}
	if f.TargetType != "" {
		q = q.Where("target_type = ?", f.TargetType)
	}
	if f.Result != "" {
		q = q.Where("result = ?", string(f.Result))
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
		q = q.Where(`(actor LIKE ? ESCAPE '\' OR action LIKE ? ESCAPE '\' OR target_label LIKE ? ESCAPE '\'`+
			` OR detail LIKE ? ESCAPE '\' OR ip LIKE ? ESCAPE '\')`, like, like, like, like, like)
	}
	var rows []auditEventModel
	if err := q.Order("id DESC").Limit(f.Limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	out := make([]*audit.Event, 0, len(rows))
	for i := range rows {
		e, err := rows[i].toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// Prune deletes events older than before.
func (r *AuditRepository) Prune(ctx context.Context, before time.Time) (int64, error) {
	res := r.db.Write.WithContext(ctx).Where("created_at < ?", before.Unix()).Delete(&auditEventModel{})
	if res.Error != nil {
		return 0, fmt.Errorf("prune audit events: %w", res.Error)
	}
	return res.RowsAffected, nil
}
