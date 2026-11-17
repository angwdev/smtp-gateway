package message

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	domain "github.com/angwdev/smtp-gateway/internal/domain/message"
)

func TestSubjectOf(t *testing.T) {
	tests := []struct {
		name   string
		header []byte
		want   string
	}{
		{"plain subject", []byte("Subject: Weekly report\r\n\r\n"), "Weekly report"},
		{"rfc2047 encoded word decodes", []byte("Subject: =?UTF-8?B?w4FyYm9s?=\r\n\r\n"), "Árbol"},
		{"no subject header gives empty", []byte("From: a@b.c\r\n\r\n"), ""},
		{"garbage input gives empty", []byte("not a header block at all, no colon anywhere"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, subjectOf(tt.header))
		})
	}
}

func TestSubjectOfTruncatesTo255Runes(t *testing.T) {
	long := strings.Repeat("a", 300)
	header := []byte("Subject: " + long + "\r\n\r\n")
	got := subjectOf(header)
	assert.Len(t, []rune(got), domain.MaxSubject)
	assert.Equal(t, strings.Repeat("a", domain.MaxSubject), got)
}

func TestSubjectOfInvalidUTF8BecomesReplacementChar(t *testing.T) {
	header := append([]byte("Subject: "), 0xff, 0xfe)
	header = append(header, []byte("\r\n\r\n")...)
	got := subjectOf(header)
	assert.Contains(t, got, "\uFFFD")
}

func TestStoredHeadersTruncates(t *testing.T) {
	small := []byte("From: a@b.c\r\n\r\n")
	h := storedHeaders(small)
	assert.False(t, h.Truncated)
	assert.Equal(t, string(small), h.Raw)

	big := bytes.Repeat([]byte("a"), domain.MaxStoredHeaderBytes+1000)
	h = storedHeaders(big)
	assert.True(t, h.Truncated)
	assert.Len(t, h.Raw, domain.MaxStoredHeaderBytes)
}

func TestParseFields(t *testing.T) {
	raw := "Received: from a\r\n by b\r\nFrom: a@b.c\r\nFrom: c@d.e\r\n\r\nignored body"
	fields := parseFields(raw)
	assert.Equal(t, []HeaderField{
		{Name: "Received", Value: "from a by b"},
		{Name: "From", Value: "a@b.c"},
		{Name: "From", Value: "c@d.e"},
	}, fields)
}

func TestParseFieldsStopsAtBlankLine(t *testing.T) {
	fields := parseFields("Subject: x\r\n\r\nSubject: should-not-appear")
	assert.Len(t, fields, 1)
	assert.Equal(t, "Subject", fields[0].Name)
}

func TestParseFieldsEmpty(t *testing.T) {
	assert.Empty(t, parseFields(""))
}

func TestParseFieldsSkipsLinesWithoutColon(t *testing.T) {
	fields := parseFields("garbage with no colon\r\nFrom: a@b.c")
	assert.Equal(t, []HeaderField{{Name: "From", Value: "a@b.c"}}, fields)
}
