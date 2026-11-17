// Package message models one relay attempt as recorded in the message log.
package message

import "time"

// Status is the outcome of a relay attempt.
type Status string

// Outcomes: accepted by the upstream, blocked by gateway policy, or failed upstream.
const (
	StatusSent     Status = "sent"
	StatusRejected Status = "rejected"
	StatusFailed   Status = "failed"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool { return s == StatusSent || s == StatusRejected || s == StatusFailed }

// Limits.
const (
	MaxDetail            = 500
	MaxSubject           = 255
	MaxStoredHeaderBytes = 16 << 10
	DefaultLimit         = 50
	MaxLimit             = 200
	MaxQuery             = 100
)

// Message is one row of the message log.
type Message struct {
	ID           int64
	CreatedAt    time.Time
	CredentialID *int64
	Username     string
	RemoteIP     string
	MailFrom     string
	RcptTo       []string
	Size         int64
	Status       Status
	Detail       string
	Subject      string
}

// Headers is the stored header block of a message.
type Headers struct {
	Raw       string
	Truncated bool
}

// Filter selects messages. Zero values mean "no constraint".
type Filter struct {
	CredentialID int64
	Status       Status
	Query        string
	From, To     time.Time
	BeforeID     int64
	Limit        int
}

// Normalized clamps Limit and Query to their bounds.
//
//nolint:gocritic // Filter is part of the public domain API and is passed by value throughout.
func (f Filter) Normalized() Filter {
	if f.Limit <= 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit > MaxLimit {
		f.Limit = MaxLimit
	}
	if len(f.Query) > MaxQuery {
		f.Query = f.Query[:MaxQuery]
	}
	return f
}

// Bucket is the per-status count for one hour.
type Bucket struct {
	Hour   time.Time
	Counts map[Status]int
}
