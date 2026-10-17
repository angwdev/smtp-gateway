package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/store"
)

// ---- fake upstream ---------------------------------------------------------

type received struct {
	authUser string
	from     string
	rcpts    []string
	data     string
}

type fakeUpstream struct {
	mu       sync.Mutex
	msgs     []received
	conns    int
	rejectTo string
}

func (f *fakeUpstream) NewSession(*smtp.Conn) (smtp.Session, error) {
	f.mu.Lock()
	f.conns++
	f.mu.Unlock()
	return &fakeSession{f: f}, nil
}

type fakeSession struct {
	f    *fakeUpstream
	user string
	cur  received
}

func (s *fakeSession) AuthMechanisms() []string { return []string{sasl.Plain} }
func (s *fakeSession) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, u, p string) error {
		if u != "gateway" || p != "upstream-secret" {
			return smtp.ErrAuthFailed
		}
		s.user = u
		return nil
	}), nil
}
func (s *fakeSession) Mail(from string, _ *smtp.MailOptions) error {
	if s.user == "" {
		return smtp.ErrAuthRequired
	}
	s.cur = received{authUser: s.user, from: from}
	return nil
}
func (s *fakeSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	if to == s.f.rejectTo {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "No such user here"}
	}
	s.cur.rcpts = append(s.cur.rcpts, to)
	return nil
}
func (s *fakeSession) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil { // connection dropped before the final dot: discard, like a real MTA
		return err
	}
	s.cur.data = string(b)
	s.f.mu.Lock()
	s.f.msgs = append(s.f.msgs, s.cur)
	s.f.mu.Unlock()
	return nil
}
func (s *fakeSession) Reset()        {}
func (s *fakeSession) Logout() error { return nil }

func (f *fakeUpstream) received() []received {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]received(nil), f.msgs...)
}

// ---- harness ---------------------------------------------------------------

type harness struct {
	t     *testing.T
	st    *store.Store
	up    *fakeUpstream
	addr  string
	cfg   *config.Config
	upLn  net.Listener
	upSrv *smtp.Server
}

func newHarness(t *testing.T, mutate func(*config.Config)) *harness {
	t.Helper()
	fu := &fakeUpstream{}
	upSrv := smtp.NewServer(fu)
	upSrv.Domain = "upstream.test"
	upSrv.AllowInsecureAuth = true
	upSrv.MaxLineLength = 1 << 20
	upLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go upSrv.Serve(upLn)
	t.Cleanup(func() { upSrv.Close() })

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	upAddr := upLn.Addr().(*net.TCPAddr)
	cfg := &config.Config{
		Hostname:          "gateway.test",
		SubmissionAddr:    "127.0.0.1:0",
		AllowInsecureAuth: true,
		MaxMessageBytes:   1 << 20,
		MaxRecipients:     10,
		MaxLineLength:     4096,
		AuthFailLimit:     3,
		AuthFailWindow:    time.Minute,
		Upstream: config.Upstream{
			Host: "127.0.0.1", Port: upAddr.Port, TLSMode: "none",
			Username: "gateway", Password: "upstream-secret",
			HeloName: "gateway.test", Timeout: 5 * time.Second,
		},
	}
	if mutate != nil {
		mutate(cfg)
	}
	gw := New(cfg, st, NewUpstream(cfg.Upstream))
	if _, err := gw.Start(nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		gw.Shutdown(ctx)
	})
	return &harness{t: t, st: st, up: fu, addr: gw.Addrs()[0].String(), cfg: cfg, upLn: upLn, upSrv: upSrv}
}

func (h *harness) cred(username string, senders []string, rate int) string {
	h.t.Helper()
	secret, err := h.st.CreateCredential(context.Background(), &store.Credential{
		Username: username, AllowedSenders: senders, RatePerHour: rate, Enabled: true,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return secret
}

func (h *harness) dial(user, pass string) (*smtp.Client, error) {
	c, err := smtp.Dial(h.addr)
	if err != nil {
		return nil, err
	}
	if err := c.Auth(sasl.NewPlainClient("", user, pass)); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func msg(from, to string) string {
	return "From: " + from + "\r\nTo: " + to + "\r\nSubject: hi\r\n\r\nbody line\r\n"
}

func send(c *smtp.Client, from string, to []string, body string) error {
	if err := c.Mail(from, nil); err != nil {
		return err
	}
	for _, r := range to {
		if err := c.Rcpt(r, nil); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, body); err != nil {
		return err
	}
	return w.Close()
}

func code(err error) int {
	var e *smtp.SMTPError
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

func (h *harness) lastLog() *store.Message {
	h.t.Helper()
	msgs, err := h.st.ListMessages(context.Background(), store.MessageFilter{Limit: 1})
	if err != nil || len(msgs) == 0 {
		h.t.Fatalf("no log entries (err=%v)", err)
	}
	return msgs[0]
}

// ---- tests -----------------------------------------------------------------

func TestRelayTranslatesCredentials(t *testing.T) {
	h := newHarness(t, nil)
	pass := h.cred("nextcloud", []string{"@example.com"}, 0)

	c, err := h.dial("nextcloud", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Two messages over one client session reuse one upstream connection.
	for i := 0; i < 2; i++ {
		if err := send(c, "cloud@example.com", []string{"a@dest.test", "b@dest.test"}, msg("Cloud <cloud@example.com>", "a@dest.test")); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	c.Quit()

	got := h.up.received()
	if len(got) != 2 {
		t.Fatalf("upstream got %d messages, want 2", len(got))
	}
	if got[0].authUser != "gateway" || got[0].from != "cloud@example.com" || len(got[0].rcpts) != 2 {
		t.Fatalf("unexpected upstream message: %+v", got[0])
	}
	if !strings.Contains(got[0].data, "Subject: hi") || !strings.Contains(got[0].data, "body line") {
		t.Fatalf("body not relayed intact: %q", got[0].data)
	}
	if h.up.conns != 1 {
		t.Fatalf("upstream connections = %d, want 1", h.up.conns)
	}
	if l := h.lastLog(); l.Status != "sent" || l.Username != "nextcloud" || l.Size == 0 {
		t.Fatalf("log = %+v", l)
	}
	cr, _ := h.st.GetCredential(context.Background(), 1)
	if cr.SentCount != 2 || cr.LastUsedAt == nil {
		t.Fatalf("credential stats not updated: %+v", cr)
	}
}

func TestBadPasswordAndLockout(t *testing.T) {
	h := newHarness(t, nil)
	pass := h.cred("app", nil, 0)

	for i := 0; i < 3; i++ {
		if _, err := h.dial("app", "wrong"); code(err) != 535 {
			t.Fatalf("attempt %d: want 535, got %v", i, err)
		}
	}
	// Locked out even with the right password now.
	if _, err := h.dial("app", pass); code(err) != 454 {
		t.Fatalf("want 454 lockout, got %v", err)
	}
	if _, err := h.dial("nobody", "x"); err == nil {
		t.Fatal("unknown user authenticated")
	}
}

func TestAuthRequired(t *testing.T) {
	h := newHarness(t, nil)
	c, err := smtp.Dial(h.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Mail("x@example.com", nil); code(err) != 502 {
		t.Fatalf("want 502 auth required, got %v", err)
	}
}

func TestLoginMechanism(t *testing.T) {
	h := newHarness(t, nil)
	pass := h.cred("legacy", nil, 0)
	c, err := smtp.Dial(h.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Auth(sasl.NewLoginClient("legacy", pass)); err != nil {
		t.Fatalf("AUTH LOGIN: %v", err)
	}
	if err := send(c, "a@b.test", []string{"x@dest.test"}, msg("a@b.test", "x@dest.test")); err != nil {
		t.Fatal(err)
	}
}

func TestSenderPolicy(t *testing.T) {
	h := newHarness(t, nil)
	pass := h.cred("alerts", []string{"alerts@example.com"}, 0)
	c, err := h.dial("alerts", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := send(c, "ceo@example.com", []string{"x@dest.test"}, msg("ceo@example.com", "x")); code(err) != 550 {
		t.Fatalf("envelope: want 550, got %v", err)
	}
	c.Reset()
	// Envelope OK, but header From spoofed.
	if err := send(c, "alerts@example.com", []string{"x@dest.test"}, msg("CEO <ceo@example.com>", "x")); code(err) != 550 {
		t.Fatalf("header: want 550, got %v", err)
	}
	if l := h.lastLog(); l.Status != "rejected" || !strings.Contains(l.Detail, "From header") {
		t.Fatalf("log = %+v", l)
	}
	c.Reset()
	if err := send(c, "alerts@example.com", []string{"x@dest.test"}, msg("Alerts <ALERTS@example.com>", "x")); err != nil {
		t.Fatalf("allowed sender refused: %v", err)
	}
	if n := len(h.up.received()); n != 1 {
		t.Fatalf("upstream got %d messages, want 1", n)
	}
}

func TestRateLimit(t *testing.T) {
	h := newHarness(t, nil)
	pass := h.cred("limited", nil, 2)
	c, err := h.dial("limited", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i := 0; i < 2; i++ {
		if err := send(c, "a@b.test", []string{"x@dest.test"}, msg("a@b.test", "x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := send(c, "a@b.test", []string{"x@dest.test"}, msg("a@b.test", "x")); code(err) != 451 {
		t.Fatalf("want 451 rate limited, got %v", err)
	}
}

func TestUpstreamRecipientRejectionPassesThrough(t *testing.T) {
	h := newHarness(t, nil)
	h.up.rejectTo = "ghost@dest.test"
	pass := h.cred("app", nil, 0)
	c, err := h.dial("app", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Mail("a@b.test", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Rcpt("ghost@dest.test", nil); code(err) != 550 || !strings.Contains(err.Error(), "No such user") {
		t.Fatalf("want upstream 550, got %v", err)
	}
	if err := c.Rcpt("real@dest.test", nil); err != nil {
		t.Fatal(err)
	}
	w, _ := c.Data()
	io.WriteString(w, msg("a@b.test", "real@dest.test"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if l := h.lastLog(); l.Status != "sent" || !strings.Contains(l.Detail, "ghost@dest.test") {
		t.Fatalf("log = %+v", l)
	}
}

func TestDisableTakesEffectOnOpenSession(t *testing.T) {
	h := newHarness(t, nil)
	pass := h.cred("app", nil, 0)
	c, err := h.dial("app", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := h.st.SetEnabled(context.Background(), 1, false); err != nil {
		t.Fatal(err)
	}
	if err := c.Mail("a@b.test", nil); code(err) != 535 {
		t.Fatalf("want 535 revoked, got %v", err)
	}
	if _, err := h.dial("app", pass); code(err) != 535 {
		t.Fatalf("disabled credential authenticated: %v", err)
	}
}

func TestUpstreamDownIsTemporary(t *testing.T) {
	h := newHarness(t, nil)
	pass := h.cred("app", nil, 0)
	h.upSrv.Close()
	c, err := h.dial("app", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Mail("a@b.test", nil); code(err) != 451 {
		t.Fatalf("want 451, got %v", err)
	}
	if l := h.lastLog(); l.Status != "failed" {
		t.Fatalf("log = %+v", l)
	}
}

func TestEnvelopeRewrite(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.Upstream.EnvelopeFrom = "bounces@relay.test" })
	pass := h.cred("app", nil, 0)
	c, err := h.dial("app", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := send(c, "app@example.com", []string{"x@dest.test"}, msg("app@example.com", "x")); err != nil {
		t.Fatal(err)
	}
	if got := h.up.received(); got[0].from != "bounces@relay.test" || !strings.Contains(got[0].data, "From: app@example.com") {
		t.Fatalf("envelope not rewritten: %+v", got[0])
	}
}

func TestLimits(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.MaxMessageBytes = 64 << 10 })
	pass := h.cred("app", nil, 0)

	c, err := h.dial("app", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Long (but allowed) line is relayed intact.
	long := strings.Repeat("x", 3000)
	if err := send(c, "a@b.test", []string{"x@dest.test"}, msg("a@b.test", "x")+long+"\r\n"); err != nil {
		t.Fatalf("3000-char line: %v", err)
	}
	if !strings.Contains(h.up.received()[0].data, long) {
		t.Fatal("long line not relayed intact")
	}
	// Oversized message is a permanent 552, not a temporary failure.
	if err := send(c, "a@b.test", []string{"x@dest.test"}, msg("a@b.test", "x")+strings.Repeat(strings.Repeat("y", 70)+"\r\n", 2000)); code(err) != 552 {
		t.Fatalf("oversized: want 552, got %v", err)
	}
	c.Close()

	// Line over the limit is a permanent 5xx.
	c, err = h.dial("app", pass)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	err = send(c, "a@b.test", []string{"x@dest.test"}, msg("a@b.test", "x")+strings.Repeat("z", 5000)+"\r\n")
	if code(err) < 500 {
		t.Fatalf("too long line: want 5xx, got %v", err)
	}
	if n := len(h.up.received()); n != 1 {
		t.Fatalf("upstream got %d messages, want 1", n)
	}
}

func TestSenderAllowed(t *testing.T) {
	cases := []struct {
		patterns []string
		addr     string
		want     bool
	}{
		{nil, "anything@x.test", true},
		{[]string{"*"}, "a@b.test", true},
		{[]string{"a@b.test"}, "A@B.test", true},
		{[]string{"a@b.test"}, "c@b.test", false},
		{[]string{"@b.test"}, "c@b.test", true},
		{[]string{"*@b.test"}, "c@b.test", true},
		{[]string{"@b.test"}, "c@sub.b.test", false},
		{[]string{"@*.b.test"}, "c@sub.b.test", true},
		{[]string{"@*.b.test"}, "c@evilb.test", false},
		{[]string{"@b.test"}, "", false},
		{[]string{"@b.test"}, "no-at-sign", false},
	}
	for _, c := range cases {
		if got := SenderAllowed(c.patterns, c.addr); got != c.want {
			t.Errorf("SenderAllowed(%v, %q) = %v, want %v", c.patterns, c.addr, got, c.want)
		}
	}
}
