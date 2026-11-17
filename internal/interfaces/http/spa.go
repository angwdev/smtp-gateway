package http

import (
	"bytes"
	"io/fs"
	"mime"
	nethttp "net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

const noncePlaceholder = "__CSP_NONCE__"

// SPA serves the built app: real files from dist (hashed assets cached
// forever, pre-compressed when the client accepts gzip) and index.html for
// every other GET so client-side routes survive a reload. The CSP nonce
// placeholder that Vite leaves in index.html is replaced per response.
func SPA(dist fs.FS) gin.HandlerFunc {
	index, indexErr := fs.ReadFile(dist, "index.html")

	return func(c *gin.Context) {
		urlPath := c.Request.URL.Path
		if strings.HasPrefix(urlPath, "/api/") {
			middleware.Fail(c, shared.ErrNotFound)
			return
		}
		if c.Request.Method != nethttp.MethodGet && c.Request.Method != nethttp.MethodHead {
			c.Status(nethttp.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(urlPath), "/")
		if name != "" && name != "index.html" {
			if data, err := fs.ReadFile(dist, name); err == nil {
				serveAsset(c, dist, name, data)
				return
			}
			if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
				c.Status(nethttp.StatusNotFound) // a missing file, not a client route
				return
			}
		}

		if indexErr != nil {
			c.String(nethttp.StatusServiceUnavailable, "The panel UI is not built into this binary. Run `make ui`.\n")
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Data(nethttp.StatusOK, "text/html; charset=utf-8",
			bytes.ReplaceAll(index, []byte(noncePlaceholder), []byte(middleware.Nonce(c))))
	}
}

func serveAsset(c *gin.Context, dist fs.FS, name string, data []byte) {
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.HasPrefix(name, "assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable") // Vite puts a content hash in the name
	} else {
		c.Header("Cache-Control", "public, max-age=3600")
	}
	c.Header("Vary", "Accept-Encoding")
	if strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
		if gz, err := fs.ReadFile(dist, name+".gz"); err == nil {
			c.Header("Content-Encoding", "gzip")
			data = gz
		}
	}
	c.Data(nethttp.StatusOK, contentType, data)
}
