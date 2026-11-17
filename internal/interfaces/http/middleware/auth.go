package middleware

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/apitoken"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/domain/session"
	"github.com/angwdev/smtp-gateway/internal/domain/shared"
)

// Principal kinds.
const (
	KindUser  = "user"
	KindToken = "token"
)

// Principal is who is making the request.
type Principal struct {
	Kind      string
	Name      string // display name of the user, or the token's name
	Email     string // users only
	Scope     apitoken.Scope
	SessionID string    // raw cookie value, users only (needed to sign out)
	ExpiresAt time.Time // users only
}

// The middleware depends on four behaviours, not on packages.
type (
	// SessionResolver looks a session cookie up.
	SessionResolver interface {
		Resolve(ctx context.Context, rawID string) (*session.Session, error)
	}
	// TokenAuthenticator checks a bearer token.
	TokenAuthenticator interface {
		Authenticate(ctx context.Context, raw, ip string) (*apitoken.Token, error)
	}
	// NoisyAuditor records events an outsider can trigger at will.
	NoisyAuditor interface {
		RecordNoisy(ctx context.Context, e appaudit.Entry)
	}
	// FailCounter is the per-address brute-force limiter.
	FailCounter interface {
		Blocked(ip string) bool
		Fail(ip string) bool
		Reset(ip string)
	}
)

// PrincipalOf returns the authenticated principal, or nil.
func PrincipalOf(c *gin.Context) *Principal {
	p, _ := c.Get(keyPrincipal)
	principal, _ := p.(*Principal)
	return principal
}

func bearer(header string) (string, bool) {
	const scheme = "Bearer "
	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	return strings.TrimSpace(header[len(scheme):]), true
}

// Authenticate accepts an API token in the Authorization header or the
// panel's session cookie. When the header is present the cookie is not
// consulted, so a bad token can never ride on a browser session.
func Authenticate(sessions SessionResolver, tokens TokenAuthenticator, auditor NoisyAuditor, fails FailCounter, cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, ip := c.Request.Context(), c.ClientIP()
		meta := appaudit.Meta{IP: ip, UserAgent: c.Request.UserAgent(), RequestID: RequestIDOf(c)}
		var principal *Principal

		if raw, ok := bearer(c.GetHeader("Authorization")); ok {
			if fails.Blocked(ip) {
				Fail(c, shared.ErrRateLimited)
				return
			}
			token, err := tokens.Authenticate(ctx, raw, ip)
			if err != nil {
				if errors.Is(err, shared.ErrUnauthorized) {
					fails.Fail(ip)
					meta.Actor, meta.ActorType = "unknown token", audit.ActorToken
					auditor.RecordNoisy(appaudit.WithMeta(ctx, meta), appaudit.Entry{
						Action: "token.auth_failed", TargetType: "token", Result: audit.ResultDenied})
				}
				Fail(c, err)
				return
			}
			fails.Reset(ip)
			principal = &Principal{Kind: KindToken, Name: token.Name, Scope: token.Scope}
			meta.Actor, meta.ActorType = token.Name, audit.ActorToken
		} else {
			cookie, err := c.Cookie(cookieName)
			if err != nil || cookie == "" {
				Fail(c, shared.ErrUnauthorized)
				return
			}
			s, err := sessions.Resolve(ctx, cookie)
			if err != nil {
				Fail(c, err)
				return
			}
			principal = &Principal{Kind: KindUser, Name: s.Display(), Email: s.Email, Scope: apitoken.ScopeWrite,
				SessionID: cookie, ExpiresAt: s.ExpiresAt}
			meta.Actor, meta.ActorType = s.Email, audit.ActorUser
		}

		c.Set(keyPrincipal, principal)
		c.Request = c.Request.WithContext(appaudit.WithMeta(ctx, meta))
		c.Next()
	}
}

// RequireScope refuses principals whose scope does not cover need.
func RequireScope(need apitoken.Scope) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p := PrincipalOf(c); p == nil || !p.Scope.Allows(need) {
			Fail(c, shared.ErrForbidden)
			return
		}
		c.Next()
	}
}

// RequireSession refuses API tokens: managing tokens and signing out are
// for a signed-in person only, so a leaked token cannot mint more tokens.
func RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p := PrincipalOf(c); p == nil || p.Kind != KindUser {
			Fail(c, shared.ErrForbidden)
			return
		}
		c.Next()
	}
}
