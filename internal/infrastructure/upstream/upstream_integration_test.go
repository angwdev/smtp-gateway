//go:build integration

package upstream_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/tlsutil"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/upstream"
)

const (
	testUser = "gateway"
	testPass = "upstream-secret"
)

// ---- fake upstream, auth mechanisms configurable per test -----------------

type fakeBackend struct{ mechs []string }

func (b *fakeBackend) NewSession(*gosmtp.Conn) (gosmtp.Session, error) {
	return &fakeSession{mechs: b.mechs}, nil
}

type fakeSession struct{ mechs []string }

func (s *fakeSession) AuthMechanisms() []string { return s.mechs }

func (s *fakeSession) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(_, u, p string) error {
			if u != testUser || p != testPass {
				return gosmtp.ErrAuthFailed
			}
			return nil
		}), nil
	case sasl.Login:
		return &fakeLoginServer{}, nil
	default:
		return nil, gosmtp.ErrAuthUnsupported
	}
}

func (s *fakeSession) Mail(string, *gosmtp.MailOptions) error { return nil }
func (s *fakeSession) Rcpt(string, *gosmtp.RcptOptions) error { return nil }
func (s *fakeSession) Data(io.Reader) error                   { return nil }
func (s *fakeSession) Reset()                                 {}
func (s *fakeSession) Logout() error                          { return nil }

// fakeLoginServer is a minimal server side of SASL LOGIN: go-sasl ships only
// the client half (the mechanism is obsolete), so the fake upstream needs
// its own, matching the client's initial-response username then a
// challenge-response password (emersion/go-sasl login.go).
type fakeLoginServer struct {
	gotUser string
	step    int
}

func (l *fakeLoginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch l.step {
	case 0:
		l.gotUser = string(response)
		l.step++
		return []byte("Password:"), false, nil
	case 1:
		l.step++
		if l.gotUser != testUser || string(response) != testPass {
			return nil, false, gosmtp.ErrAuthFailed
		}
		return nil, true, nil
	default:
		return nil, false, gosmtp.ErrAuthFailed
	}
}

// startFakeServer listens in plain TCP; tlsConfig, when set, only turns on
// the STARTTLS capability (section 2.5: the plain listener handles both
// "none" and "starttls" dials).
func startFakeServer(t *testing.T, mechs []string, tlsConfig *tls.Config) (host string, port int) {
	t.Helper()
	srv := gosmtp.NewServer(&fakeBackend{mechs: mechs})
	srv.Domain = "upstream.test"
	srv.AllowInsecureAuth = true
	srv.TLSConfig = tlsConfig
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go srv.Serve(ln) //nolint:errcheck // test server, Close() below ends Serve
	t.Cleanup(func() { _ = srv.Close() })
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

// startFakeTLSServer listens with an immediate TLS handshake, like a real
// SMTPS port.
func startFakeTLSServer(t *testing.T, mechs []string, tlsConfig *tls.Config) (host string, port int) {
	t.Helper()
	srv := gosmtp.NewServer(&fakeBackend{mechs: mechs})
	srv.Domain = "upstream.test"
	srv.AllowInsecureAuth = true
	srv.TLSConfig = tlsConfig
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	require.NoError(t, err)
	go srv.Serve(ln) //nolint:errcheck // test server, Close() below ends Serve
	t.Cleanup(func() { _ = srv.Close() })
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func selfSignedConfig(t *testing.T) *tls.Config {
	t.Helper()
	loader, err := tlsutil.NewSelfSigned(t.TempDir(), "upstream.test")
	require.NoError(t, err)
	return loader.Config()
}

func cfgFor(host string, port int, mode string) config.Upstream {
	return config.Upstream{
		Host: host, Port: port, TLSMode: mode, TLSSkipVerify: true,
		Username: testUser, Password: testPass, HeloName: "gateway.test", Timeout: 5 * time.Second,
	}
}

// ---- tests ------------------------------------------------------------

func TestConfigReturnsWhatItWasBuiltWith(t *testing.T) {
	cfg := cfgFor("127.0.0.1", 25, "none")
	assert.Equal(t, cfg, upstream.New(cfg).Config())
}

func TestDialPlainAuth(t *testing.T) {
	host, port := startFakeServer(t, []string{sasl.Plain}, nil)
	c, err := upstream.New(cfgFor(host, port, "none")).Dial(context.Background())
	require.NoError(t, err)
	_ = c.Close() //nolint:errcheck // test cleanup
}

func TestDialLoginAuth(t *testing.T) {
	host, port := startFakeServer(t, []string{sasl.Login}, nil)
	c, err := upstream.New(cfgFor(host, port, "none")).Dial(context.Background())
	require.NoError(t, err)
	_ = c.Close() //nolint:errcheck // test cleanup
}

func TestDialAuthFailure(t *testing.T) {
	host, port := startFakeServer(t, []string{sasl.Plain}, nil)
	cfg := cfgFor(host, port, "none")
	cfg.Password = "wrong"
	_, err := upstream.New(cfg).Dial(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream auth")
}

func TestDialNoSupportedMechanism(t *testing.T) {
	host, port := startFakeServer(t, nil, nil)
	_, err := upstream.New(cfgFor(host, port, "none")).Dial(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neither AUTH PLAIN nor AUTH LOGIN")
}

func TestDialWithoutUsernameSkipsAuth(t *testing.T) {
	host, port := startFakeServer(t, nil, nil)
	cfg := cfgFor(host, port, "none")
	cfg.Username = ""
	c, err := upstream.New(cfg).Dial(context.Background())
	require.NoError(t, err)
	_ = c.Close() //nolint:errcheck // test cleanup
}

func TestDialConnectionRefused(t *testing.T) {
	_, err := upstream.New(cfgFor("127.0.0.1", 1, "none")).Dial(context.Background())
	require.Error(t, err)
}

func TestDialHelloFailureClosesTheClient(t *testing.T) {
	// validateLine rejects a CRLF in the local name before any network
	// round trip, so err and a non-nil client both come out of the
	// default case, exercising the "if c != nil" cleanup branch.
	host, port := startFakeServer(t, []string{sasl.Plain}, nil)
	cfg := cfgFor(host, port, "none")
	cfg.HeloName = "bad\r\nname"
	_, err := upstream.New(cfg).Dial(context.Background())
	require.Error(t, err)
}

func TestDialStartTLSSuccess(t *testing.T) {
	host, port := startFakeServer(t, []string{sasl.Plain}, selfSignedConfig(t))
	c, err := upstream.New(cfgFor(host, port, "starttls")).Dial(context.Background())
	require.NoError(t, err)
	_ = c.Close() //nolint:errcheck // test cleanup
}

func TestDialStartTLSUnsupportedByServer(t *testing.T) {
	// No TLSConfig on the fake server: it never advertises STARTTLS, so
	// NewClientStartTLS fails without a client (the "c == nil" cleanup path).
	host, port := startFakeServer(t, []string{sasl.Plain}, nil)
	_, err := upstream.New(cfgFor(host, port, "starttls")).Dial(context.Background())
	require.Error(t, err)
}

func TestDialTLSSuccess(t *testing.T) {
	tlsConfig := selfSignedConfig(t)
	host, port := startFakeTLSServer(t, []string{sasl.Plain}, tlsConfig)
	c, err := upstream.New(cfgFor(host, port, "tls")).Dial(context.Background())
	require.NoError(t, err)
	_ = c.Close() //nolint:errcheck // test cleanup
}

func TestDialTLSHandshakeFailure(t *testing.T) {
	// A plain-text server answering a "tls" mode dial fails the handshake.
	host, port := startFakeServer(t, []string{sasl.Plain}, nil)
	_, err := upstream.New(cfgFor(host, port, "tls")).Dial(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tls handshake")
}

func TestProbeSucceeds(t *testing.T) {
	host, port := startFakeServer(t, []string{sasl.Plain}, nil)
	d, err := upstream.New(cfgFor(host, port, "none")).Probe(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, d, time.Duration(0))
}

func TestProbeFails(t *testing.T) {
	_, err := upstream.New(cfgFor("127.0.0.1", 1, "none")).Probe(context.Background())
	require.Error(t, err)
}
