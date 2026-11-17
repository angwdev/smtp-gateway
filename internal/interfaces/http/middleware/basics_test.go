package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func do(r http.Handler, method, path string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, http.NoBody)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBasics(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	r := gin.New()
	r.Use(RequestID(), Logger(log), Recovery(log), SecurityHeaders(true))
	r.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, Nonce(c)) })
	r.GET("/boom", func(*gin.Context) { panic("kaboom") })

	t.Run("request id is generated, never taken from the client", func(t *testing.T) {
		w := do(r, http.MethodGet, "/ok", map[string]string{"X-Request-Id": "attacker\nvalue"})
		id := w.Header().Get("X-Request-Id")
		assert.Len(t, id, 32)
		assert.NotContains(t, id, "attacker")
	})

	t.Run("security headers with a fresh nonce per response", func(t *testing.T) {
		a, b := do(r, http.MethodGet, "/ok", nil), do(r, http.MethodGet, "/ok", nil)
		csp := a.Header().Get("Content-Security-Policy")
		nonce := a.Body.String()
		require.NotEmpty(t, nonce)
		assert.NotEqual(t, nonce, b.Body.String())
		assert.Contains(t, csp, "default-src 'none'")
		assert.Contains(t, csp, "script-src 'self'")
		assert.Contains(t, csp, "style-src-elem 'self' 'nonce-"+nonce+"'")
		assert.Contains(t, csp, "frame-ancestors 'none'")
		assert.NotContains(t, csp, "unsafe-eval")
		assert.Equal(t, "nosniff", a.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", a.Header().Get("X-Frame-Options"))
		assert.Equal(t, "max-age=31536000", a.Header().Get("Strict-Transport-Security"))
	})

	t.Run("no HSTS over plain http", func(t *testing.T) {
		plain := gin.New()
		plain.Use(SecurityHeaders(false))
		plain.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		assert.Empty(t, do(plain, http.MethodGet, "/", nil).Header().Get("Strict-Transport-Security"))
	})

	t.Run("a panic becomes a JSON 500 and is logged with its request id", func(t *testing.T) {
		logs.Reset()
		w := do(r, http.MethodGet, "/boom", nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.JSONEq(t, `{"code":"INTERNAL_ERROR","message":"An internal error occurred."}`, w.Body.String())
		assert.Contains(t, logs.String(), "kaboom")
		assert.Contains(t, logs.String(), w.Header().Get("X-Request-Id"))
	})

	t.Run("access log has no query string", func(t *testing.T) {
		logs.Reset()
		do(r, http.MethodGet, "/ok?q=secret@example.com", nil)
		assert.True(t, strings.Contains(logs.String(), "path=/ok"))
		assert.NotContains(t, logs.String(), "secret@example.com")
	})
}
