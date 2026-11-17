package http

import (
	"io/fs"
	"log/slog"
	nethttp "net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	apptoken "github.com/angwdev/smtp-gateway/internal/application/apitoken"
	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	appsession "github.com/angwdev/smtp-gateway/internal/application/session"
	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/handler"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
	"github.com/angwdev/smtp-gateway/internal/interfaces/limiter"
	"github.com/angwdev/smtp-gateway/pkg/response"
)

// Deps is everything the router needs. internal/app fills it.
type Deps struct {
	Config *config.Config
	Log    *slog.Logger
	UI     fs.FS

	OIDC        *OIDC
	Sessions    *appsession.Handler
	Tokens      *apptoken.Handler
	Recorder    *appaudit.Recorder
	Fails       *limiter.FailLimiter
	APILimiter  *limiter.PerIP
	AuthLimiter *limiter.PerIP

	Credentials *handler.CredentialHandler
	Messages    *handler.MessageHandler
	Audit       *handler.AuditHandler
	TokenAPI    *handler.TokenHandler
	Session     *handler.SessionHandler
	Overview    *handler.OverviewHandler
	System      *handler.SystemHandler
}

const maxBodyBytes = 64 << 10

// NewRouter builds the Gin engine (skill rules framework-gin-simple and
// framework-middleware: cross-cutting concerns live in middleware, handlers
// stay thin).
//
//nolint:gocritic // hugeParam: Deps is built once at startup and read for clarity, not hot-path performance
func NewRouter(d Deps) (*gin.Engine, error) {
	gin.SetMode(gin.ReleaseMode)
	// Report validation errors by their JSON field name.
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			return name
		})
	}

	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.ForwardedByClientIP = true
	r.RemoteIPHeaders = []string{"X-Forwarded-For"}
	// nil trusts no proxy: c.ClientIP() is then the socket address.
	var proxies []string
	if len(d.Config.TrustedProxies) > 0 {
		proxies = d.Config.TrustedProxies
	}
	if err := r.SetTrustedProxies(proxies); err != nil {
		return nil, err
	}

	https := d.Config.PublicURL != nil && d.Config.PublicURL.Scheme == "https"
	r.Use(middleware.RequestID(), middleware.Logger(d.Log), middleware.Recovery(d.Log), middleware.SecurityHeaders(https))

	r.GET("/healthz", d.System.Health)
	// Login limiter is 10 requests/minute per IP; tell the client the
	// matching worst-case wait instead of the generic default.
	const loginRetryAfterSeconds = 60
	auth := r.Group("", middleware.ErrorHandler(d.Log), middleware.RateLimit(d.AuthLimiter, loginRetryAfterSeconds))
	auth.GET("/login", d.OIDC.Login)
	auth.GET("/auth/callback", d.OIDC.Callback)

	api := r.Group("/api/v1",
		middleware.ErrorHandler(d.Log),
		func(c *gin.Context) {
			c.Header("Cache-Control", "no-store")
			c.Request.Body = nethttp.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes) // API payloads are tiny
			c.Next()
		},
		middleware.CrossOrigin(),
		// 20 requests/s, burst 60: a 1-second retry (the ErrorHandler
		// default) already matches this limiter's refill rate closely
		// enough that it is not worth a precise per-bucket calculation.
		middleware.RateLimit(d.APILimiter, 1),
	)
	api.GET("/openapi.yaml", d.System.OpenAPI)

	authed := api.Group("",
		middleware.Authenticate(d.Sessions, d.Tokens, d.Recorder, d.Fails, SessionCookieName(https)),
		middleware.AuditCalls(d.Recorder),
	)
	read := authed.Group("", middleware.RequireScope(apitoken.ScopeRead))
	write := authed.Group("", middleware.RequireScope(apitoken.ScopeWrite))
	person := authed.Group("", middleware.RequireSession())

	read.GET("/session", d.Session.Get)
	read.GET("/overview", d.Overview.Get)
	read.GET("/credentials", d.Credentials.List)
	read.GET("/credentials/:id", d.Credentials.Get)
	read.GET("/messages", d.Messages.List)
	read.GET("/messages/:id", d.Messages.Get)
	read.GET("/audit", d.Audit.List)

	write.POST("/upstream/test", d.System.TestUpstream)
	write.POST("/certificate/renew", d.System.RenewCertificate)
	write.POST("/credentials", d.Credentials.Create)
	write.PATCH("/credentials/:id", d.Credentials.Patch)
	write.POST("/credentials/:id/rotate", d.Credentials.Rotate)
	write.DELETE("/credentials/:id", d.Credentials.Delete)

	person.DELETE("/session", d.OIDC.Logout)
	person.GET("/tokens", d.TokenAPI.List)
	person.POST("/tokens", d.TokenAPI.Create)
	person.DELETE("/tokens/:id", d.TokenAPI.Revoke)

	r.NoMethod(middleware.ErrorHandler(d.Log), func(c *gin.Context) {
		c.JSON(nethttp.StatusMethodNotAllowed, response.ErrorResponse{Code: response.CodeMethodNotAllowed, Message: "Method not allowed."})
	})
	r.NoRoute(middleware.ErrorHandler(d.Log), SPA(d.UI))
	return r, nil
}
