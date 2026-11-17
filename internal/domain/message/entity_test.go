package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterNormalized(t *testing.T) {
	tests := []struct {
		name      string
		in        Filter
		wantLimit int
		wantQuery string
	}{
		{"zero limit defaults to 50", Filter{Limit: 0}, 50, ""},
		{"huge limit clamps to 200", Filter{Limit: 500}, 200, ""},
		{"in-range limit stays", Filter{Limit: 7}, 7, ""},
		{"long query is cut to 100", Filter{Query: makeString(150, 'x')}, 50, makeString(100, 'x')},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Normalized()
			assert.Equal(t, tt.wantLimit, got.Limit)
			assert.Equal(t, tt.wantQuery, got.Query)
		})
	}
}

func makeString(n int, b byte) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return string(out)
}

func TestStatusValid(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusSent, true},
		{StatusRejected, true},
		{StatusFailed, true},
		{"", false},
		{"x", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.status.Valid(), "status %q", tt.status)
	}
}
