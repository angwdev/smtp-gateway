// Package migrations embeds the Goose SQL migrations.
package migrations

import "embed"

// FS holds every *.sql migration; Goose applies them in filename order.
//
//go:embed *.sql
var FS embed.FS
