package message

import (
	"bufio"
	"bytes"
	"mime"
	"net/textproto"
	"strings"
	"unicode/utf8"

	domain "github.com/angwdev/smtp-gateway/internal/domain/message"
)

// subjectOf extracts and decodes the Subject header.
func subjectOf(header []byte) string {
	h, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(header))).ReadMIMEHeader()
	if err != nil && len(h) == 0 {
		return ""
	}
	subject := h.Get("Subject")
	if decoded, err := new(mime.WordDecoder).DecodeHeader(subject); err == nil {
		subject = decoded
	}
	return truncateRunes(strings.ToValidUTF8(subject, "\uFFFD"), domain.MaxSubject)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// storedHeaders caps the header block and makes it valid UTF-8 for storage.
func storedHeaders(header []byte) *domain.Headers {
	truncated := len(header) > domain.MaxStoredHeaderBytes
	if truncated {
		header = header[:domain.MaxStoredHeaderBytes]
	}
	return &domain.Headers{Raw: strings.ToValidUTF8(string(header), "\uFFFD"), Truncated: truncated}
}

// parseFields splits a header block into fields, keeping order and unfolding
// continuation lines.
func parseFields(raw string) []HeaderField {
	var out []HeaderField
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if line == "" {
			break
		}
		if (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1].Value += " " + strings.TrimSpace(line)
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out = append(out, HeaderField{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	return out
}
