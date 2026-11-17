//go:build integration

package http

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/angwdev/smtp-gateway/api"
)

// TestEveryRouteIsDocumented keeps api/openapi.yaml honest: every /api/v1
// route the router actually registers must appear in the document, and the
// document must not grow stale entries for routes that no longer exist.
func TestEveryRouteIsDocumented(t *testing.T) {
	e := newEnv(t, nil)
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(api.Spec, &doc))
	re := regexp.MustCompile(`:(\w+)`)
	for _, route := range e.engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/") || route.Path == "/api/v1/openapi.yaml" {
			continue
		}
		p := re.ReplaceAllString(strings.TrimPrefix(route.Path, "/api/v1"), "{$1}")
		ops, ok := doc.Paths[p]
		require.True(t, ok, "path %s is not in api/openapi.yaml", p)
		_, ok = ops[strings.ToLower(route.Method)]
		assert.True(t, ok, "%s %s is not in api/openapi.yaml", route.Method, p)
	}
	assert.Len(t, doc.Paths, 12, "remove paths from the document when routes go away")
}
