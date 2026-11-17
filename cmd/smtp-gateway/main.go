// Command smtp-gateway is a credential-translating SMTP relay: clients log in
// with per-identity credentials managed in an SSO-protected panel or through
// the HTTP API, and mail is relayed to the upstream server under a single
// credential.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/angwdev/smtp-gateway/internal/app"
	"github.com/angwdev/smtp-gateway/internal/config"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(app.Healthcheck())
		case "add-credential":
			dataDir := os.Getenv("DATA_DIR")
			if dataDir == "" {
				dataDir = "/data"
			}
			os.Exit(app.AddCredential(os.Stdout, os.Stderr, dataDir, os.Args[2:]))
		case "version":
			fmt.Println(version)
			return
		case "serve":
		default:
			fmt.Fprintln(os.Stderr, "usage: smtp-gateway [serve | healthcheck | add-credential <username> [sender ...] | version]")
			os.Exit(2)
		}
	}
	if err := serve(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func serve() error {
	cfg, err := config.Load()
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.LogLevel)) // an unknown level keeps the default, info
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	if err != nil {
		return fmt.Errorf("config:\n%w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return app.Run(ctx, cfg, version)
}
