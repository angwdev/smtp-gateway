//go:build integration

package http

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	nethttp "net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptoken "github.com/angwdev/smtp-gateway/internal/application/apitoken"
	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	appcred "github.com/angwdev/smtp-gateway/internal/application/credential"
	appmsg "github.com/angwdev/smtp-gateway/internal/application/message"
	appsession "github.com/angwdev/smtp-gateway/internal/application/session"
	"github.com/angwdev/smtp-gateway/internal/config"
	domainaudit "github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/message"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/persistence/sqlite"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/tlsutil"
	"github.com/angwdev/smtp-gateway/internal/infrastructure/upstream"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/handler"
	"github.com/angwdev/smtp-gateway/internal/interfaces/limiter"
)

// ---- mock OIDC provider (ported unchanged from internal/web/web_test.go) --

type mockIDP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	claims map[string]any // claims for the next login
	codes  map[string]codeGrant
}

type codeGrant struct{ nonce, challenge string }

func newMockIDP(t *testing.T) *mockIDP {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	m := &mockIDP{key: key, codes: map[string]codeGrant{}}
	mux := nethttp.NewServeMux()
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	iss := m.srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": iss, "authorization_endpoint": iss + "/authorize", "token_endpoint": iss + "/token",
			"jwks_uri": iss + "/jwks", "userinfo_endpoint": iss + "/userinfo", "end_session_endpoint": iss + "/logout",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/userinfo", func(w nethttp.ResponseWriter, _ *nethttp.Request) { _, _ = w.Write([]byte(`{}`)) })
	mux.HandleFunc("/token", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_ = r.ParseForm()
		m.mu.Lock()
		g, ok := m.codes[r.Form.Get("code")]
		claims := m.claims
		m.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			nethttp.Error(w, `{"error":"invalid_grant"}`, nethttp.StatusBadRequest)
			return
		}
		full := map[string]any{"iss": iss, "aud": "panel", "sub": "u1", "nonce": g.nonce,
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
		for k, v := range claims {
			full[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": m.sign(full)})
	})
	return m
}

func (m *mockIDP) sign(claims map[string]any) string {
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: m.key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
	b, _ := json.Marshal(claims)
	obj, _ := signer.Sign(b)
	s, _ := obj.CompactSerialize()
	return s
}

// ---- harness ---------------------------------------------------------------

type env struct {
	t      *testing.T
	idp    *mockIDP
	panel  *httptest.Server
	audit  domainaudit.Repository
	creds  *appcred.Handler
	msgs   *appmsg.Handler
	client *nethttp.Client // cookie jar, does not follow redirects
	engine *gin.Engine
	oidc   *OIDC
	certs  *tlsutil.Loader
	db     *sqlite.DB
}

func newEnv(t *testing.T, mutate func(*config.Config)) *env {
	t.Helper()
	idp := newMockIDP(t)

	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Late-bound handler: the engine needs the panel's own URL (for the OIDC
	// redirect_uri) before it can be built, and the panel needs the engine.
	var engine *gin.Engine
	panel := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { engine.ServeHTTP(w, r) }))
	t.Cleanup(panel.Close)
	pub, err := url.Parse(panel.URL)
	require.NoError(t, err)

	cfg := &config.Config{
		Hostname: "smtp.example.com", PublicURL: pub, SessionTTL: time.Hour,
		PublicSubmissionPort: "587", PublicSMTPSPort: "465",
		OIDCIssuer: idp.srv.URL, OIDCClientID: "panel", OIDCClientSecret: "s",
		OIDCScopes: []string{"openid", "email"}, OIDCGroupsClaim: "groups",
		OIDCAllowedEmails: []string{"admin@example.com"},
		Upstream:          config.Upstream{Host: "127.0.0.1", Port: 1, TLSMode: "none", Timeout: time.Second},
		AuthFailLimit:     3,
		AuthFailWindow:    time.Minute,
	}
	if mutate != nil {
		mutate(cfg)
	}

	auditRepo := sqlite.NewAuditRepository(db)
	recorder := appaudit.NewRecorder(auditRepo, slog.New(slog.DiscardHandler))
	creds := appcred.NewHandler(sqlite.NewCredentialRepository(db), recorder)
	msgs := appmsg.NewHandler(sqlite.NewMessageRepository(db), appmsg.ContentHeaders)
	sessions := appsession.NewHandler(sqlite.NewSessionRepository(db), cfg.SessionTTL)
	tokens := apptoken.NewHandler(sqlite.NewAPITokenRepository(db), recorder)
	auditQueries := appaudit.NewQueries(auditRepo)

	certs, err := tlsutil.NewSelfSigned(t.TempDir(), cfg.Hostname)
	require.NoError(t, err)
	up := upstream.New(cfg.Upstream)
	log := slog.New(slog.DiscardHandler)
	oidcSvc := NewOIDC(cfg, []byte("0123456789abcdef0123456789abcdef"), sessions, recorder, log)

	r, err := NewRouter(Deps{
		Config: cfg, Log: log, UI: fstest.MapFS{"index.html": {Data: []byte("<html>__CSP_NONCE__</html>")}},
		OIDC: oidcSvc, Sessions: sessions, Tokens: tokens, Recorder: recorder,
		Fails:       limiter.NewFailLimiter(cfg.AuthFailLimit, cfg.AuthFailWindow),
		APILimiter:  limiter.NewPerIP(1000, 1000),
		AuthLimiter: limiter.NewPerIP(1000, 1000),

		Credentials: handler.NewCredentialHandler(creds),
		Messages:    handler.NewMessageHandler(msgs),
		Audit:       handler.NewAuditHandler(auditQueries),
		TokenAPI:    handler.NewTokenHandler(tokens),
		Session:     handler.NewSessionHandler(),
		Overview:    handler.NewOverviewHandler(msgs, creds, up, certs, nil, cfg, "test"),
		System:      handler.NewSystemHandler(up, nil, cfg, db, recorder, log),
	})
	require.NoError(t, err)
	engine = r

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &nethttp.Client{Jar: jar, CheckRedirect: func(*nethttp.Request, []*nethttp.Request) error { return nethttp.ErrUseLastResponse }}

	return &env{t: t, idp: idp, panel: panel, audit: auditRepo, creds: creds, msgs: msgs, client: client, engine: engine,
		oidc: oidcSvc, certs: certs, db: db}
}

// ---- small HTTP helpers -----------------------------------------------------

func (e *env) do(method, path string, body any, header map[string]string) (resp *nethttp.Response, respBody []byte) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(e.t, err)
		r = bytes.NewReader(b)
	}
	req, err := nethttp.NewRequest(method, e.panel.URL+path, r)
	require.NoError(e.t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err = e.client.Do(req) //nolint:bodyclose // closed below; the response is also returned for its status and headers
	require.NoError(e.t, err)
	defer resp.Body.Close()
	respBody, _ = io.ReadAll(resp.Body)
	return resp, respBody
}

// doRaw sends a request with the given raw body string and no helper
// encoding, to exercise malformed-JSON handling.
func (e *env) doRaw(method, path, body string, header map[string]string) (resp *nethttp.Response, respBody []byte) {
	e.t.Helper()
	req, err := nethttp.NewRequest(method, e.panel.URL+path, strings.NewReader(body))
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err = e.client.Do(req) //nolint:bodyclose // closed below; the response is also returned for its status and headers
	require.NoError(e.t, err)
	defer resp.Body.Close()
	respBody, _ = io.ReadAll(resp.Body)
	return resp, respBody
}

// bearer sends a request authenticated only by an API token, with no
// cookies at all, like a non-browser caller.
func (e *env) bearer(method, path, token string, body any) (resp *nethttp.Response, respBody []byte) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(e.t, err)
		r = bytes.NewReader(b)
	}
	req, err := nethttp.NewRequest(method, e.panel.URL+path, r)
	require.NoError(e.t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = (&nethttp.Client{}).Do(req) //nolint:bodyclose // closed below; the response is also returned for its status and headers
	require.NoError(e.t, err)
	defer resp.Body.Close()
	respBody, _ = io.ReadAll(resp.Body)
	return resp, respBody
}

func (e *env) jsonInto(raw []byte, v any) {
	e.t.Helper()
	require.NoError(e.t, json.Unmarshal(raw, v))
}

func (e *env) cookieValue(name string) string {
	e.t.Helper()
	u, err := url.Parse(e.panel.URL)
	require.NoError(e.t, err)
	for _, c := range e.client.Jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// login drives the full authorization-code + PKCE flow and returns the
// callback response (ported from internal/web/web_test.go login).
func (e *env) login(claims map[string]any) *nethttp.Response {
	e.t.Helper()
	resp, _ := e.do(nethttp.MethodGet, "/login?next=/logs", nil, nil)
	loc, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(e.t, err)
	require.True(e.t, strings.HasPrefix(loc.String(), e.idp.srv.URL+"/authorize"), "login did not redirect to the IdP: %q", resp.Header.Get("Location"))
	q := loc.Query()
	require.Equal(e.t, "panel", q.Get("client_id"))
	require.Equal(e.t, e.panel.URL+"/auth/callback", q.Get("redirect_uri"))
	require.Equal(e.t, "code", q.Get("response_type"))
	require.Equal(e.t, "S256", q.Get("code_challenge_method"))
	require.NotEmpty(e.t, q.Get("nonce"))

	e.idp.mu.Lock()
	e.idp.claims = claims
	e.idp.codes["code1"] = codeGrant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge")}
	e.idp.mu.Unlock()
	resp, _ = e.do(nethttp.MethodGet, "/auth/callback?code=code1&state="+url.QueryEscape(q.Get("state")), nil, nil)
	return resp
}

func (e *env) auditByAction(action string) []*domainaudit.Event {
	e.t.Helper()
	events, err := e.audit.List(context.Background(), domainaudit.Filter{Action: action, Limit: 200})
	require.NoError(e.t, err)
	return events
}

// ---- tests -----------------------------------------------------------------

func TestSignInAndSession(t *testing.T) {
	e := newEnv(t, nil)
	sessionCookie := SessionCookieName(false)

	resp, body := e.do(nethttp.MethodGet, "/api/v1/session", nil, nil) //nolint:bodyclose // do() already drains and closes the body
	assert.Equal(t, 401, resp.StatusCode)
	assert.Contains(t, string(body), `"UNAUTHORIZED"`)

	resp, _ = e.do(nethttp.MethodGet, "/", nil, nil) //nolint:bodyclose // do() already drains and closes the body
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	resp = e.login(map[string]any{"email": "admin@example.com", "email_verified": true, "name": "Ana"}) //nolint:bodyclose // login() calls do() which already drains and closes the body
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, "/logs", resp.Header.Get("Location"))
	u, _ := url.Parse(e.panel.URL)
	var found *nethttp.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			found = c
		}
	}
	require.NotNil(t, found, "no session cookie set")
	assert.True(t, found.HttpOnly)
	assert.Equal(t, nethttp.SameSiteLaxMode, found.SameSite)

	_, body = e.do(nethttp.MethodGet, "/api/v1/session", nil, nil)
	var sess struct {
		Auth string `json:"auth"`
		User struct {
			Email string `json:"email"`
		} `json:"user"`
		Scope string `json:"scope"`
	}
	e.jsonInto(body, &sess)
	assert.Equal(t, "session", sess.Auth)
	assert.Equal(t, "admin@example.com", sess.User.Email)
	assert.Equal(t, "write", sess.Scope)

	logins := e.auditByAction("session.login")
	require.Len(t, logins, 1)
	assert.Equal(t, "admin@example.com", logins[0].Actor)
	assert.NotEmpty(t, logins[0].IP)
	assert.NotEmpty(t, logins[0].UserAgent)

	old := e.cookieValue(sessionCookie)
	resp, body = e.do(nethttp.MethodDelete, "/api/v1/session", nil, map[string]string{"Sec-Fetch-Site": "same-origin"})
	assert.Equal(t, 200, resp.StatusCode)
	var out struct {
		RedirectURL string `json:"redirect_url"`
	}
	e.jsonInto(body, &out)
	assert.True(t, strings.HasPrefix(out.RedirectURL, e.idp.srv.URL+"/logout"), "redirect_url = %q", out.RedirectURL)

	e.client.Jar.SetCookies(u, []*nethttp.Cookie{{Name: sessionCookie, Value: old}})
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/session", nil, nil)
	assert.Equal(t, 401, resp.StatusCode, "the revoked session must not work even with the old cookie value")

	logouts := e.auditByAction("session.logout")
	assert.Len(t, logouts, 1)

	t.Run("a refused user", func(t *testing.T) {
		jar, _ := cookiejar.New(nil)
		e.client.Jar = jar
		resp := e.login(map[string]any{"email": "mallory@example.com", "email_verified": true})
		assert.Equal(t, 302, resp.StatusCode)
		loc := resp.Header.Get("Location")
		assert.True(t, strings.HasPrefix(loc, "/signed-out#error="), "location = %q", loc)
		for _, c := range e.client.Jar.Cookies(u) {
			assert.NotEqual(t, sessionCookie, c.Name)
		}
		denied := e.auditByAction("session.login_denied")
		require.NotEmpty(t, denied)
		last := denied[0]
		assert.Equal(t, domainaudit.ResultDenied, last.Result)
		assert.Contains(t, last.Detail, "not in the allowed e-mails or groups")
	})

	t.Run("TestAllowUnverifiedEmailOptIn", func(t *testing.T) {
		e := newEnv(t, func(c *config.Config) { c.OIDCAllowUnverifiedEmail = true })
		resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": false})
		assert.Equal(t, 302, resp.StatusCode, "opt-in: unverified listed email")
		assert.Equal(t, "/logs", resp.Header.Get("Location"))

		jar, _ := cookiejar.New(nil)
		e.client.Jar = jar
		resp = e.login(map[string]any{"email": "eve@example.com", "email_verified": false})
		assert.Equal(t, 302, resp.StatusCode)
		assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/signed-out#error="), "opt-in must still refuse unlisted e-mails")
	})

	t.Run("TestGroupAndDomainAllowList", func(t *testing.T) {
		e := newEnv(t, func(c *config.Config) {
			c.OIDCAllowedEmails = []string{"@ops.example.com"}
			c.OIDCAllowedGroups = []string{"smtp-admins"}
		})
		resp := e.login(map[string]any{"email": "bob@ops.example.com"})
		assert.Equal(t, 302, resp.StatusCode, "domain match")
		assert.Equal(t, "/logs", resp.Header.Get("Location"))

		jar, _ := cookiejar.New(nil)
		e.client.Jar = jar
		resp = e.login(map[string]any{"email": "carol@elsewhere.test", "groups": []string{"staff", "smtp-admins"}})
		assert.Equal(t, 302, resp.StatusCode, "group match")
		assert.Equal(t, "/logs", resp.Header.Get("Location"))

		jar, _ = cookiejar.New(nil)
		e.client.Jar = jar
		resp = e.login(map[string]any{"email": "dan@elsewhere.test", "groups": []string{"staff"}})
		assert.Equal(t, 302, resp.StatusCode)
		assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/signed-out#error="), "no match")
	})

	t.Run("TestCallbackRejectsForgedState", func(t *testing.T) {
		e := newEnv(t, nil)
		e.do(nethttp.MethodGet, "/login", nil, nil)
		resp, _ := e.do(nethttp.MethodGet, "/auth/callback?code=x&state=forged", nil, nil)
		assert.Equal(t, 302, resp.StatusCode)
		assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/signed-out#error="))
	})

	t.Run("TestSafeNext", func(t *testing.T) {
		for in, want := range map[string]string{"/logs": "/logs", "": "/", "//evil.test": "/", "https://evil.test": "/", `/\evil.test`: "/"} {
			assert.Equal(t, want, safeNext(in), "safeNext(%q)", in)
		}
	})
}

func TestCredentialAPI(t *testing.T) {
	e := newEnv(t, nil)
	resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": true, "name": "Ana"})
	require.Equal(t, 302, resp.StatusCode)

	resp, body := e.do(nethttp.MethodPost, "/api/v1/credentials", map[string]any{"username": "nextcloud", "allowed_senders": []string{"@example.com"}}, nil)
	require.Equal(t, 201, resp.StatusCode, string(body))
	var created struct {
		Credential struct {
			Enabled bool `json:"enabled"`
		} `json:"credential"`
		Secret string `json:"secret"`
	}
	e.jsonInto(body, &created)
	assert.Len(t, created.Secret, 32)
	assert.True(t, created.Credential.Enabled)
	assert.NotContains(t, string(body), "secret_hash")

	resp, body = e.do(nethttp.MethodGet, "/api/v1/credentials/1", nil, nil)
	assert.Equal(t, 200, resp.StatusCode)
	var fetched struct {
		Username string `json:"username"`
	}
	e.jsonInto(body, &fetched)
	assert.Equal(t, "nextcloud", fetched.Username)

	resp, _ = e.do(nethttp.MethodPost, "/api/v1/credentials", map[string]any{"username": "nextcloud"}, nil)
	assert.Equal(t, 409, resp.StatusCode)

	resp, body = e.do(nethttp.MethodPost, "/api/v1/credentials", map[string]any{"username": "bad name"}, nil)
	assert.Equal(t, 400, resp.StatusCode)
	var verr struct {
		Code    string            `json:"code"`
		Details map[string]string `json:"details"`
	}
	e.jsonInto(body, &verr)
	assert.Equal(t, "VALIDATION_ERROR", verr.Code)
	assert.Equal(t, "username", verr.Details["field"])

	resp, body = e.do(nethttp.MethodPost, "/api/v1/credentials", map[string]any{}, nil)
	assert.Equal(t, 400, resp.StatusCode)
	e.jsonInto(body, &verr)
	assert.Equal(t, "username", verr.Details["field"])

	resp, _ = e.doRaw(nethttp.MethodPost, "/api/v1/credentials", "not json", nil)
	assert.Equal(t, 400, resp.StatusCode)

	_, body = e.do(nethttp.MethodGet, "/api/v1/credentials", nil, nil)
	var list struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	e.jsonInto(body, &list)
	require.Len(t, list.Items, 1)

	resp, _ = e.do(nethttp.MethodGet, "/api/v1/credentials/999", nil, nil)
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/credentials/abc", nil, nil)
	assert.Equal(t, 404, resp.StatusCode)

	resp, _ = e.do(nethttp.MethodPatch, "/api/v1/credentials/1", map[string]any{"rate_per_hour": 5}, nil)
	assert.Equal(t, 200, resp.StatusCode)
	updates := e.auditByAction("credential.update")
	require.Len(t, updates, 1)
	assert.JSONEq(t, `{"rate_per_hour":{"from":0,"to":5}}`, changesJSON(t, updates[0].Changes))

	resp, _ = e.do(nethttp.MethodPatch, "/api/v1/credentials/1", map[string]any{"enabled": false}, nil)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Len(t, e.auditByAction("credential.disable"), 1)

	resp, _ = e.do(nethttp.MethodPatch, "/api/v1/credentials/abc", map[string]any{"enabled": true}, nil)
	assert.Equal(t, 404, resp.StatusCode, "a non-numeric id must 404 before any binding happens")
	resp, _ = e.doRaw(nethttp.MethodPatch, "/api/v1/credentials/1", "not json", nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodPatch, "/api/v1/credentials/999", map[string]any{"enabled": true}, nil)
	assert.Equal(t, 404, resp.StatusCode, "patching a missing credential must 404")

	resp, _ = e.do(nethttp.MethodPost, "/api/v1/credentials/abc/rotate", nil, nil)
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodPost, "/api/v1/credentials/999/rotate", nil, nil)
	assert.Equal(t, 404, resp.StatusCode, "rotating a missing credential must 404")

	resp, body = e.do(nethttp.MethodPost, "/api/v1/credentials/1/rotate", nil, nil)
	assert.Equal(t, 200, resp.StatusCode)
	var rotated struct {
		Secret string `json:"secret"`
	}
	e.jsonInto(body, &rotated)
	assert.Len(t, rotated.Secret, 32)
	assert.NotEqual(t, created.Secret, rotated.Secret)
	_, err := e.creds.Authenticate(context.Background(), appcred.AuthenticateCommand{Username: "nextcloud", Password: created.Secret})
	assert.ErrorIs(t, err, shared.ErrInvalidLogin)

	resp, _ = e.do(nethttp.MethodDelete, "/api/v1/credentials/abc", nil, nil)
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodDelete, "/api/v1/credentials/999", nil, nil)
	assert.Equal(t, 404, resp.StatusCode, "deleting a missing credential must 404")

	resp, _ = e.do(nethttp.MethodDelete, "/api/v1/credentials/1", nil, nil)
	assert.Equal(t, 204, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/credentials/1", nil, nil)
	assert.Equal(t, 404, resp.StatusCode)
	deletes := e.auditByAction("credential.delete")
	require.Len(t, deletes, 1)
	assert.Equal(t, "nextcloud", deletes[0].TargetLabel)

	resp, body = e.do(nethttp.MethodPost, "/api/v1/credentials", map[string]any{"username": "other"}, map[string]string{"Sec-Fetch-Site": "cross-site"})
	assert.Equal(t, 403, resp.StatusCode, string(body))
	_, body = e.do(nethttp.MethodGet, "/api/v1/credentials", nil, nil)
	e.jsonInto(body, &list)
	assert.Empty(t, list.Items, "the cross-site POST must not have created anything")
}

func changesJSON(t *testing.T, changes map[string]domainaudit.Change) string {
	t.Helper()
	b, err := json.Marshal(changes)
	require.NoError(t, err)
	return string(b)
}

func TestTokenAPI(t *testing.T) {
	e := newEnv(t, nil)
	resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": true, "name": "Ana"})
	require.Equal(t, 302, resp.StatusCode)

	resp, body := e.do(nethttp.MethodPost, "/api/v1/tokens", map[string]any{"name": "grafana", "scope": "read"}, nil)
	require.Equal(t, 201, resp.StatusCode, string(body))
	var created struct {
		Secret string `json:"secret"`
	}
	e.jsonInto(body, &created)
	assert.True(t, strings.HasPrefix(created.Secret, "sgw_"))
	assert.Len(t, created.Secret, 44)

	resp, _ = e.doRaw(nethttp.MethodPost, "/api/v1/tokens", "not json", nil)
	assert.Equal(t, 400, resp.StatusCode)

	resp, _ = e.bearer(nethttp.MethodGet, "/api/v1/overview", created.Secret, nil)
	assert.Equal(t, 200, resp.StatusCode)
	resp, _ = e.bearer(nethttp.MethodGet, "/api/v1/messages", created.Secret, nil)
	assert.Equal(t, 200, resp.StatusCode)
	resp, body = e.bearer(nethttp.MethodGet, "/api/v1/session", created.Secret, nil)
	assert.Equal(t, 200, resp.StatusCode)
	var sess struct {
		Auth      string `json:"auth"`
		TokenName string `json:"token_name"`
	}
	e.jsonInto(body, &sess)
	assert.Equal(t, "token", sess.Auth)
	assert.Equal(t, "grafana", sess.TokenName)
	resp, _ = e.bearer(nethttp.MethodPost, "/api/v1/credentials", created.Secret, map[string]any{"username": "x"})
	assert.Equal(t, 403, resp.StatusCode)
	resp, _ = e.bearer(nethttp.MethodGet, "/api/v1/tokens", created.Secret, nil)
	assert.Equal(t, 403, resp.StatusCode)
	resp, _ = e.bearer(nethttp.MethodDelete, "/api/v1/session", created.Secret, nil)
	assert.Equal(t, 403, resp.StatusCode)

	calls := e.auditByAction("api.call")
	var sawOverview, sawDenied bool
	for _, c := range calls {
		if c.Actor == "grafana" && c.ActorType == domainaudit.ActorToken && c.Detail == "GET /api/v1/overview" {
			sawOverview = true
		}
		if c.Result == domainaudit.ResultDenied {
			sawDenied = true
		}
	}
	assert.True(t, sawOverview, "no api.call row for the overview GET")
	assert.True(t, sawDenied, "no api.call row for the refused POST")

	resp, body = e.do(nethttp.MethodPost, "/api/v1/tokens", map[string]any{"name": "writer", "scope": "write"}, nil)
	require.Equal(t, 201, resp.StatusCode, string(body))
	var writer struct {
		Secret string `json:"secret"`
	}
	e.jsonInto(body, &writer)
	callsBefore := len(e.auditByAction("api.call"))
	resp, _ = e.bearer(nethttp.MethodPost, "/api/v1/credentials", writer.Secret, map[string]any{"username": "fromtoken"})
	require.Equal(t, 201, resp.StatusCode)
	creates := e.auditByAction("credential.create")
	require.NotEmpty(t, creates)
	assert.Equal(t, domainaudit.ActorToken, creates[len(creates)-1].ActorType)
	assert.Equal(t, callsBefore, len(e.auditByAction("api.call")), "a self-audited use case must not also get an api.call row")

	// Revoke before the brute-force check below: once the per-IP failure
	// limiter blocks an address it refuses every bearer request, valid or
	// not, until the window elapses, so a revoked-token check could not
	// otherwise tell "revoked" apart from "the IP is locked out".
	_, body = e.do(nethttp.MethodGet, "/api/v1/tokens", nil, nil)
	var list struct {
		Items []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	e.jsonInto(body, &list)
	var grafanaID int64
	for _, it := range list.Items {
		if it.Name == "grafana" {
			grafanaID = it.ID
		}
	}
	require.NotZero(t, grafanaID)
	resp, _ = e.do(nethttp.MethodDelete, "/api/v1/tokens/abc", nil, nil)
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodDelete, "/api/v1/tokens/999999", nil, nil)
	assert.Equal(t, 404, resp.StatusCode, "revoking a missing token must 404")
	resp, _ = e.do(nethttp.MethodDelete, fmt.Sprintf("/api/v1/tokens/%d", grafanaID), nil, nil)
	assert.Equal(t, 204, resp.StatusCode)
	resp, _ = e.bearer(nethttp.MethodGet, "/api/v1/overview", created.Secret, nil)
	assert.Equal(t, 401, resp.StatusCode) // one failure already counted against the limiter below

	// AuthFailLimit is 3 and the revoked-token check above already used one
	// of them, so only two more bad attempts are free before the third is
	// blocked outright (limiter.FailLimiter.Blocked trips at count==limit).
	for range 2 {
		resp, _ = e.bearer(nethttp.MethodGet, "/api/v1/overview", "sgw_wrongwrongwrongwrongwrongwrongwrongwrong", nil)
		assert.Equal(t, 401, resp.StatusCode)
	}
	resp, _ = e.bearer(nethttp.MethodGet, "/api/v1/overview", "sgw_wrongwrongwrongwrongwrongwrongwrongwrong", nil)
	assert.Equal(t, 429, resp.StatusCode)
	assert.NotEmpty(t, e.auditByAction("token.auth_failed"))

	past := time.Now().Add(-time.Hour)
	resp, body = e.do(nethttp.MethodPost, "/api/v1/tokens", map[string]any{"name": "expired", "scope": "read", "expires_at": past.Format(time.RFC3339)}, nil)
	assert.Equal(t, 400, resp.StatusCode)
	var verr struct {
		Details map[string]string `json:"details"`
	}
	e.jsonInto(body, &verr)
	assert.Equal(t, "expires_at", verr.Details["field"])
}

// forwardedForAttempt sends a bad bearer token with a forged
// X-Forwarded-For header and returns the token.auth_failed audit row it
// produced, so the recorded IP can be checked against the socket peer
// (section 2.4, "Client IP").
func (e *env) forwardedForAttempt(t *testing.T, forged string) *domainaudit.Event {
	t.Helper()
	before := len(e.auditByAction("token.auth_failed"))
	req, err := nethttp.NewRequest(nethttp.MethodGet, e.panel.URL+"/api/v1/overview", nethttp.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer sgw_wrongwrongwrongwrongwrongwrongwrongwrong")
	req.Header.Set("X-Forwarded-For", forged)
	resp, err := (&nethttp.Client{}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck // test cleanup
	assert.Equal(t, 401, resp.StatusCode)
	events := e.auditByAction("token.auth_failed")
	require.Len(t, events, before+1)
	return events[len(events)-1]
}

func TestClientIPIgnoresForwardedForWithoutTrustedProxy(t *testing.T) {
	// No WEB_TRUSTED_PROXIES configured: the forged header must be ignored
	// and the recorded IP must be the real socket peer, not the header.
	e := newEnv(t, nil)
	event := e.forwardedForAttempt(t, "203.0.113.9")
	assert.NotEqual(t, "203.0.113.9", event.IP, "a forged X-Forwarded-For must be ignored without a trusted proxy")
	assert.Equal(t, "127.0.0.1", event.IP, "the recorded IP must be the socket peer")
}

func TestClientIPUsesForwardedForInsideTrustedProxy(t *testing.T) {
	// The test harness connects over 127.0.0.1, so trusting that /32 makes
	// the peer a configured trusted proxy and X-Forwarded-For is honoured.
	e := newEnv(t, func(c *config.Config) { c.TrustedProxies = []string{"127.0.0.1/32"} })
	event := e.forwardedForAttempt(t, "203.0.113.9")
	assert.Equal(t, "203.0.113.9", event.IP, "inside a trusted proxy CIDR, X-Forwarded-For must be used")
}

func TestReadEndpoints(t *testing.T) {
	e := newEnv(t, nil)
	resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": true, "name": "Ana"})
	require.Equal(t, 302, resp.StatusCode)

	ctx := context.Background()
	require.NoError(t, e.msgs.Record(ctx, appmsg.RecordCommand{RemoteIP: "203.0.113.1", MailFrom: "a@b.test", RcptTo: []string{"c@d.test"}, Status: message.StatusSent, Header: []byte("Subject: hi\r\n\r\n")}))
	require.NoError(t, e.msgs.Record(ctx, appmsg.RecordCommand{RemoteIP: "203.0.113.1", MailFrom: "a@b.test", RcptTo: []string{"c@d.test"}, Status: message.StatusFailed}))
	require.NoError(t, e.msgs.Record(ctx, appmsg.RecordCommand{RemoteIP: "203.0.113.1", MailFrom: "a@b.test", RcptTo: []string{"c@d.test"}, Status: message.StatusSent}))

	_, body := e.do(nethttp.MethodGet, "/api/v1/messages?status=failed", nil, nil)
	var page struct {
		Items []struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
		NextBefore *int64 `json:"next_before"`
	}
	e.jsonInto(body, &page)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "failed", page.Items[0].Status)

	_, body = e.do(nethttp.MethodGet, "/api/v1/messages?limit=2", nil, nil)
	e.jsonInto(body, &page)
	require.Len(t, page.Items, 2)
	require.NotNil(t, page.NextBefore)

	_, body = e.do(nethttp.MethodGet, fmt.Sprintf("/api/v1/messages?limit=2&before=%d", *page.NextBefore), nil, nil)
	e.jsonInto(body, &page)
	require.Len(t, page.Items, 1)
	assert.Nil(t, page.NextBefore)

	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages?status=nope", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages?from=yesterday", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages?to=yesterday", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages?before=abc", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages?limit=abc", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages?credential_id=abc", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages?from=2020-01-01T00:00:00Z", nil, nil)
	assert.Equal(t, 200, resp.StatusCode, "a valid RFC 3339 from= must be accepted")
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages/abc", nil, nil)
	assert.Equal(t, 404, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/messages/999999", nil, nil)
	assert.Equal(t, 404, resp.StatusCode, "fetching a missing message must 404")

	require.NoError(t, e.msgs.Record(ctx, appmsg.RecordCommand{RemoteIP: "203.0.113.1", MailFrom: "a@b.test", Status: message.StatusSent}))
	_, body = e.do(nethttp.MethodGet, "/api/v1/messages?limit=1", nil, nil)
	e.jsonInto(body, &page)
	require.Len(t, page.Items, 1)
	var noRcptDetail struct {
		RcptTo []string `json:"rcpt_to"`
	}
	_, body = e.do(nethttp.MethodGet, fmt.Sprintf("/api/v1/messages/%d", page.Items[0].ID), nil, nil)
	e.jsonInto(body, &noRcptDetail)
	assert.Equal(t, []string{}, noRcptDetail.RcptTo, "a nil RcptTo must become an empty array, never null")

	_, body = e.do(nethttp.MethodGet, "/api/v1/messages", nil, nil)
	e.jsonInto(body, &page)
	require.NotEmpty(t, page.Items)
	var withHeaders int64
	for _, it := range page.Items {
		withHeaders = it.ID
	}
	_, body = e.do(nethttp.MethodGet, fmt.Sprintf("/api/v1/messages/%d", withHeaders), nil, nil)
	var detail struct {
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
		HeadersRaw string `json:"headers_raw"`
	}
	e.jsonInto(body, &detail)
	assert.Contains(t, detail.HeadersRaw, "Subject:")

	resp, body = e.do(nethttp.MethodPost, "/api/v1/credentials", map[string]any{"username": "relay1"}, nil)
	require.Equal(t, 201, resp.StatusCode, string(body))
	resp, _ = e.do(nethttp.MethodPatch, "/api/v1/credentials/1", map[string]any{"rate_per_hour": 7}, nil)
	require.Equal(t, 200, resp.StatusCode)

	resp, _ = e.do(nethttp.MethodGet, "/api/v1/audit?result=bogus", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/audit?from=yesterday", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/audit?to=yesterday", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/audit?before=abc", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/audit?limit=abc", nil, nil)
	assert.Equal(t, 400, resp.StatusCode)

	_, body = e.do(nethttp.MethodGet, "/api/v1/audit?action=credential.update", nil, nil)
	var changePage struct {
		Items []struct {
			Action  string                     `json:"action"`
			Changes map[string]json.RawMessage `json:"changes"`
		} `json:"items"`
	}
	e.jsonInto(body, &changePage)
	require.NotEmpty(t, changePage.Items, "the rate_per_hour patch above must show up")
	assert.Contains(t, changePage.Items[0].Changes, "rate_per_hour")

	_, body = e.do(nethttp.MethodGet, "/api/v1/audit?action=credential.", nil, nil)
	var auditPage struct {
		Items []struct {
			Action string `json:"action"`
		} `json:"items"`
	}
	e.jsonInto(body, &auditPage)
	for _, it := range auditPage.Items {
		assert.True(t, strings.HasPrefix(it.Action, "credential."))
	}

	_, body = e.do(nethttp.MethodGet, "/api/v1/audit?result=denied", nil, nil)
	e.jsonInto(body, &auditPage)

	_, body = e.do(nethttp.MethodGet, "/api/v1/overview", nil, nil)
	var ov struct {
		Series []struct {
			Hour string `json:"hour"`
		} `json:"series"`
		Credentials struct {
			Total  int `json:"total"`
			Active int `json:"active"`
		} `json:"credentials"`
		Certificate struct {
			Source string `json:"source"`
		} `json:"certificate"`
		Upstream struct {
			Addr string `json:"addr"`
		} `json:"upstream"`
	}
	e.jsonInto(body, &ov)
	assert.Len(t, ov.Series, 24)
	assert.Equal(t, "self-signed", ov.Certificate.Source)
	assert.Equal(t, "127.0.0.1:1", ov.Upstream.Addr)
	assert.Equal(t, 1, ov.Credentials.Total)
	assert.Equal(t, 1, ov.Credentials.Active, "the credential created above is enabled by default")
	assert.NotContains(t, strings.ToLower(string(body)), "password")

	resp, body = e.do(nethttp.MethodPost, "/api/v1/upstream/test", nil, nil)
	assert.Equal(t, 200, resp.StatusCode)
	var ut struct {
		OK bool `json:"ok"`
	}
	e.jsonInto(body, &ut)
	assert.False(t, ut.OK)
	assert.NotEmpty(t, e.auditByAction("upstream.test"))

	resp, _ = e.do(nethttp.MethodPost, "/api/v1/certificate/renew", nil, nil)
	assert.Equal(t, 404, resp.StatusCode)

	resp, body = e.do(nethttp.MethodGet, "/healthz", nil, nil)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "ok\n", string(body))
	resp, _ = e.do(nethttp.MethodGet, "/api/v1/openapi.yaml", nil, nil)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "yaml")

	resp, _ = e.do(nethttp.MethodPut, "/api/v1/credentials", nil, nil)
	assert.Equal(t, 405, resp.StatusCode)

	resp, _ = e.do(nethttp.MethodGet, "/api/v1/overview", nil, nil)
	assert.NotEmpty(t, resp.Header.Get("Content-Security-Policy"))
	assert.NotEmpty(t, resp.Header.Get("X-Content-Type-Options"))
	resp, _ = e.do(nethttp.MethodGet, "/", nil, nil)
	assert.NotEmpty(t, resp.Header.Get("Content-Security-Policy"))
}

// ---- additional coverage: healthz, overview certificate levels, OIDC edge cases --

func TestHealthzReportsDatabaseFailure(t *testing.T) {
	e := newEnv(t, nil)
	require.NoError(t, e.db.Close())
	resp, body := e.do(nethttp.MethodGet, "/healthz", nil, nil)
	assert.Equal(t, 503, resp.StatusCode)
	assert.Equal(t, "unavailable\n", string(body))
}

// shortCert builds a minimal self-signed certificate expiring at notAfter,
// so overview certView's day-left thresholds can be exercised deterministically.
func shortCert(t *testing.T, notAfter time.Time) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "smtp.example.com"},
		DNSNames:     []string{"smtp.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func TestOverviewCertificateLevels(t *testing.T) {
	e := newEnv(t, nil)
	resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": true})
	require.Equal(t, 302, resp.StatusCode)

	type certView struct {
		Source   string `json:"source"`
		Label    string `json:"label"`
		Level    string `json:"level"`
		DaysLeft int    `json:"days_left"`
	}
	get := func() certView {
		_, body := e.do(nethttp.MethodGet, "/api/v1/overview", nil, nil)
		var ov struct {
			Certificate certView `json:"certificate"`
		}
		e.jsonInto(body, &ov)
		return ov.Certificate
	}

	cases := []struct {
		name     string
		notAfter time.Duration
		level    string
	}{
		{"about to expire", 2 * 24 * time.Hour, "bad"},
		{"renewing soon", 10 * 24 * time.Hour, "warn"},
		{"healthy", 60 * 24 * time.Hour, "ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e.certs.Set(shortCert(t, time.Now().Add(tc.notAfter)), tlsutil.SourceFile)
			v := get()
			assert.Equal(t, "file", v.Source)
			assert.Equal(t, "Certificate file", v.Label)
			assert.Equal(t, tc.level, v.Level)
		})
	}
}

func TestCheckProviderLogsReachability(t *testing.T) {
	ctx := context.Background()
	bad := newEnv(t, func(c *config.Config) { c.OIDCIssuer = "http://127.0.0.1:1" })
	bad.oidc.CheckProvider(ctx) // unreachable: must log and return, never panic

	good := newEnv(t, nil)
	good.oidc.CheckProvider(ctx) // reachable: must discover the provider and log success
}

func TestOIDCSignAndVerify(t *testing.T) {
	o := &OIDC{stateKey: []byte("0123456789abcdef0123456789abcdef")}

	_, err := o.sign(make(chan int))
	assert.Error(t, err, "a value that cannot be marshaled must fail to sign")

	assert.Error(t, o.verify("no-dot-here", &loginState{}), "a cookie without a signature separator must fail")
	assert.Error(t, o.verify("payload.wrongsignature", &loginState{}), "a bad signature must fail")

	payload := "not-valid-base64!!!"
	raw := payload + "." + o.mac(payload)
	assert.Error(t, o.verify(raw, &loginState{}), "a signature-valid but non-base64 payload must fail to decode")

	st := loginState{State: "s", Nonce: "n"}
	signed, err := o.sign(st)
	require.NoError(t, err)
	var out loginState
	require.NoError(t, o.verify(signed, &out))
	assert.Equal(t, st, out)
}

func TestClaimedEmailFallbacks(t *testing.T) {
	assert.Equal(t, "a@b.com", claimedEmail(map[string]any{"email": "A@B.com"}))
	assert.Equal(t, "carol", claimedEmail(map[string]any{"preferred_username": "carol"}))
	assert.Equal(t, "sub-123", claimedEmail(map[string]any{"sub": "sub-123"}))
	assert.Equal(t, "unknown", claimedEmail(map[string]any{}))
}

func TestOIDCAuthorizeBranches(t *testing.T) {
	log := slog.New(slog.DiscardHandler)

	t.Run("no allow-lists accepts anyone", func(t *testing.T) {
		o := &OIDC{cfg: &config.Config{}, log: log}
		u, reason := o.authorize(map[string]any{"email": "x@y.com"})
		require.NotNil(t, u)
		assert.Empty(t, reason)
	})

	t.Run("email falls back to preferred_username then sub", func(t *testing.T) {
		o := &OIDC{cfg: &config.Config{}, log: log}
		u, _ := o.authorize(map[string]any{"preferred_username": "carol"})
		assert.Equal(t, "carol", u.Email)
		u, _ = o.authorize(map[string]any{"sub": "sub-42"})
		assert.Equal(t, "sub-42", u.Email)
	})

	t.Run("groups claim as a []any", func(t *testing.T) {
		o := &OIDC{cfg: &config.Config{OIDCAllowedGroups: []string{"admins"}, OIDCGroupsClaim: "groups"}, log: log}
		u, reason := o.authorize(map[string]any{"email": "x@y.com", "groups": []any{"staff", "admins"}})
		require.NotNil(t, u, "reason = %q", reason)
	})

	t.Run("listed but unverified email is refused with a specific reason", func(t *testing.T) {
		o := &OIDC{cfg: &config.Config{OIDCAllowedEmails: []string{"x@y.com"}}, log: log}
		u, reason := o.authorize(map[string]any{"email": "x@y.com", "email_verified": false})
		assert.Nil(t, u)
		assert.Contains(t, reason, "unverified")
	})

	t.Run("no email at all when emails are configured", func(t *testing.T) {
		o := &OIDC{cfg: &config.Config{OIDCAllowedEmails: []string{"x@y.com"}}, log: log}
		u, reason := o.authorize(map[string]any{})
		assert.Nil(t, u)
		assert.Contains(t, reason, "no e-mail address")
	})

	t.Run("groups configured but token has none", func(t *testing.T) {
		o := &OIDC{cfg: &config.Config{OIDCAllowedGroups: []string{"admins"}, OIDCGroupsClaim: "groups"}, log: log}
		u, reason := o.authorize(map[string]any{"email": "x@y.com"})
		assert.Nil(t, u)
		assert.Contains(t, reason, "OIDC_ALLOWED_GROUPS")
	})
}

// signLoginState forges a login cookie without going through /login, so
// Callback's own branches after the cookie check can be reached directly.
func signLoginState(t *testing.T, st loginState) string {
	t.Helper()
	o := &OIDC{stateKey: []byte("0123456789abcdef0123456789abcdef")}
	signed, err := o.sign(st)
	require.NoError(t, err)
	return signed
}

func (e *env) setLoginCookie(t *testing.T, value string) {
	t.Helper()
	u, err := url.Parse(e.panel.URL)
	require.NoError(t, err)
	e.client.Jar.SetCookies(u, []*nethttp.Cookie{{Name: loginCookie, Value: value}})
}

func TestCallbackWhenProviderUnreachable(t *testing.T) {
	// A forged-but-valid cookie bypasses /login, so the client's provider
	// cache is empty and Callback's own discovery call must fail cleanly.
	e := newEnv(t, func(c *config.Config) { c.OIDCIssuer = "http://127.0.0.1:1" })
	st := loginState{State: "s1", Nonce: "n1", Verifier: "v", Next: "/", Exp: time.Now().Add(time.Minute).Unix()}
	e.setLoginCookie(t, signLoginState(t, st))
	resp, _ := e.do(nethttp.MethodGet, "/auth/callback?state=s1&code=whatever", nil, nil)
	assert.Equal(t, 302, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/signed-out#error="))
}

func TestCallbackWithBogusCodeFailsExchange(t *testing.T) {
	e := newEnv(t, nil)
	st := loginState{State: "s1", Nonce: "n1", Verifier: "v", Next: "/", Exp: time.Now().Add(time.Minute).Unix()}
	e.setLoginCookie(t, signLoginState(t, st))
	resp, _ := e.do(nethttp.MethodGet, "/auth/callback?state=s1&code=never-granted", nil, nil)
	assert.Equal(t, 302, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/signed-out#error="))
}

func TestCallbackRejectsWrongAudience(t *testing.T) {
	e := newEnv(t, nil)
	resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": true, "aud": "someone-else"})
	assert.Equal(t, 302, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/signed-out#error="), "location = %q", resp.Header.Get("Location"))
}

func TestCallbackRejectsNonceMismatch(t *testing.T) {
	e := newEnv(t, nil)
	resp := e.login(map[string]any{"email": "admin@example.com", "email_verified": true, "nonce": "not-the-one-we-sent"})
	assert.Equal(t, 302, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/signed-out#error="), "location = %q", resp.Header.Get("Location"))
}
