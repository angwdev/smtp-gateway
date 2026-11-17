package handler

import (
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	appcred "github.com/angwdev/smtp-gateway/internal/application/credential"
	appmsg "github.com/angwdev/smtp-gateway/internal/application/message"
	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/acme"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/tlsutil"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

// UpstreamInfo is what the overview needs from the upstream client.
type UpstreamInfo interface{ Config() config.Upstream }

// OverviewHandler serves GET /api/v1/overview.
type OverviewHandler struct {
	msgs     *appmsg.Handler
	creds    *appcred.Handler
	upstream UpstreamInfo
	certs    *tlsutil.Loader
	acme     *acme.Manager // nil unless TLS_SOURCE=acme
	cfg      *config.Config
	version  string
}

// NewOverviewHandler returns the handler.
func NewOverviewHandler(msgs *appmsg.Handler, creds *appcred.Handler, upstream UpstreamInfo, certs *tlsutil.Loader,
	acmeMgr *acme.Manager, cfg *config.Config, version string,
) *OverviewHandler {
	return &OverviewHandler{msgs: msgs, creds: creds, upstream: upstream, certs: certs, acme: acmeMgr, cfg: cfg, version: version}
}

// hourCountResponse is one bar of the 24-hour chart.
type hourCountResponse struct {
	Hour     time.Time `json:"hour"`
	Sent     int       `json:"sent"`
	Failed   int       `json:"failed"`
	Rejected int       `json:"rejected"`
}

type credentialsSummaryResponse struct {
	Total  int `json:"total"`
	Active int `json:"active"`
}

type upstreamResponse struct {
	Addr         string `json:"addr"`
	TLS          string `json:"tls"`
	Username     string `json:"username"`
	EnvelopeFrom string `json:"envelope_from"`
}

type acmeStatusResponse struct {
	Renewing  bool       `json:"renewing"`
	ARI       bool       `json:"ari"`
	NextCheck *time.Time `json:"next_check"`
	LastError string     `json:"last_error"`
}

type certificateResponse struct {
	Source   string              `json:"source"`
	Label    string              `json:"label"`
	Names    []string            `json:"names"`
	NotAfter *time.Time          `json:"not_after"`
	DaysLeft int                 `json:"days_left"`
	Level    string              `json:"level"`
	ACME     *acmeStatusResponse `json:"acme"`
}

type clientSettingsResponse struct {
	Hostname       string `json:"hostname"`
	SubmissionPort string `json:"submission_port"`
	SMTPSPort      string `json:"smtps_port"`
}

// OverviewResponse is the API view of the overview screen.
type OverviewResponse struct {
	Counts24h      map[string]int             `json:"counts_24h"`
	Series         []hourCountResponse        `json:"series"`
	Credentials    credentialsSummaryResponse `json:"credentials"`
	Upstream       upstreamResponse           `json:"upstream"`
	Certificate    certificateResponse        `json:"certificate"`
	ClientSettings clientSettingsResponse     `json:"client_settings"`
	Version        string                     `json:"version"`
}

// caNames and dnsNames turn an ACME CA directory URL or DNS provider key
// into the words shown in the panel (ported from internal/web/handlers.go).
var caNames = map[string]string{
	"https://acme-v02.api.letsencrypt.org/directory":         "Let's Encrypt",
	"https://acme-staging-v02.api.letsencrypt.org/directory": "Let's Encrypt staging",
	"https://acme.zerossl.com/v2/DV90":                       "ZeroSSL",
	"https://dv.acme-v02.api.pki.goog/directory":             "Google Trust Services",
}

var dnsNames = map[string]string{"cloudflare": "Cloudflare", "route53": "Route 53"}

// certView builds the certificate section (ported from internal/web/handlers.go:46-77).
func (h *OverviewHandler) certView() certificateResponse {
	info := h.certs.Info()
	v := certificateResponse{Source: info.Source, Names: info.Names, NotAfter: utcPtr(&info.NotAfter)}
	v.DaysLeft = int(info.Remaining().Hours() / 24)
	switch {
	case info.Source == tlsutil.SourceSelfSigned:
		v.Label, v.Level = "Self-signed", "warn"
	case v.DaysLeft < 3:
		v.Level = "bad"
	case v.DaysLeft < 14:
		v.Level = "warn"
	default:
		v.Level = "ok"
	}
	if info.Source == tlsutil.SourceFile {
		v.Label = "Certificate file"
	}
	if h.acme != nil {
		st := h.acme.Status()
		ca := caNames[st.CA]
		if ca == "" {
			if u, err := url.Parse(st.CA); err == nil {
				ca = u.Host
			}
		}
		v.Label = ca + " via " + dnsNames[st.Provider] + " DNS"
		if st.LastError != "" && v.Level == "ok" {
			v.Level = "warn"
		}
		var nextCheck *time.Time
		if !st.NextCheck.IsZero() {
			nextCheck = utcPtr(&st.NextCheck)
		}
		v.ACME = &acmeStatusResponse{Renewing: st.Renewing, ARI: st.ARI, NextCheck: nextCheck, LastError: st.LastError}
	}
	return v
}

// Get handles GET /overview.
func (h *OverviewHandler) Get(c *gin.Context) {
	ov, err := h.msgs.Overview(c.Request.Context())
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	creds, err := h.creds.List(c.Request.Context())
	if err != nil {
		middleware.Fail(c, err)
		return
	}
	active := 0
	for _, cred := range creds {
		if cred.Enabled {
			active++
		}
	}
	series := make([]hourCountResponse, 0, len(ov.Series))
	for _, s := range ov.Series {
		series = append(series, hourCountResponse{Hour: s.Hour.UTC(), Sent: s.Sent, Failed: s.Failed, Rejected: s.Rejected})
	}
	uc := h.upstream.Config()
	c.JSON(http.StatusOK, OverviewResponse{
		Counts24h: map[string]int{
			"sent": ov.Counts[message.StatusSent], "failed": ov.Counts[message.StatusFailed], "rejected": ov.Counts[message.StatusRejected],
		},
		Series:      series,
		Credentials: credentialsSummaryResponse{Total: len(creds), Active: active},
		Upstream:    upstreamResponse{Addr: uc.Addr(), TLS: uc.TLSMode, Username: uc.Username, EnvelopeFrom: uc.EnvelopeFrom},
		Certificate: h.certView(),
		ClientSettings: clientSettingsResponse{
			Hostname: h.cfg.Hostname, SubmissionPort: h.cfg.PublicSubmissionPort, SMTPSPort: h.cfg.PublicSMTPSPort,
		},
		Version: h.version,
	})
}
