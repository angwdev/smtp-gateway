package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/session"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

const testCookie = "sg_session"

type fakeSessions struct {
	resolve func(ctx context.Context, rawID string) (*session.Session, error)
}

func (f fakeSessions) Resolve(ctx context.Context, rawID string) (*session.Session, error) {
	return f.resolve(ctx, rawID)
}

type fakeTokens struct {
	authenticate func(ctx context.Context, raw, ip string) (*apitoken.Token, error)
}

func (f fakeTokens) Authenticate(ctx context.Context, raw, ip string) (*apitoken.Token, error) {
	return f.authenticate(ctx, raw, ip)
}

type fakeAuditor struct {
	entries []appaudit.Entry
	metas   []appaudit.Meta
}

// RecordNoisy implements NoisyAuditor.
//
//nolint:gocritic // hugeParam: keep the interface's value API (matches appaudit.Recorder)
func (f *fakeAuditor) RecordNoisy(ctx context.Context, e appaudit.Entry) {
	f.entries = append(f.entries, e)
	f.metas = append(f.metas, appaudit.MetaFrom(ctx))
}

type fakeFails struct {
	blocked    bool
	failCalls  int
	resetCalls int
}

func (f *fakeFails) Blocked(string) bool { return f.blocked }
func (f *fakeFails) Fail(string) bool    { f.failCalls++; return false }
func (f *fakeFails) Reset(string)        { f.resetCalls++ }

func newAuthRouter(sessions SessionResolver, tokens TokenAuthenticator, auditor NoisyAuditor, fails FailCounter) (*gin.Engine, *appaudit.Meta) {
	r := gin.New()
	var seen appaudit.Meta
	r.Use(RequestID(), ErrorHandler(slog.New(slog.NewTextHandler(discardWriter{}, nil))))
	r.Use(Authenticate(sessions, tokens, auditor, fails, testCookie))
	r.GET("/x", func(c *gin.Context) {
		seen = appaudit.MetaFrom(c.Request.Context())
		p := PrincipalOf(c)
		c.JSON(http.StatusOK, gin.H{"kind": p.Kind, "name": p.Name, "scope": string(p.Scope)})
	})
	return r, &seen
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestAuthenticate(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	noSessions := fakeSessions{resolve: func(context.Context, string) (*session.Session, error) {
		t.Fatal("sessions.Resolve must not be called")
		return nil, nil
	}}
	noTokens := fakeTokens{authenticate: func(context.Context, string, string) (*apitoken.Token, error) {
		t.Fatal("tokens.Authenticate must not be called")
		return nil, nil
	}}

	t.Run("no cookie, no header", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{}
		r, _ := newAuthRouter(noSessions, noTokens, auditor, fails)
		w := do(r, http.MethodGet, "/x", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Empty(t, auditor.entries)
	})

	t.Run("valid session cookie", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{}
		sessions := fakeSessions{resolve: func(context.Context, string) (*session.Session, error) {
			return &session.Session{Email: "ana@example.com", Name: "Ana", ExpiresAt: expiry}, nil
		}}
		r, seen := newAuthRouter(sessions, noTokens, auditor, fails)
		req := httpReqWithCookie(http.MethodGet, "/x", testCookie, "raw-cookie")
		w := doReq(r, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"kind":"user","name":"Ana","scope":"write"}`, w.Body.String())
		assert.Equal(t, "ana@example.com", seen.Actor)
		assert.Equal(t, audit.ActorUser, seen.ActorType)
		assert.NotEmpty(t, seen.IP)
		assert.NotEmpty(t, seen.RequestID)
	})

	t.Run("expired or unknown session", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{}
		sessions := fakeSessions{resolve: func(context.Context, string) (*session.Session, error) {
			return nil, shared.ErrUnauthorized
		}}
		r, _ := newAuthRouter(sessions, noTokens, auditor, fails)
		req := httpReqWithCookie(http.MethodGet, "/x", testCookie, "raw-cookie")
		w := doReq(r, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Authorization: Bearer token", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{}
		tokens := fakeTokens{authenticate: func(context.Context, string, string) (*apitoken.Token, error) {
			return &apitoken.Token{Name: "grafana", Scope: apitoken.ScopeRead}, nil
		}}
		r, _ := newAuthRouter(noSessions, tokens, auditor, fails)
		w := do(r, http.MethodGet, "/x", map[string]string{"Authorization": "Bearer sgw_abc"})
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"kind":"token","name":"grafana","scope":"read"}`, w.Body.String())
		assert.Equal(t, 1, fails.resetCalls)
	})

	t.Run("case-insensitive bearer scheme", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{}
		tokens := fakeTokens{authenticate: func(context.Context, string, string) (*apitoken.Token, error) {
			return &apitoken.Token{Name: "grafana", Scope: apitoken.ScopeRead}, nil
		}}
		r, _ := newAuthRouter(noSessions, tokens, auditor, fails)
		w := do(r, http.MethodGet, "/x", map[string]string{"Authorization": "bearer sgw_abc"})
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("bad token", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{}
		tokens := fakeTokens{authenticate: func(context.Context, string, string) (*apitoken.Token, error) {
			return nil, shared.ErrUnauthorized
		}}
		r, _ := newAuthRouter(noSessions, tokens, auditor, fails)
		w := do(r, http.MethodGet, "/x", map[string]string{"Authorization": "Bearer bad"})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		require.Len(t, auditor.entries, 1)
		assert.Equal(t, "token.auth_failed", auditor.entries[0].Action)
		assert.Equal(t, audit.ResultDenied, auditor.entries[0].Result)
		assert.Equal(t, 1, fails.failCalls)
	})

	t.Run("bad token and a valid session cookie in the same request", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{}
		tokens := fakeTokens{authenticate: func(context.Context, string, string) (*apitoken.Token, error) {
			return nil, shared.ErrUnauthorized
		}}
		sessions := fakeSessions{resolve: func(context.Context, string) (*session.Session, error) {
			t.Fatal("the cookie must not be consulted when Authorization is present")
			return nil, nil
		}}
		r, _ := newAuthRouter(sessions, tokens, auditor, fails)
		req := httpReqWithCookie(http.MethodGet, "/x", testCookie, "raw-cookie")
		req.Header.Set("Authorization", "Bearer bad")
		w := doReq(r, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("token store down", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{}
		tokens := fakeTokens{authenticate: func(context.Context, string, string) (*apitoken.Token, error) {
			return nil, assertErr
		}}
		r, _ := newAuthRouter(noSessions, tokens, auditor, fails)
		w := do(r, http.MethodGet, "/x", map[string]string{"Authorization": "Bearer sgw_abc"})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Empty(t, auditor.entries, "an infrastructure error is not a failed login")
		assert.Equal(t, 0, fails.failCalls)
	})

	t.Run("IP already blocked", func(t *testing.T) {
		auditor := &fakeAuditor{}
		fails := &fakeFails{blocked: true}
		r, _ := newAuthRouter(noSessions, noTokens, auditor, fails)
		w := do(r, http.MethodGet, "/x", map[string]string{"Authorization": "Bearer sgw_abc"})
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
	})
}

var assertErr = &storeDownError{}

type storeDownError struct{}

func (*storeDownError) Error() string { return "store down" }

func TestRequireScopeAndSession(t *testing.T) {
	setPrincipal := func(p *Principal) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set(keyPrincipal, p)
			c.Next()
		}
	}
	newRouter := func(p *Principal, mw gin.HandlerFunc) *gin.Engine {
		r := gin.New()
		r.Use(ErrorHandler(slog.New(slog.NewTextHandler(discardWriter{}, nil))))
		r.Use(setPrincipal(p))
		r.Use(mw)
		r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
		return r
	}

	t.Run("RequireScope(write) with a read token", func(t *testing.T) {
		r := newRouter(&Principal{Kind: KindToken, Scope: apitoken.ScopeRead}, RequireScope(apitoken.ScopeWrite))
		assert.Equal(t, http.StatusForbidden, do(r, http.MethodGet, "/x", nil).Code)
	})

	t.Run("RequireScope(read) with a read token", func(t *testing.T) {
		r := newRouter(&Principal{Kind: KindToken, Scope: apitoken.ScopeRead}, RequireScope(apitoken.ScopeRead))
		assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/x", nil).Code)
	})

	t.Run("RequireScope(write) with a user", func(t *testing.T) {
		r := newRouter(&Principal{Kind: KindUser, Scope: apitoken.ScopeWrite}, RequireScope(apitoken.ScopeWrite))
		assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/x", nil).Code)
	})

	t.Run("RequireSession() with a write token", func(t *testing.T) {
		r := newRouter(&Principal{Kind: KindToken, Scope: apitoken.ScopeWrite}, RequireSession())
		assert.Equal(t, http.StatusForbidden, do(r, http.MethodGet, "/x", nil).Code)
	})

	t.Run("RequireSession() with a user", func(t *testing.T) {
		r := newRouter(&Principal{Kind: KindUser, Scope: apitoken.ScopeWrite}, RequireSession())
		assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/x", nil).Code)
	})
}

func httpReqWithCookie(method, path, name, value string) *http.Request {
	req := httptest.NewRequest(method, path, http.NoBody)
	req.AddCookie(&http.Cookie{Name: name, Value: value})
	return req
}

func doReq(r http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
