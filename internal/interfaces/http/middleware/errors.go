package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/angwdev/smtp-gateway/internal/domain/shared"
	"github.com/angwdev/smtp-gateway/pkg/response"
)

// Fail reports err for the ErrorHandler to answer and stops the chain.
// Handlers call it and return.
func Fail(c *gin.Context, err error) {
	_ = c.Error(err) // c.Error returns the same error wrapped for chaining; nothing to handle
	c.Abort()
}

// ErrorHandler answers every failed request with the unified error body.
// Unexpected errors are logged with the request id; the client only sees a
// generic message.
func ErrorHandler(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		err := c.Errors.Last().Err
		status, body := Translate(err)
		if status == http.StatusInternalServerError {
			log.Error("request failed", "err", err, "request_id", RequestIDOf(c), "path", c.Request.URL.Path)
		}
		if status == http.StatusTooManyRequests {
			retryAfter := 1 // conservative default when the limiter's window is not known here
			var rateLimited *shared.RateLimitedError
			if errors.As(err, &rateLimited) && rateLimited.RetrySeconds > 0 {
				retryAfter = rateLimited.RetrySeconds
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		c.JSON(status, body)
	}
}

// Translate maps an error to a status and body.
func Translate(err error) (int, response.ErrorResponse) {
	var (
		invalid  *shared.ValidationError
		binding  validator.ValidationErrors
		syntax   *json.SyntaxError
		badType  *json.UnmarshalTypeError
		tooLarge *http.MaxBytesError
	)
	switch {
	case errors.As(err, &invalid):
		return http.StatusBadRequest, response.ErrorResponse{Code: response.CodeValidation, Message: invalid.Message,
			Details: map[string]any{"field": invalid.Field}}
	case errors.As(err, &binding) && len(binding) > 0:
		field := strings.ToLower(binding[0].Field())
		return http.StatusBadRequest, response.ErrorResponse{Code: response.CodeValidation,
			Message: "Check the value of " + field + ".", Details: map[string]any{"field": field}}
	case errors.As(err, &tooLarge):
		return http.StatusBadRequest, response.ErrorResponse{Code: response.CodeValidation, Message: "The request body is too large."}
	case errors.As(err, &syntax), errors.As(err, &badType), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return http.StatusBadRequest, response.ErrorResponse{Code: response.CodeValidation, Message: "The request body is not valid JSON."}
	case errors.Is(err, shared.ErrNotFound):
		return http.StatusNotFound, response.ErrorResponse{Code: response.CodeNotFound, Message: "Not found."}
	case errors.Is(err, shared.ErrConflict):
		return http.StatusConflict, response.ErrorResponse{Code: response.CodeConflict, Message: "That name is already taken."}
	case errors.Is(err, shared.ErrUnauthorized), errors.Is(err, shared.ErrInvalidLogin):
		return http.StatusUnauthorized, response.ErrorResponse{Code: response.CodeUnauthorized, Message: "Sign in, or send a valid API token."}
	case errors.Is(err, shared.ErrForbidden):
		return http.StatusForbidden, response.ErrorResponse{Code: response.CodeForbidden, Message: "You are not allowed to do that."}
	case errors.Is(err, shared.ErrRateLimited):
		return http.StatusTooManyRequests, response.ErrorResponse{Code: response.CodeRateLimited, Message: "Too many requests. Try again shortly."}
	}
	return http.StatusInternalServerError, response.ErrorResponse{Code: response.CodeInternal, Message: "An internal error occurred."}
}
