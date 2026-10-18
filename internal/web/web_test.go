package web

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/proxy"
	"github.com/angwdev/smtp-gateway/internal/store"
	"github.com/angwdev/smtp-gateway/internal/tlsutil"
)

// ---- mock OIDC provider ----------------------------------------------------

type mockIdP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	claims map[string]any // claims for the next login
	codes  map[string]codeGrant
}

type codeGrant struct{ nonce, challenge string }

func newMockIdP(t *testing.T) *mockIdP {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	m := &mockIdP{key: key, codes: map[string]codeGrant{}}
	mux := http.NewServeMux()
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	iss := m.srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": iss, "authorization_endpoint": iss + "/authorize", "token_endpoint": iss + "/token",
			"jwks_uri": iss + "/jwks", "userinfo_endpoint": iss + "/userinfo", "end_session_endpoint": iss + "/logout",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) })
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		m.mu.Lock()
		g, ok := m.codes[r.Form.Get("code")]
		claims := m.claims
		m.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		full := map[string]any{"iss": iss, "aud": "panel", "sub": "u1", "nonce": g.nonce,
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
		for k, v := range claims {
			full[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": m.sign(full)})
	})
	return m
}

func (m *mockIdP) sign(claims map[string]any) string {
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: m.key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
	b, _ := json.Marshal(claims)
	obj, _ := signer.Sign(b)
	s, _ := obj.CompactSerialize()
	return s
}

// ---- harness ---------------------------------------------------------------

type env struct {
	t      *testing.T
	idp    *mockIdP
	panel  *httptest.Server
	st     *store.Store
	client *http.Client

	lastBody string // body of the last login callback
}

func newEnv(t *testing.T, mutate func(*config.Config)) *env {
	idp := newMockIdP(t)
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	e := &env{t: t, idp: idp, st: st}
	var handler http.Handler
	e.panel = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(e.panel.Close)
	pub, _ := url.Parse(e.panel.URL)
	cfg := &config.Config{
		Hostname: "smtp.example.com", PublicURL: pub, SessionTTL: time.Hour,
		PublicSubmissionPort: "587", PublicSMTPSPort: "465",
		OIDCIssuer: idp.srv.URL, OIDCClientID: "panel", OIDCClientSecret: "s",
		OIDCScopes: []string{"openid", "email"}, OIDCGroupsClaim: "groups",
		OIDCAllowedEmails: []string{"admin@example.com"},
		Upstream:          config.Upstream{Host: "127.0.0.1", Port: 1, TLSMode: "none", Timeout: time.Second},
	}
	if mutate != nil {
		mutate(cfg)
	}
	certs, err := tlsutil.NewSelfSigned(t.TempDir(), cfg.Hostname)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(cfg, st, proxy.NewUpstream(cfg.Upstream), certs, nil, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	handler = s.Handler()
	jar, _ := cookiejar.New(nil)
	e.client = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return e
}

func (e *env) get(path string) (*http.Response, string) {
	e.t.Helper()
	resp, err := e.client.Get(e.panel.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func (e *env) post(path string, form url.Values, origin string) (*http.Response, string) {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, e.panel.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// login drives the full authorization-code + PKCE flow and returns the
// callback response.
func (e *env) login(claims map[string]any) *http.Response {
	e.t.Helper()
	resp, _ := e.get("/login?next=/logs")
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || !strings.HasPrefix(loc.String(), e.idp.srv.URL+"/authorize") {
		e.t.Fatalf("login did not redirect to IdP: %v %q", err, resp.Header.Get("Location"))
	}
	q := loc.Query()
	if q.Get("client_id") != "panel" || q.Get("redirect_uri") != e.panel.URL+"/auth/callback" || q.Get("response_type") != "code" {
		e.t.Fatalf("authorize request: client_id=%q redirect_uri=%q response_type=%q", q.Get("client_id"), q.Get("redirect_uri"), q.Get("response_type"))
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" {
		e.t.Fatalf("missing PKCE/nonce: %v", q)
	}
	e.idp.mu.Lock()
	e.idp.claims = claims
	e.idp.codes["code1"] = codeGrant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge")}
	e.idp.mu.Unlock()
	resp, e.lastBody = e.get("/auth/callback?code=code1&state=" + url.QueryEscape(q.Get("state")))
	return resp
}

var csrfRe = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func (e *env) csrf() string {
	e.t.Helper()
	_, body := e.get("/")
	m := csrfRe.FindStringSubmatch(body)
	if m == nil {
		e.t.Fatal("no csrf token on dashboard")
	}
	return m[1]
}

// ---- tests -----------------------------------------------------------------

func TestUnauthenticatedRedirectsToLogin(t *testing.T) {
	e := newEnv(t, nil)
	resp, _ := e.get("/credentials/new")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login?next=%2Fcredentials%2Fnew" {
		t.Fatalf("got %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := e.get("/healthz"); resp.StatusCode != 200 {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}
	// Every self-hosted asset type the pages use must be allowed by the CSP.
	csp := resp.Header.Get("Content-Security-Policy")
	for _, d := range []string{"script-src 'self'", "style-src 'self'", "font-src 'self'"} {
		if !strings.Contains(csp, d) {
			t.Errorf("CSP %q lacks %s", csp, d)
		}
	}
	for _, p := range []string{"/static/style.css", "/static/app.js", "/static/theme.js", "/static/fonts/plex-sans.woff2", "/static/fonts/plex-mono-400.woff2"} {
		if resp, _ := e.get(p); resp.StatusCode != 200 {
			t.Errorf("GET %s = %d", p, resp.StatusCode)
		}
	}
}

func TestLoginCreateAndUseCredential(t *testing.T) {
	e := newEnv(t, nil)
	resp := e.login(map[string]any{"email": "Admin@Example.com", "email_verified": true, "name": "Ada"})
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/logs" {
		t.Fatalf("callback: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if _, body := e.get("/"); !strings.Contains(body, "Create the first login") || !strings.Contains(body, "Ada") {
		t.Fatal("dashboard not rendered for logged-in user")
	}

	form := url.Values{"username": {"nextcloud"}, "description": {"Cloud"}, "allowed_senders": {"@example.com\ncloud@other.test"},
		"rate_per_hour": {"100"}, "enabled": {"on"}}
	if resp, _ := e.post("/credentials", form, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without csrf = %d, want 403", resp.StatusCode)
	}
	form.Set("csrf", e.csrf())
	if resp, _ := e.post("/credentials", form, "https://evil.test"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin POST = %d, want 403", resp.StatusCode)
	}
	resp, body := e.post("/credentials", form, e.panel.URL)
	if resp.StatusCode != 200 {
		t.Fatalf("create = %d: %s", resp.StatusCode, body)
	}
	m := regexp.MustCompile(`id="pw">([A-Za-z0-9]{32})<`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("password not shown: %s", body)
	}
	c, err := e.st.Authenticate(context.Background(), "nextcloud", m[1])
	if err != nil {
		t.Fatalf("generated password does not authenticate: %v", err)
	}
	if c.RatePerHour != 100 || len(c.AllowedSenders) != 2 || c.CreatedBy != "admin@example.com" {
		t.Fatalf("stored credential = %+v", c)
	}

	// Duplicate is a form error, not a 500.
	if resp, body := e.post("/credentials", form, ""); resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "already taken") {
		t.Fatalf("duplicate = %d", resp.StatusCode)
	}

	// Rotate invalidates the old password.
	if resp, _ := e.post("/credentials/1/rotate", url.Values{"csrf": {form.Get("csrf")}}, ""); resp.StatusCode != 200 {
		t.Fatalf("rotate = %d", resp.StatusCode)
	}
	if _, err := e.st.Authenticate(context.Background(), "nextcloud", m[1]); err == nil {
		t.Fatal("old password still valid after rotate")
	}

	// Delete requires typing the username.
	e.post("/credentials/1/delete", url.Values{"csrf": {form.Get("csrf")}, "confirm": {"nope"}}, "")
	if _, err := e.st.GetCredential(context.Background(), 1); err != nil {
		t.Fatal("deleted without confirmation")
	}
	e.post("/credentials/1/delete", url.Values{"csrf": {form.Get("csrf")}, "confirm": {"nextcloud"}}, "")
	if _, err := e.st.GetCredential(context.Background(), 1); err == nil {
		t.Fatal("not deleted")
	}

	if _, body := e.get("/audit"); !strings.Contains(body, "credential.rotate") || !strings.Contains(body, "credential.delete") {
		t.Fatal("audit log incomplete")
	}
	if resp, _ := e.post("/tls/renew", url.Values{"csrf": {form.Get("csrf")}}, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("renew without acme = %d, want 404", resp.StatusCode)
	}
	// Field errors render next to the field.
	bad := url.Values{"csrf": {form.Get("csrf")}, "username": {"x"}, "allowed_senders": {"not-an-address"}, "enabled": {"on"}}
	if resp, body := e.post("/credentials", bad, ""); resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `id="senders-err"`) || !strings.Contains(body, `aria-invalid="true"`) {
		t.Fatalf("field error = %d", resp.StatusCode)
	} else if !regexp.MustCompile(`name="enabled"\s+checked`).MatchString(body) {
		// Regression: an early validation return used to drop the checkbox,
		// so fixing the error and resubmitting created a disabled login.
		t.Fatal("re-rendered form lost the enabled checkbox")
	}
	for _, p := range []string{"/logs", "/logs?status=sent&q=x", "/credentials/new", "/audit", "/signed-out", "/nope"} {
		resp, _ := e.get(p)
		if want := map[bool]int{true: 404, false: 200}[p == "/nope"]; resp.StatusCode != want {
			t.Fatalf("GET %s = %d, want %d", p, resp.StatusCode, want)
		}
	}

	resp, _ = e.post("/logout", url.Values{"csrf": {form.Get("csrf")}}, "")
	if !strings.HasPrefix(resp.Header.Get("Location"), e.idp.srv.URL+"/logout?") {
		t.Fatalf("logout redirect = %q", resp.Header.Get("Location"))
	}
	if resp, _ := e.get("/"); resp.StatusCode != http.StatusFound {
		t.Fatal("session survived logout")
	}
}

func TestDisallowedUsers(t *testing.T) {
	e := newEnv(t, nil)
	if resp := e.login(map[string]any{"email": "eve@example.com", "email_verified": true}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("other email = %d, want 403", resp.StatusCode)
	} else if !strings.Contains(e.lastBody, "signed in as eve@example.com") {
		t.Fatalf("reason does not name the account: %s", e.lastBody)
	}
	// authentik >= 2025.10 sends email_verified=false by default: refuse, and
	// say why instead of a generic "not allowed".
	if resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": false}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unverified email = %d, want 403", resp.StatusCode)
	} else if !strings.Contains(e.lastBody, "email_verified=false") || !strings.Contains(e.lastBody, "OIDC_ALLOW_UNVERIFIED_EMAIL") {
		t.Fatalf("reason does not explain the unverified e-mail: %s", e.lastBody)
	}
	if resp := e.login(map[string]any{"sub": "u1"}); resp.StatusCode != http.StatusForbidden || !strings.Contains(e.lastBody, "no e-mail address") {
		t.Fatalf("missing email: %d %s", resp.StatusCode, e.lastBody)
	}
	if resp, _ := e.get("/"); resp.StatusCode != http.StatusFound {
		t.Fatal("rejected user got a session")
	}
}

func TestAllowUnverifiedEmailOptIn(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.OIDCAllowUnverifiedEmail = true })
	if resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": false}); resp.StatusCode != http.StatusFound {
		t.Fatalf("opt-in: unverified listed email = %d, want 302", resp.StatusCode)
	}
	e.client.Jar, _ = cookiejar.New(nil)
	if resp := e.login(map[string]any{"email": "eve@example.com", "email_verified": false}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("opt-in must still refuse unlisted e-mails: %d", resp.StatusCode)
	}
}

func TestGroupAndDomainAllowList(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.OIDCAllowedEmails = []string{"@ops.example.com"}
		c.OIDCAllowedGroups = []string{"smtp-admins"}
	})
	if resp := e.login(map[string]any{"email": "bob@ops.example.com"}); resp.StatusCode != http.StatusFound {
		t.Fatalf("domain match = %d", resp.StatusCode)
	}
	e.client.Jar, _ = cookiejar.New(nil)
	if resp := e.login(map[string]any{"email": "carol@elsewhere.test", "groups": []string{"staff", "smtp-admins"}}); resp.StatusCode != http.StatusFound {
		t.Fatalf("group match = %d", resp.StatusCode)
	}
	e.client.Jar, _ = cookiejar.New(nil)
	if resp := e.login(map[string]any{"email": "dan@elsewhere.test", "groups": []string{"staff"}}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("no match = %d", resp.StatusCode)
	}
}

func TestCallbackRejectsForgedState(t *testing.T) {
	e := newEnv(t, nil)
	e.get("/login")
	if resp, _ := e.get("/auth/callback?code=x&state=forged"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("forged state = %d", resp.StatusCode)
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{"/logs": "/logs", "": "/", "//evil.test": "/", "https://evil.test": "/", `/\evil.test`: "/"} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}
