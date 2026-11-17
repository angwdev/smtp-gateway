package sqlite

import (
	"strings"
	"time"

	"github.com/angwdev/smtp-gateway/internal/domain/credential"
	"github.com/angwdev/smtp-gateway/internal/domain/message"
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
