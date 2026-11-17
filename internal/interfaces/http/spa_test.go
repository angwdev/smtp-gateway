package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

func spaEngine(files fstest.MapFS) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.SecurityHeaders(true), middleware.ErrorHandler(nil))
	r.NoRoute(SPA(files))
	return r
}

func get(r *gin.Engine, path string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(nethttp.MethodGet, path, nethttp.NoBody)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSPA(t *testing.T) {
	files := fstest.MapFS{
		"index.html":              {Data: []byte(`<meta property="csp-nonce" nonce="__CSP_NONCE__"><script type="module" nonce="__CSP_NONCE__" src="/assets/app-abc123.js"></script>`)},
		"assets/app-abc123.js":    {Data: []byte("console.log(1)")},
		"assets/app-abc123.js.gz": {Data: []byte("gzipped")},
		"favicon.svg":             {Data: []byte("<svg/>")},
	}
	r := spaEngine(files)

	t.Run("client routes get index.html with the response nonce, never cached", func(t *testing.T) {
		for _, path := range []string{"/", "/messages", "/logins/3", "/signed-out"} {
			w := get(r, path, nil)
			assert.Equal(t, 200, w.Code, path)
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
			assert.NotContains(t, w.Body.String(), "__CSP_NONCE__")
			assert.Contains(t, w.Header().Get("Content-Security-Policy"), "'nonce-")
		}
	})

	t.Run("hashed assets are immutable and served gzipped when accepted", func(t *testing.T) {
		w := get(r, "/assets/app-abc123.js", map[string]string{"Accept-Encoding": "gzip, br"})
		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
		assert.Equal(t, "gzipped", w.Body.String())
		assert.Contains(t, w.Header().Get("Content-Type"), "javascript")
		assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))

		w = get(r, "/assets/app-abc123.js", nil)
		assert.Empty(t, w.Header().Get("Content-Encoding"))
		assert.Equal(t, "console.log(1)", w.Body.String())
	})

	t.Run("a missing asset is 404, not index.html", func(t *testing.T) {
		assert.Equal(t, 404, get(r, "/assets/nope.js", nil).Code)
	})

	t.Run("unknown API paths are JSON 404", func(t *testing.T) {
		w := get(r, "/api/v1/nope", nil)
		assert.Equal(t, 404, w.Code)
		assert.JSONEq(t, `{"code":"NOT_FOUND","message":"Not found."}`, w.Body.String())
	})

	t.Run("no build embedded", func(t *testing.T) {
		w := get(spaEngine(fstest.MapFS{".gitkeep": {}}), "/", nil)
		assert.Equal(t, 503, w.Code)
	})
}
