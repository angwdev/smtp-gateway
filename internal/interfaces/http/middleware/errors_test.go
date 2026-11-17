package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/pkg/response"
)

func TestTranslate(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
		field  any
	}{
		{"validation", fmt.Errorf("create: %w", shared.Invalid("username", "Use letters.")), 400, response.CodeValidation, "username"},
		{"bad json", &json.SyntaxError{}, 400, response.CodeValidation, nil},
		{"empty body", io.EOF, 400, response.CodeValidation, nil},
		{"not found", fmt.Errorf("get: %w", shared.ErrNotFound), 404, response.CodeNotFound, nil},
		{"conflict", fmt.Errorf("create: %w", shared.ErrConflict), 409, response.CodeConflict, nil},
		{"unauthorized", shared.ErrUnauthorized, 401, response.CodeUnauthorized, nil},
		{"forbidden", shared.ErrForbidden, 403, response.CodeForbidden, nil},
		{"rate limited", shared.ErrRateLimited, 429, response.CodeRateLimited, nil},
		{"anything else", errors.New("disk on fire"), 500, response.CodeInternal, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := Translate(tt.err)
			assert.Equal(t, tt.status, status)
			assert.Equal(t, tt.code, body.Code)
			assert.Equal(t, tt.field, body.Details["field"])
			assert.NotContains(t, body.Message, "disk on fire", "internal detail must not leak")
		})
	}
}

func TestErrorHandlerWritesOnce(t *testing.T) {
	var logs bytes.Buffer
	r := gin.New()
	r.Use(RequestID(), ErrorHandler(slog.New(slog.NewTextHandler(&logs, nil))))
	r.GET("/missing", func(c *gin.Context) { Fail(c, shared.ErrNotFound) })
	r.GET("/broken", func(c *gin.Context) { Fail(c, errors.New("disk on fire")) })

	w := do(r, http.MethodGet, "/missing", nil)
	assert.Equal(t, 404, w.Code)
	assert.JSONEq(t, `{"code":"NOT_FOUND","message":"Not found."}`, w.Body.String())
	assert.Empty(t, logs.String(), "expected errors are not logged as failures")

	w = do(r, http.MethodGet, "/broken", nil)
	assert.Equal(t, 500, w.Code)
	assert.Contains(t, logs.String(), "disk on fire")
	assert.NotContains(t, w.Body.String(), "disk on fire")
}
