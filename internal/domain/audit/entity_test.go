package audit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterNormalized(t *testing.T) {
	longQuery := strings.Repeat("q", 150)
	tests := []struct {
		name  string
		in    Filter
		limit int
		query string
	}{
		{"zero limit defaults", Filter{Limit: 0}, DefaultLimit, ""},
		{"negative limit defaults", Filter{Limit: -1}, DefaultLimit, ""},
		{"limit above max is clamped", Filter{Limit: 500}, MaxLimit, ""},
		{"limit in range is unchanged", Filter{Limit: 75}, 75, ""},
		{"limit at the max boundary is unchanged", Filter{Limit: MaxLimit}, MaxLimit, ""},
		{"a long query is passed through unchanged", Filter{Limit: 10, Query: longQuery}, 10, longQuery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Normalized()
			assert.Equal(t, tt.limit, got.Limit)
			assert.Equal(t, tt.query, got.Query)
		})
	}
}
