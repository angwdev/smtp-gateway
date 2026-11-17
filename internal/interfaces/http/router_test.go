package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/config"
)

// TestNewRouterRejectsBadTrustedProxies exercises the one way NewRouter can
// fail: a malformed WEB_TRUSTED_PROXIES entry that gin's SetTrustedProxies
// cannot parse as a CIDR or IP.
func TestNewRouterRejectsBadTrustedProxies(t *testing.T) {
	cfg := &config.Config{TrustedProxies: []string{"not-a-cidr-or-ip"}}
	_, err := NewRouter(Deps{Config: cfg})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-cidr-or-ip")
}
