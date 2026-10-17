// Command smtp-gateway is a credential-translating SMTP relay: clients log in
// with per-identity credentials managed in an SSO-protected panel, and mail is
// relayed to the upstream server under a single credential.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/angwdev/smtp-gateway/internal/acme"
	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/proxy"
	"github.com/angwdev/smtp-gateway/internal/store"
	"github.com/angwdev/smtp-gateway/internal/tlsutil"
	"github.com/angwdev/smtp-gateway/internal/web"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(healthcheck())
		case "add-credential":
			os.Exit(addCredential(os.Args[2:]))
		case "version":
			fmt.Println(version)
			return
		case "serve":
		default:
			fmt.Fprintf(os.Stderr, "usage: smtp-gateway [serve | healthcheck | add-credential <username> [sender ...] | version]\n")
			os.Exit(2)
		}
	}
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func setupLogging(level string) {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}

func openStore(cfg *config.Config) (*store.Store, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	return store.Open(filepath.Join(cfg.DataDir, "gateway.db"))
}

func run() error {
	cfg, err := config.Load()
	setupLogging(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("config:\n%w", err)
	}
	slog.Info("starting smtp-gateway", "version", version, "hostname", cfg.Hostname,
		"upstream", cfg.Upstream.Addr(), "upstream_tls", cfg.Upstream.TLSMode)
	if cfg.AllowInsecureAuth {
		slog.Warn("SMTP_ALLOW_INSECURE_AUTH is on: clients may send passwords without TLS")
	}

	st, err := openStore(cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	certs, acmeMgr, err := setupTLS(ctx, cfg)
	if err != nil {
		return err
	}

	up := proxy.NewUpstream(cfg.Upstream)
	gw := proxy.New(cfg, st, up)
	smtpErrs, err := gw.Start(certs.Config())
	if err != nil {
		return err
	}

	webErrs := make(chan error, 1)
	if cfg.WebAddr != "" {
		key := []byte(cfg.SessionSecret)
		if len(key) == 0 {
			// Persisted so sessions survive restarts without extra config.
			hexKey, err := st.Setting("session_key", func() string {
				b := make([]byte, 32)
				_, _ = rand.Read(b)
				return hex.EncodeToString(b)
			})
			if err != nil {
				return err
			}
			key, _ = hex.DecodeString(hexKey)
		}
		if len(cfg.OIDCAllowedEmails) == 0 && len(cfg.OIDCAllowedGroups) == 0 {
			slog.Warn("OIDC_ALLOWED_EMAILS and OIDC_ALLOWED_GROUPS are empty: any user your identity provider authenticates can manage credentials")
		}
		ws, err := web.New(cfg, st, up, certs, acmeMgr, key)
		if err != nil {
			return err
		}
		go func() { webErrs <- ws.ListenAndServe(ctx) }()
	} else {
		slog.Info("panel disabled (WEB_ADDR is empty)")
	}

	go pruneLoop(ctx, st, cfg.LogRetention)

	select {
	case <-ctx.Done():
	case err = <-smtpErrs:
	case err = <-webErrs:
	}
	slog.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	gw.Shutdown(sctx)
	return err
}

// setupTLS picks the certificate source. With ACME, a self-signed
// certificate is served until the first one is issued, so SMTP is never down
// because the CA or DNS provider is.
func setupTLS(ctx context.Context, cfg *config.Config) (*tlsutil.Loader, *acme.Manager, error) {
	switch cfg.TLSSource {
	case "file":
		l, err := tlsutil.NewFile(cfg.TLSCertFile, cfg.TLSKeyFile)
		return l, nil, err
	case "acme":
		l, err := tlsutil.NewSelfSigned(cfg.DataDir, cfg.Hostname)
		if err != nil {
			return nil, nil, err
		}
		m, err := acme.New(cfg.ACME, cfg.DataDir, l)
		if err != nil {
			return nil, nil, err
		}
		slog.Info("acme enabled", "domains", strings.Join(cfg.ACME.Domains, ","), "dns_provider", cfg.ACME.DNSProvider, "ca", cfg.ACME.CAURL)
		go m.Run(ctx)
		return l, m, nil
	}
	slog.Warn("serving a self-signed certificate; set TLS_SOURCE=acme or TLS_CERT_FILE for a trusted one", "host", cfg.Hostname)
	l, err := tlsutil.NewSelfSigned(cfg.DataDir, cfg.Hostname)
	return l, nil, err
}

func pruneLoop(ctx context.Context, st *store.Store, retention time.Duration) {
	if retention <= 0 {
		return
	}
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if n, err := st.Prune(ctx, time.Now().Add(-retention)); err != nil {
			slog.Error("prune", "err", err)
		} else if n > 0 {
			slog.Info("pruned message log", "rows", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// healthcheck is used by the container HEALTHCHECK (the image has no shell or
// curl): it checks the panel's /healthz, or the SMTP port if the panel is off.
func healthcheck() int {
	webAddr := os.Getenv("WEB_ADDR")
	if _, set := os.LookupEnv("WEB_ADDR"); !set {
		webAddr = ":8080"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	if webAddr != "" {
		resp, err := client.Get("http://" + localAddr(webAddr) + "/healthz")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintln(os.Stderr, "status", resp.Status)
			return 1
		}
		return 0
	}
	addr := os.Getenv("SMTP_SUBMISSION_ADDR")
	if addr == "" {
		addr = ":2587"
	}
	c, err := net.DialTimeout("tcp", localAddr(addr), 3*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	c.Close()
	return 0
}

func localAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

// addCredential creates a credential from the command line (bootstrap or
// scripting) and prints the generated password.
func addCredential(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: smtp-gateway add-credential <username> [allowed-sender ...]")
		return 2
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}
	st, err := openStore(&config.Config{DataDir: dataDir})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer st.Close()
	c := &store.Credential{Username: args[0], AllowedSenders: args[1:], Enabled: true, CreatedBy: "cli"}
	secret, err := st.CreateCredential(context.Background(), c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, store.ErrDuplicate) {
			return 3
		}
		return 1
	}
	_ = st.Audit(context.Background(), "cli", "credential.create", c.Username)
	fmt.Printf("username: %s\npassword: %s\n", c.Username, secret)
	return 0
}
