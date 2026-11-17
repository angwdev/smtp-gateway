package proxy

import (
	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/upstream"
)

// Upstream is kept for the v1 packages until Task 6.2 deletes them.
type Upstream = upstream.Client

// NewUpstream is kept for the v1 packages until Task 6.2 deletes them.
func NewUpstream(cfg config.Upstream) *Upstream { return upstream.New(cfg) }
