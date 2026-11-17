// Package smtp implements the client-facing SMTP submission server. Clients
// authenticate with per-identity credentials via the credential use cases;
// accepted mail is streamed straight through to the upstream server, which
// sees only the single gateway credential. Nothing is queued: the client
// gets the upstream's answer, so delivery responsibility never silently sits
// in the gateway.
package smtp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	appcred "github.com/angwdev/smtp-gateway/internal/application/credential"
	appmsg "github.com/angwdev/smtp-gateway/internal/application/message"
	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/credential"
	"github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/internal/interfaces/limiter"
)

const maxHeaderBytes = 64 << 10

// Gateway is the client-facing SMTP server.
type Gateway struct {
	cfg      *config.Config
	creds    Credentials
	msgs     Messages
	audit    Auditor
	upstream Dialer
	fails    *limiter.FailLimiter
	servers  []*gosmtp.Server
	addrs    []net.Addr
}

// Addrs returns the bound listener addresses (useful with ":0" in tests).
func (g *Gateway) Addrs() []net.Addr { return g.addrs }

// New builds the SMTP gateway.
func New(cfg *config.Config, creds Credentials, msgs Messages, auditor Auditor, upstream Dialer) *Gateway {
	return &Gateway{cfg: cfg, creds: creds, msgs: msgs, audit: auditor, upstream: upstream,
		fails: limiter.NewFailLimiter(cfg.AuthFailLimit, cfg.AuthFailWindow)}
}

func (g *Gateway) newServer(addr string, tlsConf *tls.Config) *gosmtp.Server {
	s := gosmtp.NewServer(g)
	s.Addr = addr
	s.Domain = g.cfg.Hostname
	s.TLSConfig = tlsConf
	s.AllowInsecureAuth = g.cfg.AllowInsecureAuth
	s.MaxMessageBytes = g.cfg.MaxMessageBytes
	s.MaxRecipients = g.cfg.MaxRecipients
	// RFC 5321 says 1000, but many apps emit long unwrapped HTML lines and
	// real MTAs (Postfix) accept them; the limiter only counts, it does not
	// buffer whole lines.
	if g.cfg.MaxLineLength > 0 {
		s.MaxLineLength = g.cfg.MaxLineLength
	}
	s.ReadTimeout = 5 * time.Minute
	s.WriteTimeout = 1 * time.Minute
	s.ErrorLog = slogLogger{}
	return s
}

// Start launches the configured listeners. ListenAndServe errors are sent on
// the returned channel.
func (g *Gateway) Start(tlsConf *tls.Config) (<-chan error, error) {
	// one slot per listener, so a failing Serve never blocks on send
	errc := make(chan error, 2)
	start := func(addr string, implicitTLS bool) error {
		s := g.newServer(addr, tlsConf)
		var ln net.Listener
		var err error
		if implicitTLS {
			ln, err = tls.Listen("tcp", addr, tlsConf)
		} else {
			ln, err = (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
		}
		if err != nil {
			return err
		}
		g.servers = append(g.servers, s)
		g.addrs = append(g.addrs, ln.Addr())
		slog.Info("smtp listening", "addr", addr, "implicit_tls", implicitTLS, "starttls", !implicitTLS && tlsConf != nil)
		go func() {
			if err := s.Serve(ln); err != nil && !errors.Is(err, gosmtp.ErrServerClosed) {
				errc <- err
			}
		}()
		return nil
	}
	if g.cfg.SubmissionAddr != "" {
		if err := start(g.cfg.SubmissionAddr, false); err != nil {
			return nil, err
		}
	}
	if g.cfg.SMTPSAddr != "" {
		if err := start(g.cfg.SMTPSAddr, true); err != nil {
			return nil, err
		}
	}
	return errc, nil
}

// Shutdown stops every listener, waiting for ctx or for in-flight sessions
// to drain.
func (g *Gateway) Shutdown(ctx context.Context) {
	for _, s := range g.servers {
		_ = s.Shutdown(ctx)
	}
}

// Run serves until ctx is cancelled or a listener fails, then shuts down,
// giving open sessions 20 seconds to finish.
func (g *Gateway) Run(ctx context.Context, tlsConf *tls.Config) error {
	errc, err := g.Start(tlsConf)
	if err != nil {
		return fmt.Errorf("start smtp listeners: %w", err)
	}
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errc:
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	g.Shutdown(sctx)
	if serveErr != nil {
		return fmt.Errorf("smtp listener: %w", serveErr)
	}
	return nil
}

// NewSession implements gosmtp.Backend.
func (g *Gateway) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	ip := c.Conn().RemoteAddr().String()
	if h, _, err := net.SplitHostPort(ip); err == nil {
		ip = h
	}
	return &session{g: g, conn: c, ip: ip, log: slog.With("ip", ip)}, nil
}

type session struct {
	g    *Gateway
	conn *gosmtp.Conn
	ip   string
	log  *slog.Logger

	cred *credential.Credential

	from     string
	opts     *gosmtp.MailOptions
	rcpts    []string
	rejected []string
	header   []byte
	up       *gosmtp.Client
}

// AuthMechanisms implements gosmtp.Session.
func (s *session) AuthMechanisms() []string { return []string{sasl.Plain, sasl.Login} }

// Auth implements gosmtp.Session.
func (s *session) Auth(mech string) (sasl.Server, error) {
	if s.g.fails.Blocked(s.ip) {
		s.log.Warn("auth blocked: too many failures")
		return nil, &gosmtp.SMTPError{Code: 454, EnhancedCode: gosmtp.EnhancedCode{4, 7, 0}, Message: "Too many failed attempts, try again later"}
	}
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			if identity != "" && identity != username {
				return gosmtp.ErrAuthFailed
			}
			return s.authenticate(username, password)
		}), nil
	case sasl.Login:
		return newLoginServer(s.authenticate), nil
	}
	return nil, gosmtp.ErrAuthUnknownMechanism
}

func (s *session) authenticate(username, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cred, err := s.g.creds.Authenticate(ctx, appcred.AuthenticateCommand{Username: username, Password: password, IP: s.ip})
	if err != nil {
		if errors.Is(err, shared.ErrInvalidLogin) {
			s.authFailed(ctx, username)
			return gosmtp.ErrAuthFailed
		}
		s.log.Error("auth lookup", "err", err)
		return tempErr("Authentication temporarily unavailable")
	}
	s.g.fails.Reset(s.ip)
	s.cred = cred
	s.log = s.log.With("user", cred.Username)
	s.log.Debug("auth ok")
	return nil
}

// authFailed counts the failure and leaves an audit trail: one row per
// failed login (rate-capped by the recorder) and one row when the address
// gets locked out.
func (s *session) authFailed(ctx context.Context, username string) {
	if len(username) > 64 {
		username = username[:64]
	}
	s.log.Warn("auth failed", "user", username)
	ctx = appaudit.WithMeta(ctx, appaudit.Meta{Actor: username, ActorType: audit.ActorSMTP, IP: s.ip})
	s.g.audit.RecordNoisy(ctx, appaudit.Entry{Action: "smtp.auth_failed", TargetType: "credential", TargetLabel: username, Result: audit.ResultDenied})
	if s.g.fails.Fail(s.ip) {
		s.g.audit.Record(ctx, appaudit.Entry{Action: "smtp.lockout", Result: audit.ResultDenied,
			Detail: "too many failed logins; blocked for " + s.g.cfg.AuthFailWindow.String()})
	}
}

// Mail implements gosmtp.Session.
func (s *session) Mail(from string, opts *gosmtp.MailOptions) error {
	if s.cred == nil {
		return gosmtp.ErrAuthRequired
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.g.cfg.Upstream.Timeout)
	defer cancel()

	// Re-read so that disabling or editing a credential applies immediately,
	// even to long-lived client connections.
	cred, err := s.g.creds.Get(ctx, s.cred.ID)
	if err != nil || !cred.Enabled {
		s.cred = nil
		return &gosmtp.SMTPError{Code: 535, EnhancedCode: gosmtp.EnhancedCode{5, 7, 8}, Message: "Credential revoked"}
	}
	s.cred = cred
	s.from, s.opts = from, opts

	if !cred.AllowedSenders.Allows(from) {
		s.record(message.StatusRejected, "envelope sender not allowed for this credential", 0)
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "Sender address not allowed for this login"}
	}
	if cred.RatePerHour > 0 {
		n, err := s.g.msgs.CountSince(ctx, cred.ID, time.Now().Add(-time.Hour))
		if err != nil {
			s.log.Error("rate count", "err", err)
			return tempErr("Temporary local error")
		}
		if n >= cred.RatePerHour {
			s.record(message.StatusRejected, "hourly rate limit reached", 0)
			return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 7, 1}, Message: "Hourly sending limit reached, try again later"}
		}
	}

	if s.up == nil {
		up, err := s.g.upstream.Dial(ctx)
		if err != nil {
			s.log.Error("upstream connect", "err", err)
			s.record(message.StatusFailed, "upstream connect: "+err.Error(), 0)
			return upstreamErr(err, "Upstream server unavailable")
		}
		s.up = up
	}

	upFrom := from
	if s.g.cfg.Upstream.EnvelopeFrom != "" {
		upFrom = s.g.cfg.Upstream.EnvelopeFrom
	}
	var upOpts *gosmtp.MailOptions
	if opts != nil {
		upOpts = &gosmtp.MailOptions{Body: opts.Body, Size: opts.Size}
	}
	if err := s.up.Mail(upFrom, upOpts); err != nil {
		s.record(message.StatusFailed, "upstream MAIL FROM: "+err.Error(), 0)
		s.dropUpstreamIfBroken(err)
		return upstreamErr(err, "Upstream rejected sender")
	}
	return nil
}

// Rcpt implements gosmtp.Session.
func (s *session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	if s.up == nil {
		return &gosmtp.SMTPError{Code: 503, EnhancedCode: gosmtp.EnhancedCode{5, 5, 1}, Message: "MAIL first"}
	}
	if err := s.up.Rcpt(to, nil); err != nil {
		s.rejected = append(s.rejected, to)
		s.dropUpstreamIfBroken(err)
		return upstreamErr(err, "Upstream rejected recipient")
	}
	s.rcpts = append(s.rcpts, to)
	return nil
}

// Data implements gosmtp.Session.
func (s *session) Data(r io.Reader) error {
	if s.up == nil {
		return tempErr("Upstream connection lost")
	}
	br := bufio.NewReaderSize(r, 4096)
	header, err := readHeader(br)
	if err != nil {
		s.record(message.StatusRejected, "reading message: "+err.Error(), 0)
		return dataReadErr(err)
	}
	s.header = header
	if froms, ok := headerFrom(header); len(s.cred.AllowedSenders) > 0 {
		if !ok {
			s.record(message.StatusRejected, "could not parse From header", 0)
			return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 6, 0}, Message: "Unparseable From header"}
		}
		for _, f := range froms {
			if !s.cred.AllowedSenders.Allows(f) {
				s.record(message.StatusRejected, "From header "+f+" not allowed for this credential", 0)
				return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "From address not allowed for this login"}
			}
		}
	}

	w, err := s.up.Data()
	if err != nil {
		s.record(message.StatusFailed, "upstream DATA: "+err.Error(), 0)
		s.dropUpstreamIfBroken(err)
		return upstreamErr(err, "Upstream refused message")
	}
	cw := &countWriter{w: w}
	_, err = cw.Write(header)
	if err == nil {
		_, err = io.Copy(cw, br)
	}
	if err != nil {
		// The upstream transaction is in an unknown state; never reuse it.
		s.closeUpstream()
		s.record(message.StatusFailed, "transfer: "+err.Error(), cw.n)
		return dataReadErr(err)
	}
	resp, err := w.CloseWithResponse()
	if err != nil {
		s.record(message.StatusFailed, "upstream: "+err.Error(), cw.n)
		s.dropUpstreamIfBroken(err)
		return upstreamErr(err, "Upstream rejected message")
	}
	s.record(message.StatusSent, resp.StatusText, cw.n)
	s.log.Info("relayed", "from", s.from, "rcpts", len(s.rcpts), "bytes", cw.n)
	return nil
}

// Reset is called by go-smtp after every message and on RSET. The upstream
// connection is kept and reset so a client sending a batch over one session
// reuses one upstream connection.
func (s *session) Reset() {
	s.from, s.opts, s.rcpts, s.rejected, s.header = "", nil, nil, nil, nil
	if s.up != nil {
		if err := s.up.Reset(); err != nil {
			s.closeUpstream()
		}
	}
}

// Logout implements gosmtp.Session.
func (s *session) Logout() error {
	if s.up != nil {
		_ = s.up.Quit()
		s.closeUpstream()
	}
	return nil
}

func (s *session) closeUpstream() {
	if s.up != nil {
		_ = s.up.Close()
		s.up = nil
	}
}

// dropUpstreamIfBroken discards the upstream connection on anything other
// than a regular SMTP reply (network errors, timeouts, 421 shutdowns).
func (s *session) dropUpstreamIfBroken(err error) {
	var smtpErr *gosmtp.SMTPError
	if !errors.As(err, &smtpErr) || smtpErr.Code == 421 {
		s.closeUpstream()
	}
}

func (s *session) record(status message.Status, detail string, size int64) {
	cmd := appmsg.RecordCommand{
		RemoteIP: s.ip, MailFrom: s.from, RcptTo: s.rcpts, Rejected: s.rejected,
		Size: size, Status: status, Detail: detail, Header: s.header,
	}
	if s.cred != nil {
		id := s.cred.ID
		cmd.CredentialID, cmd.Username = &id, s.cred.Username
	}
	if status != message.StatusSent {
		s.log.Warn("message "+string(status), "from", s.from, "detail", detail)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.g.msgs.Record(ctx, cmd); err != nil {
		s.log.Error("log message", "err", err)
	}
}

// ---- helpers ---------------------------------------------------------------

func tempErr(msg string) error {
	return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 3, 0}, Message: msg}
}

// upstreamErr passes upstream SMTP replies through verbatim; anything else
// (network trouble) becomes a temporary failure so clients retry rather than
// bounce.
func upstreamErr(err error, msg string) error {
	var smtpErr *gosmtp.SMTPError
	if errors.As(err, &smtpErr) {
		return smtpErr
	}
	return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 4, 1}, Message: msg}
}

// dataReadErr maps errors from reading the client's DATA stream: limits we
// enforce are permanent, anything else is a temporary failure.
func dataReadErr(err error) error {
	var smtpErr *gosmtp.SMTPError
	switch {
	case errors.As(err, &smtpErr): // e.g. gosmtp.ErrDataTooLarge
		return smtpErr
	case errors.Is(err, gosmtp.ErrTooLongLine):
		return &gosmtp.SMTPError{Code: 554, EnhancedCode: gosmtp.EnhancedCode{5, 6, 0}, Message: "Message contains a line that is too long"}
	}
	return tempErr("Message transfer interrupted")
}

// readHeader returns the raw header block including the blank separator
// line. If no separator appears within maxHeaderBytes, whatever was read is
// returned and treated as an opaque prefix of the body.
func readHeader(br *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for buf.Len() < maxHeaderBytes {
		line, err := br.ReadSlice('\n')
		buf.Write(line)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			return buf.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimRight(line, "\r\n")) == 0 {
			return buf.Bytes(), nil
		}
	}
	return buf.Bytes(), nil
}

var addrParser = &mail.AddressParser{WordDecoder: &mime.WordDecoder{
	CharsetReader: func(_ string, r io.Reader) (io.Reader, error) { return r, nil },
}}

// headerFrom returns the addresses in the From header. ok is false when a
// From header is present but cannot be parsed. A missing From is ok: the
// envelope sender has already been checked.
func headerFrom(header []byte) (addrs []string, ok bool) {
	h, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(header))).ReadMIMEHeader()
	if err != nil && len(h) == 0 {
		return nil, false
	}
	raw := h.Values("From")
	for _, v := range raw {
		list, err := addrParser.ParseList(v)
		if err != nil {
			return nil, false
		}
		for _, a := range list {
			addrs = append(addrs, a.Address)
		}
	}
	return addrs, true
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

type slogLogger struct{}

// Printf implements the go-smtp server's error logger interface.
func (slogLogger) Printf(format string, v ...any) {
	slog.Debug("smtp", "msg", strings.TrimSpace(fmt.Sprintf(format, v...)))
}

// Println implements the go-smtp server's error logger interface.
func (slogLogger) Println(v ...any) { slog.Debug("smtp", "msg", strings.TrimSpace(fmt.Sprintln(v...))) }
