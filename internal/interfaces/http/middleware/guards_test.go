package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/internal/interfaces/limiter"
	"github.com/angwdev/smtp-gateway/pkg/response"
)

func crossOriginRouter() *gin.Engine {
	r := gin.New()
	r.Use(ErrorHandler(slog.New(slog.NewTextHandler(discardWriter{}, nil))))
	r.Use(CrossOrigin())
	r.Any("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestCrossOrigin(t *testing.T) {
	r := crossOriginRouter()

	t.Run("POST cross-site is rejected", func(t *testing.T) {
		w := do(r, http.MethodPost, "/x", map[string]string{"Sec-Fetch-Site": "cross-site"})
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), response.CodeForbidden)
	})

	t.Run("POST same-origin passes", func(t *testing.T) {
		w := do(r, http.MethodPost, "/x", map[string]string{"Sec-Fetch-Site": "same-origin"})
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST with a mismatched Origin host is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/x", http.NoBody)
		req.Host = "panel.example.com"
		req.Header.Set("Origin", "https://evil.example")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("POST with neither header passes (curl, API clients)", func(t *testing.T) {
		w := do(r, http.MethodPost, "/x", nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET cross-site passes (safe method)", func(t *testing.T) {
		w := do(r, http.MethodGet, "/x", map[string]string{"Sec-Fetch-Site": "cross-site"})
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRateLimit(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler(slog.New(slog.NewTextHandler(discardWriter{}, nil))))
	r.Use(RateLimit(limiter.NewPerIP(1, 2), 1))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/x", nil).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/x", nil).Code)
	w := do(r, http.MethodGet, "/x", nil)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "1", w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), response.CodeRateLimited)
}

func TestRateLimitAdvertisesItsWindow(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler(slog.New(slog.NewTextHandler(discardWriter{}, nil))))
	r.Use(RateLimit(limiter.NewPerIP(1, 1), 60))
	r.GET("/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/login", nil).Code)
	w := do(r, http.MethodGet, "/login", nil)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "60", w.Header().Get("Retry-After"), "a limiter with a known window advertises it")
}

func auditCallsRouter(auditor NoisyAuditor, principal *Principal, handler gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(ErrorHandler(slog.New(slog.NewTextHandler(discardWriter{}, nil))))
	r.Use(func(c *gin.Context) {
		if principal != nil {
			c.Set(keyPrincipal, principal)
		}
		c.Next()
	})
	r.Use(AuditCalls(auditor))
	r.GET("/x", handler)
	return r
}

func TestAuditCalls(t *testing.T) {
	tokenPrincipal := &Principal{Kind: KindToken, Name: "grafana", Scope: apitoken.ScopeRead}
	userPrincipal := &Principal{Kind: KindUser, Name: "ana"}

	t.Run("an unmarked handler leaves one api.call row", func(t *testing.T) {
		auditor := &fakeAuditor{}
		r := auditCallsRouter(auditor, tokenPrincipal, func(c *gin.Context) { c.Status(http.StatusOK) })
		w := do(r, http.MethodGet, "/x?y=1", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		if assert.Len(t, auditor.entries, 1) {
			assert.Equal(t, "api.call", auditor.entries[0].Action)
			assert.Equal(t, "GET /x?y=1", auditor.entries[0].Detail)
			assert.Equal(t, audit.ResultOK, auditor.entries[0].Result)
		}
	})

	t.Run("a handler that fails with forbidden records a denied call", func(t *testing.T) {
		auditor := &fakeAuditor{}
		r := auditCallsRouter(auditor, tokenPrincipal, func(c *gin.Context) { Fail(c, shared.ErrForbidden) })
		w := do(r, http.MethodGet, "/x", nil)
		assert.Equal(t, http.StatusForbidden, w.Code)
		if assert.Len(t, auditor.entries, 1) {
			assert.Equal(t, audit.ResultDenied, auditor.entries[0].Result)
		}
	})

	t.Run("a handler that marks itself audited leaves no api.call row", func(t *testing.T) {
		auditor := &fakeAuditor{}
		r := auditCallsRouter(auditor, tokenPrincipal, func(c *gin.Context) {
			MarkAudited(c)
			c.Status(http.StatusOK)
		})
		do(r, http.MethodGet, "/x", nil)
		assert.Empty(t, auditor.entries)
	})

	t.Run("a user principal leaves no api.call row", func(t *testing.T) {
		auditor := &fakeAuditor{}
		r := auditCallsRouter(auditor, userPrincipal, func(c *gin.Context) { c.Status(http.StatusOK) })
		do(r, http.MethodGet, "/x", nil)
		assert.Empty(t, auditor.entries)
	})
}
