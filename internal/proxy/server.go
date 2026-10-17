// Package proxy implements the client-facing SMTP submission server. Clients
// authenticate with per-identity credentials from the store; accepted mail is
// streamed straight through to the upstream server, which sees only the single
// gateway credential. Nothing is queued: the client gets the upstream's
// answer, so delivery responsibility never silently sits in the gateway.
package proxy

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
	"github.com/emersion/go-smtp"

	"github.com/angw/smtp-gateway/internal/config"
	"github.com/angw/smtp-gateway/internal/store"
)

const maxHeaderBytes = 64 << 10

type Gateway struct {
	cfg      *config.Config
	store    *store.Store
	upstream *Upstream
	fails    *failLimiter
	servers  []*smtp.Server
	addrs    []net.Addr
}

// Addrs returns the bound listener addresses (useful with ":0" in tests).
func (g *Gateway) Addrs() []net.Addr { return g.addrs }

func New(cfg *config.Config, st *store.Store, up *Upstream) *Gateway {
	return &Gateway{
		cfg:      cfg,
		store:    st,
		upstream: up,
		fails:    newFailLimiter(cfg.AuthFailLimit, cfg.AuthFailWindow),
	}
}

func (g *Gateway) newServer(addr string, tlsConf *tls.Config) *smtp.Server {
	s := smtp.NewServer(g)
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
	errc := make(chan error, 2)
	start := func(addr string, implicitTLS bool) error {
		s := g.newServer(addr, tlsConf)
		var ln net.Listener
		var err error
		if implicitTLS {
			ln, err = tls.Listen("tcp", addr, tlsConf)
		} else {
			ln, err = net.Listen("tcp", addr)
		}
		if err != nil {
			return err
		}
		g.servers = append(g.servers, s)
		g.addrs = append(g.addrs, ln.Addr())
		slog.Info("smtp listening", "addr", addr, "implicit_tls", implicitTLS, "starttls", !implicitTLS && tlsConf != nil)
		go func() {
			if err := s.Serve(ln); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
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

func (g *Gateway) Shutdown(ctx context.Context) {
	for _, s := range g.servers {
		_ = s.Shutdown(ctx)
	}
}

// NewSession implements smtp.Backend.
func (g *Gateway) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ip := c.Conn().RemoteAddr().String()
	if h, _, err := net.SplitHostPort(ip); err == nil {
		ip = h
	}
	return &session{g: g, conn: c, ip: ip, log: slog.With("ip", ip)}, nil
}

type session struct {
	g    *Gateway
	conn *smtp.Conn
	ip   string
	log  *slog.Logger

	cred *store.Credential

	from     string
	opts     *smtp.MailOptions
	rcpts    []string
	rejected []string
	up       *smtp.Client
}

func (s *session) AuthMechanisms() []string { return []string{sasl.Plain, sasl.Login} }

func (s *session) Auth(mech string) (sasl.Server, error) {
	if s.g.fails.Blocked(s.ip) {
		s.log.Warn("auth blocked: too many failures")
		return nil, &smtp.SMTPError{Code: 454, EnhancedCode: smtp.EnhancedCode{4, 7, 0}, Message: "Too many failed attempts, try again later"}
	}
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			if identity != "" && identity != username {
				return smtp.ErrAuthFailed
			}
			return s.authenticate(username, password)
		}), nil
	case sasl.Login:
		return newLoginServer(s.authenticate), nil
	}
	return nil, smtp.ErrAuthUnknownMechanism
}

func (s *session) authenticate(username, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cred, err := s.g.store.Authenticate(ctx, username, password)
	if err != nil {
		if errors.Is(err, store.ErrInvalidLogin) {
			s.g.fails.Fail(s.ip)
			s.log.Warn("auth failed", "user", username)
			return smtp.ErrAuthFailed
		}
		s.log.Error("auth lookup", "err", err)
		return tempErr("Authentication temporarily unavailable")
	}
	s.g.fails.Reset(s.ip)
	s.cred = cred
	s.log = s.log.With("user", cred.Username)
	if err := s.g.store.TouchCredential(ctx, cred.ID, s.ip); err != nil {
		s.log.Error("touch credential", "err", err)
	}
	s.log.Debug("auth ok")
	return nil
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	if s.cred == nil {
		return smtp.ErrAuthRequired
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.g.cfg.Upstream.Timeout)
	defer cancel()

	// Re-read so that disabling or editing a credential applies immediately,
	// even to long-lived client connections.
	cred, err := s.g.store.GetCredential(ctx, s.cred.ID)
	if err != nil || !cred.Enabled {
		s.cred = nil
		return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "Credential revoked"}
	}
	s.cred = cred
	s.from, s.opts = from, opts

	if !SenderAllowed(cred.AllowedSenders, from) {
		s.record("rejected", "envelope sender not allowed for this credential", 0)
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "Sender address not allowed for this login"}
	}
	if cred.RatePerHour > 0 {
		n, err := s.g.store.CountSince(ctx, cred.ID, time.Now().Add(-time.Hour))
		if err != nil {
			s.log.Error("rate count", "err", err)
			return tempErr("Temporary local error")
		}
		if n >= cred.RatePerHour {
			s.record("rejected", "hourly rate limit reached", 0)
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 7, 1}, Message: "Hourly sending limit reached, try again later"}
		}
	}

	if s.up == nil {
		up, err := s.g.upstream.Dial(ctx)
		if err != nil {
			s.log.Error("upstream connect", "err", err)
			s.record("failed", "upstream connect: "+err.Error(), 0)
			return upstreamErr(err, "Upstream server unavailable")
		}
		s.up = up
	}

	upFrom := from
	if s.g.cfg.Upstream.EnvelopeFrom != "" {
		upFrom = s.g.cfg.Upstream.EnvelopeFrom
	}
	var upOpts *smtp.MailOptions
	if opts != nil {
		upOpts = &smtp.MailOptions{Body: opts.Body, Size: opts.Size}
	}
	if err := s.up.Mail(upFrom, upOpts); err != nil {
		s.record("failed", "upstream MAIL FROM: "+err.Error(), 0)
		s.dropUpstreamIfBroken(err)
		return upstreamErr(err, "Upstream rejected sender")
	}
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	if s.up == nil {
		return &smtp.SMTPError{Code: 503, EnhancedCode: smtp.EnhancedCode{5, 5, 1}, Message: "MAIL first"}
	}
	if err := s.up.Rcpt(to, nil); err != nil {
		s.rejected = append(s.rejected, to)
		s.dropUpstreamIfBroken(err)
		return upstreamErr(err, "Upstream rejected recipient")
	}
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *session) Data(r io.Reader) error {
	if s.up == nil {
		return tempErr("Upstream connection lost")
	}
	br := bufio.NewReaderSize(r, 4096)
	header, err := readHeader(br)
	if err != nil {
		s.record("rejected", "reading message: "+err.Error(), 0)
		return dataReadErr(err)
	}
	if froms, ok := headerFrom(header); len(s.cred.AllowedSenders) > 0 {
		if !ok {
			s.record("rejected", "could not parse From header", 0)
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 6, 0}, Message: "Unparseable From header"}
		}
		for _, f := range froms {
			if !SenderAllowed(s.cred.AllowedSenders, f) {
				s.record("rejected", "From header "+f+" not allowed for this credential", 0)
				return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "From address not allowed for this login"}
			}
		}
	}

	w, err := s.up.Data()
	if err != nil {
		s.record("failed", "upstream DATA: "+err.Error(), 0)
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
		s.record("failed", "transfer: "+err.Error(), cw.n)
		return dataReadErr(err)
	}
	resp, err := w.CloseWithResponse()
	if err != nil {
		s.record("failed", "upstream: "+err.Error(), cw.n)
		s.dropUpstreamIfBroken(err)
		return upstreamErr(err, "Upstream rejected message")
	}
	s.record("sent", resp.StatusText, cw.n)
	s.log.Info("relayed", "from", s.from, "rcpts", len(s.rcpts), "bytes", cw.n)
	return nil
}

// Reset is called by go-smtp after every message and on RSET. The upstream
// connection is kept and reset so a client sending a batch over one session
// reuses one upstream connection.
func (s *session) Reset() {
	s.from, s.opts, s.rcpts, s.rejected = "", nil, nil, nil
	if s.up != nil {
		if err := s.up.Reset(); err != nil {
			s.closeUpstream()
		}
	}
}

func (s *session) Logout() error {
	if s.up != nil {
		_ = s.up.Quit()
		s.closeUpstream()
	}
	return nil
}

func (s *session) closeUpstream() {
	if s.up != nil {
		s.up.Close()
		s.up = nil
	}
}

// dropUpstreamIfBroken discards the upstream connection on anything other
// than a regular SMTP reply (network errors, timeouts, 421 shutdowns).
func (s *session) dropUpstreamIfBroken(err error) {
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) || smtpErr.Code == 421 {
		s.closeUpstream()
	}
}

func (s *session) record(status, detail string, size int64) {
	m := &store.Message{
		RemoteIP: s.ip,
		MailFrom: s.from,
		RcptTo:   s.rcpts,
		Size:     size,
		Status:   status,
		Detail:   detail,
	}
	if len(s.rejected) > 0 {
		m.Detail += " (upstream refused: " + strings.Join(s.rejected, ", ") + ")"
	}
	if s.cred != nil {
		id := s.cred.ID
		m.CredentialID, m.Username = &id, s.cred.Username
	}
	if status != "sent" {
		s.log.Warn("message "+status, "from", s.from, "detail", m.Detail)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.g.store.LogMessage(ctx, m); err != nil {
		s.log.Error("log message", "err", err)
	}
}

// ---- helpers ---------------------------------------------------------------

func tempErr(msg string) error {
	return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: msg}
}

// upstreamErr passes upstream SMTP replies through verbatim; anything else
// (network trouble) becomes a temporary failure so clients retry rather than
// bounce.
func upstreamErr(err error, msg string) error {
	var smtpErr *smtp.SMTPError
	if errors.As(err, &smtpErr) {
		return smtpErr
	}
	return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 4, 1}, Message: msg}
}

// dataReadErr maps errors from reading the client's DATA stream: limits we
// enforce are permanent, anything else is a temporary failure.
func dataReadErr(err error) error {
	var smtpErr *smtp.SMTPError
	switch {
	case errors.As(err, &smtpErr): // e.g. smtp.ErrDataTooLarge
		return smtpErr
	case errors.Is(err, smtp.ErrTooLongLine):
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 6, 0}, Message: "Message contains a line that is too long"}
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
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
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

func (slogLogger) Printf(format string, v ...any) {
	slog.Debug("smtp", "msg", strings.TrimSpace(fmt.Sprintf(format, v...)))
}
func (slogLogger) Println(v ...any) { slog.Debug("smtp", "msg", strings.TrimSpace(fmt.Sprintln(v...))) }
