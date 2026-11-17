// Package app is the composition root: it builds every dependency by hand
// (constructor injection) and runs the servers until the context ends.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	apptoken "github.com/angwdev/smtp-gateway/internal/application/apitoken"
	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	appcred "github.com/angwdev/smtp-gateway/internal/application/credential"
	"github.com/angwdev/smtp-gateway/internal/application/maintenance"
	appmsg "github.com/angwdev/smtp-gateway/internal/application/message"
	appsession "github.com/angwdev/smtp-gateway/internal/application/session"
	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/acme"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/persistence/sqlite"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/tlsutil"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/upstream"
	httpiface "github.com/angwdev/smtp-gateway/internal/interfaces/http"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/handler"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/ui"
	"github.com/angwdev/smtp-gateway/internal/interfaces/limiter"
	smtpiface "github.com/angwdev/smtp-gateway/internal/interfaces/smtp"
)

// Per-address limits for the panel (section 2.4 of the plan).
const (
	apiRate, apiBurst   = rate.Limit(20), 60
	authRate, authBurst = rate.Limit(10.0 / 60), 10
)

// Run builds the application and serves until ctx is cancelled or a server
// fails. Every goroutine it starts is owned by one errgroup, so all of them
// stop together and Run returns the first real error.
func Run(ctx context.Context, cfg *config.Config, version string) error {
	log := slog.Default()
	log.Info("starting smtp-gateway", "version", version, "hostname", cfg.Hostname,
		"upstream", cfg.Upstream.Addr(), "upstream_tls", cfg.Upstream.TLSMode)
	if cfg.AllowInsecureAuth {
		log.Warn("SMTP_ALLOW_INSECURE_AUTH is on: clients may send passwords without TLS")
	}

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	db, err := sqlite.Open(ctx, filepath.Join(cfg.DataDir, "gateway.db"), log)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("close database", "err", err)
		}
	}()

	// Infrastructure.
	credRepo, msgRepo, auditRepo := sqlite.NewCredentialRepository(db), sqlite.NewMessageRepository(db), sqlite.NewAuditRepository(db)
	tokenRepo, sessionRepo, settings := sqlite.NewAPITokenRepository(db), sqlite.NewSessionRepository(db), sqlite.NewSettingsRepository(db)
	up := upstream.New(cfg.Upstream)

	// Application.
	recorder := appaudit.NewRecorder(auditRepo, log)
	creds := appcred.NewHandler(credRepo, recorder)
	msgs := appmsg.NewHandler(msgRepo, appmsg.ContentPolicy(cfg.MessageContent))
	tokens := apptoken.NewHandler(tokenRepo, recorder)
	sessions := appsession.NewHandler(sessionRepo, cfg.SessionTTL)

	g, gctx := errgroup.WithContext(ctx)

	certs, acmeMgr, err := setupTLS(cfg, log)
	if err != nil {
		return err
	}
	if acmeMgr != nil {
		g.Go(func() error { acmeMgr.Run(gctx); return nil })
	}

	gateway := smtpiface.New(cfg, creds, msgs, recorder, up)
	g.Go(func() error { return gateway.Run(gctx, certs.Config()) })

	if cfg.WebAddr != "" {
		router, limiters, err := buildPanel(gctx, cfg, version, log, db, settings, recorder, creds, msgs, tokens, sessions, auditRepo, up, certs, acmeMgr)
		if err != nil {
			return err
		}
		for _, l := range limiters {
			g.Go(func() error { l.Run(gctx); return nil })
		}
		g.Go(func() error { return httpiface.Serve(gctx, cfg.WebAddr, router, log) })
	} else {
		log.Info("panel disabled (WEB_ADDR is empty)")
	}

	maint := &maintenance.Service{Messages: msgRepo, Audit: auditRepo, Sessions: sessionRepo, DB: db,
		Drops: recorder, Retention: cfg.LogRetention, Log: log}
	g.Go(func() error { maint.Run(gctx); return nil })

	err = g.Wait()
	log.Info("stopped")
	return err
}

// buildPanel wires the HTTP interface and returns the limiters whose sweep
// loops the caller must run.
func buildPanel(ctx context.Context, cfg *config.Config, version string, log *slog.Logger, db *sqlite.DB,
	settings *sqlite.SettingsRepository, recorder *appaudit.Recorder, creds *appcred.Handler, msgs *appmsg.Handler,
	tokens *apptoken.Handler, sessions *appsession.Handler, auditRepo *sqlite.AuditRepository,
	up *upstream.Client, certs *tlsutil.Loader, acmeMgr *acme.Manager,
) (http.Handler, []*limiter.PerIP, error) {
	key := []byte(cfg.SessionSecret)
	if len(key) == 0 {
		// Kept in the database so the login-state cookie survives restarts.
		hexKey, err := settings.GetOrCreate(ctx, "session_key", func() (string, error) {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				return "", fmt.Errorf("read random bytes: %w", err)
			}
			return hex.EncodeToString(b), nil
		})
		if err != nil {
			return nil, nil, err
		}
		if key, err = hex.DecodeString(hexKey); err != nil {
			return nil, nil, fmt.Errorf("decode session key: %w", err)
		}
	}
	if len(cfg.OIDCAllowedEmails) == 0 && len(cfg.OIDCAllowedGroups) == 0 {
		log.Warn("OIDC_ALLOWED_EMAILS and OIDC_ALLOWED_GROUPS are empty: any user your identity provider authenticates can manage credentials")
	}
	if len(cfg.TrustedProxies) == 0 {
		log.Warn("WEB_TRUSTED_PROXIES is empty: behind a reverse proxy every audit entry will show the proxy's address")
	}

	oidc := httpiface.NewOIDC(cfg, key, sessions, recorder, log)
	go oidc.CheckProvider(ctx) // bounded: CheckProvider has its own 15-second timeout

	apiLimiter, authLimiter := limiter.NewPerIP(apiRate, apiBurst), limiter.NewPerIP(authRate, authBurst)
	router, err := httpiface.NewRouter(httpiface.Deps{
		Config: cfg, Log: log, UI: ui.FS(), OIDC: oidc, Sessions: sessions, Tokens: tokens, Recorder: recorder,
		Fails: limiter.NewFailLimiter(cfg.AuthFailLimit, cfg.AuthFailWindow), APILimiter: apiLimiter, AuthLimiter: authLimiter,
		Credentials: handler.NewCredentialHandler(creds),
		Messages:    handler.NewMessageHandler(msgs),
		Audit:       handler.NewAuditHandler(appaudit.NewQueries(auditRepo)),
		TokenAPI:    handler.NewTokenHandler(tokens),
		Session:     handler.NewSessionHandler(),
		Overview:    handler.NewOverviewHandler(msgs, creds, up, certs, acmeMgr, cfg, version),
		System:      handler.NewSystemHandler(up, acmeMgr, cfg, db, recorder, log),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build router: %w", err)
	}
	return router, []*limiter.PerIP{apiLimiter, authLimiter}, nil
}

// setupTLS picks the certificate source. With ACME, a self-signed
// certificate is served until the first one is issued, so SMTP is never down
// because the CA or DNS provider is. Unlike the v1 version this moved from
// (cmd/smtp-gateway/main.go), it does not start the ACME manager's loop
// itself: the caller runs it inside the shared errgroup so every goroutine
// stops together.
func setupTLS(cfg *config.Config, log *slog.Logger) (*tlsutil.Loader, *acme.Manager, error) {
	switch cfg.TLSSource {
	case "file":
		l, err := tlsutil.NewFile(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("tls setup: %w", err)
		}
		return l, nil, nil
	case "acme":
		l, err := tlsutil.NewSelfSigned(cfg.DataDir, cfg.Hostname)
		if err != nil {
			return nil, nil, fmt.Errorf("tls setup: %w", err)
		}
		m, err := acme.New(cfg.ACME, cfg.DataDir, l)
		if err != nil {
			return nil, nil, fmt.Errorf("tls setup: %w", err)
		}
		log.Info("acme enabled", "domains", strings.Join(cfg.ACME.Domains, ","), "dns_provider", cfg.ACME.DNSProvider, "ca", cfg.ACME.CAURL)
		return l, m, nil
	}
	log.Warn("serving a self-signed certificate; set TLS_SOURCE=acme or TLS_CERT_FILE for a trusted one", "host", cfg.Hostname)
	l, err := tlsutil.NewSelfSigned(cfg.DataDir, cfg.Hostname)
	if err != nil {
		return nil, nil, fmt.Errorf("tls setup: %w", err)
	}
	return l, nil, nil
}
