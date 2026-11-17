package credential

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

func TestSenderAllowed(t *testing.T) {
	cases := []struct {
		patterns []string
		addr     string
		want     bool
	}{
		{nil, "anything@x.test", true},
		{[]string{"*"}, "a@b.test", true},
		{[]string{"a@b.test"}, "A@B.test", true},
		{[]string{"a@b.test"}, "c@b.test", false},
		{[]string{"@b.test"}, "c@b.test", true},
		{[]string{"*@b.test"}, "c@b.test", true},
		{[]string{"@b.test"}, "c@sub.b.test", false},
		{[]string{"@*.b.test"}, "c@sub.b.test", true},
		{[]string{"@*.b.test"}, "c@evilb.test", false},
		{[]string{"@b.test"}, "", false},
		{[]string{"@b.test"}, "no-at-sign", false},
	}
	for _, c := range cases {
		if got := SenderPolicy(c.patterns).Allows(c.addr); got != c.want {
			t.Errorf("SenderPolicy(%v).Allows(%q) = %v, want %v", c.patterns, c.addr, got, c.want)
		}
	}
}

func TestNewSenderPolicy(t *testing.T) {
	manyTooMany := make([]string, 51)
	for i := range manyTooMany {
		manyTooMany[i] = "a@b.c"
	}
	tests := []struct {
		name    string
		in      []string
		want    SenderPolicy
		wantErr bool
	}{
		{"empty means any sender", nil, nil, false},
		{"trims, lowercases, drops blanks", []string{" Alerts@Example.com ", "", "@*.Example.com", "*"}, SenderPolicy{"alerts@example.com", "@*.example.com", "*"}, false},
		{"needs an at sign", []string{"example.com"}, nil, true},
		{"too many", manyTooMany, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSenderPolicy(tt.in)
			if tt.wantErr {
				var ve *shared.ValidationError
				require.ErrorAs(t, err, &ve)
				assert.Equal(t, "allowed_senders", ve.Field)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
