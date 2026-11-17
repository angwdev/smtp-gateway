package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	appcred "github.com/angwdev/smtp-gateway/internal/application/credential"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/persistence/sqlite"
)

// AddCredential creates a credential from the command line and prints its
// password. Exit codes are the v1 ones: 0 ok, 1 error, 2 usage, 3 duplicate.
func AddCredential(stdout, stderr io.Writer, dataDir string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: smtp-gateway add-credential <username> [allowed-sender ...]") //nolint:errcheck // CLI exiting; nothing useful to do with a write error here
		return 2
	}
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(dataDir, "gateway.db"), slog.New(slog.DiscardHandler))
	if err != nil {
		fmt.Fprintln(stderr, err) //nolint:errcheck // CLI exiting; nothing useful to do with a write error here
		return 1
	}
	defer db.Close() //nolint:errcheck // exiting; nothing useful to do with a close error

	recorder := appaudit.NewRecorder(sqlite.NewAuditRepository(db), slog.Default())
	creds := appcred.NewHandler(sqlite.NewCredentialRepository(db), recorder)
	ctx = appaudit.WithMeta(ctx, appaudit.Meta{Actor: "cli", ActorType: audit.ActorCLI})
	c, secret, err := creds.Create(ctx, appcred.CreateCommand{Username: args[0], AllowedSenders: args[1:], Enabled: true, CreatedBy: "cli"})
	if err != nil {
		fmt.Fprintln(stderr, err) //nolint:errcheck // CLI exiting; nothing useful to do with a write error here
		if errors.Is(err, shared.ErrConflict) {
			return 3
		}
		return 1
	}
	fmt.Fprintf(stdout, "username: %s\npassword: %s\n", c.Username, secret) //nolint:errcheck // CLI exiting; nothing useful to do with a write error here
	return 0
}

// Healthcheck is used by the container HEALTHCHECK (the image has no shell
// or curl): it checks the panel's /healthz, or the SMTP port if the panel
// is off.
func Healthcheck() int {
	webAddr := os.Getenv("WEB_ADDR")
	if _, set := os.LookupEnv("WEB_ADDR"); !set {
		webAddr = ":8080"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	if webAddr != "" {
		resp, err := client.Get("http://" + localAddr(webAddr) + "/healthz") //nolint:gosec,noctx // fixed local healthcheck address, not user input
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("close healthcheck response body", "error", cerr)
		}
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
	c, err := net.DialTimeout("tcp", localAddr(addr), 3*time.Second) //nolint:gosec,noctx // fixed local healthcheck address, not user input
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if cerr := c.Close(); cerr != nil {
		slog.Warn("close healthcheck connection", "error", cerr)
	}
	return 0
}

func localAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}
