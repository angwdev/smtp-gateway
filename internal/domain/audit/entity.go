// Package audit models the record of who did what, from where.
package audit

import "time"

// ActorType says what kind of principal acted.
type ActorType string

// Actor kinds.
const (
	ActorUser   ActorType = "user"
	ActorToken  ActorType = "token"
	ActorCLI    ActorType = "cli"
	ActorSystem ActorType = "system"
	ActorSMTP   ActorType = "smtp"
)

// Result is the outcome of the audited action.
type Result string

// Outcomes.
const (
	ResultOK     Result = "ok"
	ResultDenied Result = "denied"
	ResultError  Result = "error"
)

// Limits applied before storing.
const (
	MaxDetail    = 1000
	MaxUserAgent = 300
	MaxActor     = 255
	DefaultLimit = 50
	MaxLimit     = 200
)

// Change is the before and after value of one field.
type Change struct {
	From any `json:"from"`
	To   any `json:"to"`
}

// Event is one audit row.
type Event struct {
	ID          int64
	CreatedAt   time.Time
	Actor       string
	ActorType   ActorType
	Action      string
	TargetType  string
	TargetID    *int64
	TargetLabel string
	Result      Result
	IP          string
	UserAgent   string
	RequestID   string
	Detail      string
	Changes     map[string]Change
}

// Filter selects events. Action matches exactly, or as a prefix when it ends
// with a dot ("credential." matches every credential action).
type Filter struct {
	Actor      string
	Action     string
	TargetType string
	Result     Result
	Query      string
	From, To   time.Time
	BeforeID   int64
	Limit      int
}

// Normalized clamps Limit.
//
//nolint:gocritic // Filter is part of the public domain API and is passed by value throughout.
func (f Filter) Normalized() Filter {
	if f.Limit <= 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit > MaxLimit {
		f.Limit = MaxLimit
	}
	return f
}
