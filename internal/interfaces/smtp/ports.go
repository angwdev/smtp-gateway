package smtp

import (
	"context"
	"time"

	gosmtp "github.com/emersion/go-smtp"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	appcred "github.com/angwdev/smtp-gateway/internal/application/credential"
	appmsg "github.com/angwdev/smtp-gateway/internal/application/message"
	"github.com/angwdev/smtp-gateway/internal/domain/credential"
)

// Credentials is what the SMTP server needs from the credential use cases.
type Credentials interface {
	Authenticate(ctx context.Context, cmd appcred.AuthenticateCommand) (*credential.Credential, error)
	Get(ctx context.Context, id int64) (*credential.Credential, error)
}

// Messages is what it needs from the message log.
type Messages interface {
	Record(ctx context.Context, cmd appmsg.RecordCommand) error
	CountSince(ctx context.Context, credentialID int64, since time.Time) (int, error)
}

// Auditor records security events.
type Auditor interface {
	Record(ctx context.Context, e appaudit.Entry)
	RecordNoisy(ctx context.Context, e appaudit.Entry)
}

// Dialer opens an authenticated connection to the upstream server.
type Dialer interface {
	Dial(ctx context.Context) (*gosmtp.Client, error)
}
