package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/credential"
	"github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/domain/session"
)

// Timestamps are unix seconds, as in v1. The helpers keep the conversion in
// one place.
func unixPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.Unix()
	return &v
}

func timePtr(v *int64) *time.Time {
	if v == nil {
		return nil
	}
	t := time.Unix(*v, 0)
	return &t
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

type credentialModel struct {
	ID             int64  `gorm:"column:id;primaryKey"`
	Username       string `gorm:"column:username"`
	SecretHash     string `gorm:"column:secret_hash"`
	Description    string `gorm:"column:description"`
	AllowedSenders string `gorm:"column:allowed_senders"`
	RatePerHour    int    `gorm:"column:rate_per_hour"`
	Enabled        bool   `gorm:"column:enabled"`
	CreatedAt      int64  `gorm:"column:created_at;autoCreateTime:false"`
	CreatedBy      string `gorm:"column:created_by"`
	UpdatedAt      int64  `gorm:"column:updated_at;autoUpdateTime:false"`
	LastUsedAt     *int64 `gorm:"column:last_used_at"`
	LastUsedIP     string `gorm:"column:last_used_ip"`
	SentCount      int64  `gorm:"column:sent_count"`
}

// TableName overrides GORM's pluralisation guess.
func (credentialModel) TableName() string { return "credentials" }

func (m *credentialModel) toDomain() *credential.Credential {
	return &credential.Credential{
		ID:             m.ID,
		Username:       m.Username,
		Description:    m.Description,
		AllowedSenders: credential.SenderPolicy(splitLines(m.AllowedSenders)),
		RatePerHour:    m.RatePerHour,
		Enabled:        m.Enabled,
		SecretHash:     m.SecretHash,
		CreatedAt:      time.Unix(m.CreatedAt, 0),
		CreatedBy:      m.CreatedBy,
		UpdatedAt:      time.Unix(m.UpdatedAt, 0),
		LastUsedAt:     timePtr(m.LastUsedAt),
		LastUsedIP:     m.LastUsedIP,
		SentCount:      m.SentCount,
	}
}

func credentialModelFrom(c *credential.Credential) *credentialModel {
	return &credentialModel{
		ID:             c.ID,
		Username:       c.Username,
		SecretHash:     c.SecretHash,
		Description:    c.Description,
		AllowedSenders: strings.Join(c.AllowedSenders, "\n"),
		RatePerHour:    c.RatePerHour,
		Enabled:        c.Enabled,
		CreatedAt:      c.CreatedAt.Unix(),
		CreatedBy:      c.CreatedBy,
		UpdatedAt:      c.UpdatedAt.Unix(),
		LastUsedAt:     unixPtr(c.LastUsedAt),
		LastUsedIP:     c.LastUsedIP,
		SentCount:      c.SentCount,
	}
}

type messageModel struct {
	ID           int64  `gorm:"column:id;primaryKey"`
	CreatedAt    int64  `gorm:"column:created_at;autoCreateTime:false"`
	CredentialID *int64 `gorm:"column:credential_id"`
	Username     string `gorm:"column:username"`
	RemoteIP     string `gorm:"column:remote_ip"`
	MailFrom     string `gorm:"column:mail_from"`
	RcptTo       string `gorm:"column:rcpt_to"`
	Size         int64  `gorm:"column:size"`
	Status       string `gorm:"column:status"`
	Detail       string `gorm:"column:detail"`
	Subject      string `gorm:"column:subject"`
}

// TableName overrides GORM's pluralisation guess.
func (messageModel) TableName() string { return "messages" }

// rcptSeparator is the v1 storage format of the recipient list.
const rcptSeparator = ", "

func (m *messageModel) toDomain() *message.Message {
	out := &message.Message{
		ID: m.ID, CreatedAt: time.Unix(m.CreatedAt, 0), CredentialID: m.CredentialID,
		Username: m.Username, RemoteIP: m.RemoteIP, MailFrom: m.MailFrom, Size: m.Size,
		Status: message.Status(m.Status), Detail: m.Detail, Subject: m.Subject,
	}
	if m.RcptTo != "" {
		out.RcptTo = strings.Split(m.RcptTo, rcptSeparator)
	}
	return out
}

type messageHeaderModel struct {
	MessageID int64  `gorm:"column:message_id;primaryKey"`
	Raw       string `gorm:"column:raw"`
	Truncated bool   `gorm:"column:truncated"`
}

// TableName overrides GORM's pluralisation guess.
func (messageHeaderModel) TableName() string { return "message_headers" }

type auditEventModel struct {
	ID          int64  `gorm:"column:id;primaryKey"`
	CreatedAt   int64  `gorm:"column:created_at;autoCreateTime:false"`
	Actor       string `gorm:"column:actor"`
	ActorType   string `gorm:"column:actor_type"`
	Action      string `gorm:"column:action"`
	TargetType  string `gorm:"column:target_type"`
	TargetID    *int64 `gorm:"column:target_id"`
	TargetLabel string `gorm:"column:target_label"`
	Result      string `gorm:"column:result"`
	IP          string `gorm:"column:ip"`
	UserAgent   string `gorm:"column:user_agent"`
	RequestID   string `gorm:"column:request_id"`
	Detail      string `gorm:"column:detail"`
	Changes     string `gorm:"column:changes"`
}

// TableName overrides GORM's pluralisation guess.
func (auditEventModel) TableName() string { return "audit_events" }

func (m *auditEventModel) toDomain() (*audit.Event, error) {
	out := &audit.Event{
		ID: m.ID, CreatedAt: time.Unix(m.CreatedAt, 0), Actor: m.Actor, ActorType: audit.ActorType(m.ActorType),
		Action: m.Action, TargetType: m.TargetType, TargetID: m.TargetID, TargetLabel: m.TargetLabel,
		Result: audit.Result(m.Result), IP: m.IP, UserAgent: m.UserAgent, RequestID: m.RequestID, Detail: m.Detail,
	}
	if m.Changes != "" {
		if err := json.Unmarshal([]byte(m.Changes), &out.Changes); err != nil {
			return nil, fmt.Errorf("decode audit changes of event %d: %w", m.ID, err)
		}
	}
	return out, nil
}

type apiTokenModel struct {
	ID         int64  `gorm:"column:id;primaryKey"`
	Name       string `gorm:"column:name"`
	TokenHash  string `gorm:"column:token_hash"`
	Hint       string `gorm:"column:hint"`
	Scope      string `gorm:"column:scope"`
	CreatedAt  int64  `gorm:"column:created_at;autoCreateTime:false"`
	CreatedBy  string `gorm:"column:created_by"`
	ExpiresAt  *int64 `gorm:"column:expires_at"`
	LastUsedAt *int64 `gorm:"column:last_used_at"`
	LastUsedIP string `gorm:"column:last_used_ip"`
	RevokedAt  *int64 `gorm:"column:revoked_at"`
}

// TableName overrides GORM's pluralisation guess.
func (apiTokenModel) TableName() string { return "api_tokens" }

func (m *apiTokenModel) toDomain() *apitoken.Token {
	return &apitoken.Token{
		ID: m.ID, Name: m.Name, Hash: m.TokenHash, Hint: m.Hint, Scope: apitoken.Scope(m.Scope),
		CreatedAt: time.Unix(m.CreatedAt, 0), CreatedBy: m.CreatedBy, ExpiresAt: timePtr(m.ExpiresAt),
		LastUsedAt: timePtr(m.LastUsedAt), LastUsedIP: m.LastUsedIP, RevokedAt: timePtr(m.RevokedAt),
	}
}

func apiTokenModelFrom(t *apitoken.Token) *apiTokenModel {
	return &apiTokenModel{
		ID: t.ID, Name: t.Name, TokenHash: t.Hash, Hint: t.Hint, Scope: string(t.Scope),
		CreatedAt: t.CreatedAt.Unix(), CreatedBy: t.CreatedBy, ExpiresAt: unixPtr(t.ExpiresAt),
		LastUsedAt: unixPtr(t.LastUsedAt), LastUsedIP: t.LastUsedIP, RevokedAt: unixPtr(t.RevokedAt),
	}
}

type sessionModel struct {
	IDHash    string `gorm:"column:id_hash;primaryKey"`
	Email     string `gorm:"column:email"`
	Name      string `gorm:"column:name"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:false"`
	ExpiresAt int64  `gorm:"column:expires_at"`
	IP        string `gorm:"column:ip"`
	UserAgent string `gorm:"column:user_agent"`
}

// TableName overrides GORM's pluralisation guess.
func (sessionModel) TableName() string { return "sessions" }

func (m *sessionModel) toDomain() *session.Session {
	return &session.Session{
		IDHash: m.IDHash, Email: m.Email, Name: m.Name, CreatedAt: time.Unix(m.CreatedAt, 0),
		ExpiresAt: time.Unix(m.ExpiresAt, 0), IP: m.IP, UserAgent: m.UserAgent,
	}
}

func sessionModelFrom(s *session.Session) *sessionModel {
	return &sessionModel{
		IDHash: s.IDHash, Email: s.Email, Name: s.Name, CreatedAt: s.CreatedAt.Unix(),
		ExpiresAt: s.ExpiresAt.Unix(), IP: s.IP, UserAgent: s.UserAgent,
	}
}

type settingModel struct {
	Key   string `gorm:"column:key;primaryKey"`
	Value string `gorm:"column:value"`
}

// TableName overrides GORM's pluralisation guess.
func (settingModel) TableName() string { return "settings" }
