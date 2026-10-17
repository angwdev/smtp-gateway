package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/angw/smtp-gateway/internal/config"
)

// Upstream opens authenticated connections to the real mail server using the
// single gateway credential.
type Upstream struct {
	cfg config.Upstream
}

func NewUpstream(cfg config.Upstream) *Upstream { return &Upstream{cfg: cfg} }

func (u *Upstream) Config() config.Upstream { return u.cfg }

func (u *Upstream) Dial(ctx context.Context) (*smtp.Client, error) {
	tlsConf := &tls.Config{
		ServerName:         u.cfg.Host,
		InsecureSkipVerify: u.cfg.TLSSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}
	d := net.Dialer{Timeout: u.cfg.Timeout}
	conn, err := d.DialContext(ctx, "tcp", u.cfg.Addr())
	if err != nil {
		return nil, err
	}

	var c *smtp.Client
	switch u.cfg.TLSMode {
	case "tls":
		tc := tls.Client(conn, tlsConf)
		hctx, cancel := context.WithTimeout(ctx, u.cfg.Timeout)
		err = tc.HandshakeContext(hctx)
		cancel()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("tls handshake: %w", err)
		}
		c = newClient(tc, u.cfg.Timeout)
		err = c.Hello(u.cfg.HeloName)
	case "starttls":
		// NewClientStartTLS greets with "localhost"; once TLS is up the client
		// re-introduces itself, so Hello sets our real name for the session.
		conn.SetDeadline(time.Now().Add(u.cfg.Timeout))
		c, err = smtp.NewClientStartTLS(conn, tlsConf)
		conn.SetDeadline(time.Time{})
		if err == nil {
			c.CommandTimeout = u.cfg.Timeout
			err = c.Hello(u.cfg.HeloName)
		}
	default:
		c = newClient(conn, u.cfg.Timeout)
		err = c.Hello(u.cfg.HeloName)
	}
	if err != nil {
		if c != nil {
			c.Close()
		} else {
			conn.Close()
		}
		return nil, err
	}

	if u.cfg.Username != "" {
		var mech sasl.Client
		switch {
		case c.SupportsAuth("PLAIN"):
			mech = sasl.NewPlainClient("", u.cfg.Username, u.cfg.Password)
		case c.SupportsAuth("LOGIN"):
			mech = sasl.NewLoginClient(u.cfg.Username, u.cfg.Password)
		default:
			c.Close()
			return nil, errors.New("upstream offers neither AUTH PLAIN nor AUTH LOGIN")
		}
		if err := c.Auth(mech); err != nil {
			c.Close()
			return nil, fmt.Errorf("upstream auth: %w", err)
		}
	}
	return c, nil
}

func newClient(conn net.Conn, timeout time.Duration) *smtp.Client {
	c := smtp.NewClient(conn)
	c.CommandTimeout = timeout
	return c
}

// Probe connects, authenticates and disconnects, reporting how long it took.
func (u *Upstream) Probe(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	c, err := u.Dial(ctx)
	if err != nil {
		return time.Since(start), err
	}
	_ = c.Quit()
	return time.Since(start), nil
}
