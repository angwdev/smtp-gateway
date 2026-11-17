package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/api"
	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/acme"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

// Prober is what the system handler needs from the upstream client.
type Prober interface {
	Probe(ctx context.Context) (time.Duration, error)
	Config() config.Upstream
}

// Pinger is what the system handler needs from the database.
type Pinger interface {
	Ping(ctx context.Context) error
}

// SystemHandler serves /healthz, /upstream/test, /certificate/renew and
// /openapi.yaml.
type SystemHandler struct {
	upstream Prober
	acme     *acme.Manager // nil unless TLS_SOURCE=acme
	cfg      *config.Config
	db       Pinger
	audit    *appaudit.Recorder
	log      *slog.Logger
}

// NewSystemHandler returns the handler.
func NewSystemHandler(upstream Prober, acmeMgr *acme.Manager, cfg *config.Config, db Pinger, auditor *appaudit.Recorder, log *slog.Logger) *SystemHandler {
	return &SystemHandler{upstream: upstream, acme: acmeMgr, cfg: cfg, db: db, audit: auditor, log: log}
}

// Health handles GET /healthz. It never leaks the raw database error to the
// client, only to the log.
func (h *SystemHandler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		h.log.Error("healthz: database ping failed", "err", err)
		c.String(http.StatusServiceUnavailable, "unavailable\n")
		return
	}
	c.String(http.StatusOK, "ok\n")
}

// upstreamTestResponse is the body of POST /upstream/test.
type upstreamTestResponse struct {
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	DurationMS int64  `json:"duration_ms"`
}

// TestUpstream handles POST /upstream/test.
func (h *SystemHandler) TestUpstream(c *gin.Context) {
	d, err := h.upstream.Probe(c.Request.Context())
	resp := upstreamTestResponse{OK: err == nil, DurationMS: d.Milliseconds()}
	result := audit.ResultOK
	if err == nil {
		resp.Message = "Connected and authenticated to " + h.upstream.Config().Addr() + " in " + d.Round(time.Millisecond).String() + "."
	} else {
		resp.Message = err.Error()
		result = audit.ResultError
	}
	h.audit.Record(c.Request.Context(), appaudit.Entry{Action: "upstream.test", TargetType: "upstream", Result: result})
	middleware.MarkAudited(c)
	c.JSON(http.StatusOK, resp)
}

// RenewCertificate handles POST /certificate/renew.
func (h *SystemHandler) RenewCertificate(c *gin.Context) {
	if h.acme == nil {
		middleware.Fail(c, shared.ErrNotFound)
		return
	}
	h.acme.RenewNow()
	h.audit.Record(c.Request.Context(), appaudit.Entry{Action: "certificate.renew", TargetType: "certificate", Detail: strings.Join(h.cfg.ACME.Domains, ",")})
	middleware.MarkAudited(c)
	c.JSON(http.StatusAccepted, gin.H{})
}

// OpenAPI handles GET /openapi.yaml.
func (h *SystemHandler) OpenAPI(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml", api.Spec)
}
