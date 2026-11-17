package message

import (
	"time"

	domain "github.com/angwdev/smtp-gateway/internal/domain/message"
)

// HeaderField is one header line, unfolded.
type HeaderField struct {
	Name  string
	Value string
}

// Detail is a message with its stored headers.
type Detail struct {
	Message   *domain.Message
	Fields    []HeaderField
	Raw       string
	Truncated bool
}

// HourCount is one bar of the 24-hour chart.
type HourCount struct {
	Hour                   time.Time
	Sent, Failed, Rejected int
}

// Overview is the message part of the overview screen.
type Overview struct {
	Counts map[domain.Status]int
	Series []HourCount // always 24 entries, oldest first
}
