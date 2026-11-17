// Package message holds the use cases for the message log.
package message

import (
	domain "github.com/angwdev/smtp-gateway/internal/domain/message"
)

// ContentPolicy says how much of a message's header block is kept.
type ContentPolicy string

// Policies, from LOG_MESSAGE_CONTENT.
const (
	ContentHeaders ContentPolicy = "headers" // subject and the header block
	ContentSubject ContentPolicy = "subject" // subject only
	ContentNone    ContentPolicy = "none"
)

// RecordCommand describes one relay attempt.
type RecordCommand struct {
	CredentialID *int64
	Username     string
	RemoteIP     string
	MailFrom     string
	RcptTo       []string
	Rejected     []string // recipients the upstream refused
	Size         int64
	Status       domain.Status
	Detail       string
	Header       []byte // raw header block, nil when the attempt ended before DATA
}
