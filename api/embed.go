// Package api embeds the OpenAPI description of the HTTP API.
package api

import _ "embed"

// Spec is api/openapi.yaml.
//
//go:embed openapi.yaml
var Spec []byte
