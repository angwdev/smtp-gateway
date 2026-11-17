// Package handler holds the Gin handlers of the JSON API.
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	appmsg "github.com/angwdev/smtp-gateway/internal/application/message"
	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/credential"
	"github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// CredentialResponse is the API view of a credential. The secret hash is
// deliberately absent.
type CredentialResponse struct {
	ID             int64      `json:"id"`
	Username       string     `json:"username"`
	Description    string     `json:"description"`
	AllowedSenders []string   `json:"allowed_senders"`
	RatePerHour    int        `json:"rate_per_hour"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	CreatedBy      string     `json:"created_by"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	LastUsedIP     string     `json:"last_used_ip"`
	SentCount      int64      `json:"sent_count"`
}

func utc(t time.Time) time.Time { return t.UTC() }

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func fromCredential(c *credential.Credential) CredentialResponse {
	senders := []string(c.AllowedSenders)
	if senders == nil {
		senders = []string{}
	}
	return CredentialResponse{
		ID: c.ID, Username: c.Username, Description: c.Description, AllowedSenders: senders,
		RatePerHour: c.RatePerHour, Enabled: c.Enabled, CreatedAt: utc(c.CreatedAt), CreatedBy: c.CreatedBy,
		UpdatedAt: utc(c.UpdatedAt), LastUsedAt: utcPtr(c.LastUsedAt), LastUsedIP: c.LastUsedIP, SentCount: c.SentCount,
	}
}

// MessageResponse is the API view of one message-log row.
type MessageResponse struct {
	ID           int64     `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	CredentialID *int64    `json:"credential_id"`
	Username     string    `json:"username"`
	RemoteIP     string    `json:"remote_ip"`
	MailFrom     string    `json:"mail_from"`
	RcptTo       []string  `json:"rcpt_to"`
	Size         int64     `json:"size"`
	Status       string    `json:"status"`
	Detail       string    `json:"detail"`
	Subject      string    `json:"subject"`
}

func fromMessage(m *message.Message) MessageResponse {
	rcpts := m.RcptTo
	if rcpts == nil {
		rcpts = []string{}
	}
	return MessageResponse{
		ID: m.ID, CreatedAt: utc(m.CreatedAt), CredentialID: m.CredentialID, Username: m.Username,
		RemoteIP: m.RemoteIP, MailFrom: m.MailFrom, RcptTo: rcpts, Size: m.Size,
		Status: string(m.Status), Detail: m.Detail, Subject: m.Subject,
	}
}

// HeaderFieldResponse is one unfolded header line.
type HeaderFieldResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MessageDetailResponse is a message plus its stored headers.
type MessageDetailResponse struct {
	MessageResponse
	Headers          []HeaderFieldResponse `json:"headers"`
	HeadersRaw       string                `json:"headers_raw"`
	HeadersTruncated bool                  `json:"headers_truncated"`
}

func fromMessageDetail(d *appmsg.Detail) MessageDetailResponse {
	fields := make([]HeaderFieldResponse, 0, len(d.Fields))
	for _, f := range d.Fields {
		fields = append(fields, HeaderFieldResponse{Name: f.Name, Value: f.Value})
	}
	return MessageDetailResponse{
		MessageResponse: fromMessage(d.Message), Headers: fields, HeadersRaw: d.Raw, HeadersTruncated: d.Truncated,
	}
}

// AuditChangeResponse is the before/after value of one changed field.
type AuditChangeResponse struct {
	From any `json:"from"`
	To   any `json:"to"`
}

// AuditEventResponse is the API view of one audit row.
type AuditEventResponse struct {
	ID          int64                          `json:"id"`
	CreatedAt   time.Time                      `json:"created_at"`
	Actor       string                         `json:"actor"`
	ActorType   string                         `json:"actor_type"`
	Action      string                         `json:"action"`
	TargetType  string                         `json:"target_type"`
	TargetID    *int64                         `json:"target_id"`
	TargetLabel string                         `json:"target_label"`
	Result      string                         `json:"result"`
	IP          string                         `json:"ip"`
	UserAgent   string                         `json:"user_agent"`
	RequestID   string                         `json:"request_id"`
	Detail      string                         `json:"detail"`
	Changes     map[string]AuditChangeResponse `json:"changes"`
}

func fromAuditEvent(e *audit.Event) AuditEventResponse {
	changes := make(map[string]AuditChangeResponse, len(e.Changes))
	for k, v := range e.Changes {
		changes[k] = AuditChangeResponse{From: v.From, To: v.To}
	}
	return AuditEventResponse{
		ID: e.ID, CreatedAt: utc(e.CreatedAt), Actor: e.Actor, ActorType: string(e.ActorType), Action: e.Action,
		TargetType: e.TargetType, TargetID: e.TargetID, TargetLabel: e.TargetLabel, Result: string(e.Result),
		IP: e.IP, UserAgent: e.UserAgent, RequestID: e.RequestID, Detail: e.Detail, Changes: changes,
	}
}

// TokenResponse is the API view of an API token. The hash is never sent.
type TokenResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Hint       string     `json:"hint"`
	Scope      string     `json:"scope"`
	CreatedAt  time.Time  `json:"created_at"`
	CreatedBy  string     `json:"created_by"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	LastUsedIP string     `json:"last_used_ip"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

func fromToken(t *apitoken.Token) TokenResponse {
	return TokenResponse{
		ID: t.ID, Name: t.Name, Hint: t.Hint, Scope: string(t.Scope), CreatedAt: utc(t.CreatedAt), CreatedBy: t.CreatedBy,
		ExpiresAt: utcPtr(t.ExpiresAt), LastUsedAt: utcPtr(t.LastUsedAt), LastUsedIP: t.LastUsedIP, RevokedAt: utcPtr(t.RevokedAt),
	}
}

// pathID reads the :id path parameter.
func pathID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, shared.ErrNotFound
	}
	return id, nil
}

// queryTime reads an optional RFC 3339 query parameter.
func queryTime(c *gin.Context, name string) (time.Time, error) {
	raw := c.Query(name)
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, shared.Invalid(name, "Use an RFC 3339 time such as 2026-11-17T00:00:00Z.")
	}
	return t, nil
}

// queryInt reads an optional non-negative integer query parameter.
func queryInt(c *gin.Context, name string) (int64, error) {
	raw := c.Query(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, shared.Invalid(name, "Use a whole number.")
	}
	return n, nil
}
