// Package http is the HTTP interface: the JSON API under /api/v1, the OIDC
// sign-in routes, and the embedded single-page app.
package http

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	appaudit "github.com/angwdev/smtp-gateway/internal/application/audit"
	appsession "github.com/angwdev/smtp-gateway/internal/application/session"
	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/domain/audit"
	"github.com/angwdev/smtp-gateway/internal/interfaces/http/middleware"
)

const loginCookie = "sg_login"

// user is the small view of a claims set that authorize fills: just enough
// to start a server-side session.
type user struct {
	Email string
	Name  string
}

// OIDC handles the browser sign-in flow.
type OIDC struct {
	cfg      *config.Config
	client   *oidcClient
	stateKey []byte // HMAC key for the short-lived login-state cookie
	sessions *appsession.Handler
	audit    *appaudit.Recorder
	cookie   string // session cookie name
	secure   bool
	log      *slog.Logger
}

// SessionCookieName is "__Host-sg_session" over https (bound to this host,
// Secure, Path=/) and "sg_session" over plain http.
func SessionCookieName(https bool) string {
	if https {
		return "__Host-sg_session"
	}
	return "sg_session"
}

// NewOIDC builds the sign-in flow, matching internal/web.New's oidcClient
// wiring, with the session cookie name and Secure flag picked from the
// public URL's scheme.
func NewOIDC(cfg *config.Config, stateKey []byte, sessions *appsession.Handler, recorder *appaudit.Recorder, log *slog.Logger) *OIDC {
	https := cfg.PublicURL != nil && cfg.PublicURL.Scheme == "https"
	return &OIDC{
		cfg: cfg,
		client: &oidcClient{
			issuer:       cfg.OIDCIssuer,
			clientID:     cfg.OIDCClientID,
			clientSecret: cfg.OIDCClientSecret,
			redirectURL:  cfg.PublicURL.String() + "/auth/callback",
			scopes:       cfg.OIDCScopes,
		},
		stateKey: stateKey,
		sessions: sessions,
		audit:    recorder,
		cookie:   SessionCookieName(https),
		secure:   https,
		log:      log,
	}
}

// CheckProvider logs exactly what the panel will send to the identity
// provider and whether its discovery document is reachable, so a wrong
// client id, issuer or redirect URI shows up in the container log at
// start (ported from internal/web/server.go checkSSO).
func (o *OIDC) CheckProvider(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	log := o.log.With("issuer", o.cfg.OIDCIssuer, "client_id", o.cfg.OIDCClientID, "redirect_uri", o.client.redirectURL)
	p, _, err := o.client.get(ctx)
	if err != nil {
		log.Warn("sso: identity provider not reachable yet (sign-in will retry)", "err", err)
		return
	}
	log.Info("sso: identity provider found; register redirect_uri exactly as logged", "authorize_url", p.Endpoint().AuthURL)
}

// ---- signed cookies --------------------------------------------------------

func (o *OIDC) sign(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	return payload + "." + o.mac(payload), nil
}

func (o *OIDC) verify(raw string, v any) error {
	payload, sig, ok := strings.Cut(raw, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(o.mac(payload))) {
		return errors.New("bad signature")
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (o *OIDC) mac(data string) string {
	m := hmac.New(sha256.New, o.stateKey)
	m.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (o *OIDC) setCookie(c *gin.Context, name, value string, maxAge time.Duration) {
	c.SetSameSite(nethttp.SameSiteLaxMode)
	c.SetCookie(name, value, int(maxAge.Seconds()), "/", "", o.secure, true)
}

// refuse sends the browser to the SPA's signed-out page with the reason in
// the URL fragment (never sent to a server, never logged by a proxy).
func (o *OIDC) refuse(c *gin.Context, message string, err error) {
	o.log.Warn("sso sign-in rejected", "reason", message, "err", err, "request_id", middleware.RequestIDOf(c))
	c.Redirect(nethttp.StatusFound, "/signed-out#error="+url.QueryEscape(message))
}

// ---- OIDC client -----------------------------------------------------------

type oidcClient struct {
	issuer, clientID, clientSecret, redirectURL string
	scopes                                      []string

	mu       sync.Mutex
	provider *oidc.Provider
	endSess  string
}

// get lazily discovers the provider so the gateway starts (and keeps relaying
// mail) even when the identity provider is down.
func (o *oidcClient) get(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider == nil {
		p, err := oidc.NewProvider(ctx, o.issuer)
		if err != nil {
			return nil, nil, fmt.Errorf("oidc discovery: %w", err)
		}
		var extra struct {
			EndSession string `json:"end_session_endpoint"`
		}
		_ = p.Claims(&extra)
		o.provider, o.endSess = p, extra.EndSession
	}
	return o.provider, &oauth2.Config{
		ClientID:     o.clientID,
		ClientSecret: o.clientSecret,
		RedirectURL:  o.redirectURL,
		Endpoint:     o.provider.Endpoint(),
		Scopes:       o.scopes,
	}, nil
}

type loginState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Next     string `json:"r"`
	Exp      int64  `json:"x"`
}

// Login starts the authorization-code flow.
func (o *OIDC) Login(c *gin.Context) {
	_, oc, err := o.client.get(c.Request.Context())
	if err != nil {
		o.refuse(c, "The identity provider is unreachable. Try again shortly.", err)
		return
	}
	st := loginState{State: randomString(), Nonce: randomString(), Verifier: oauth2.GenerateVerifier(),
		Next: safeNext(c.Query("next")), Exp: time.Now().Add(10 * time.Minute).Unix()}
	signed, err := o.sign(st)
	if err != nil {
		o.refuse(c, "Could not start sign-in.", err)
		return
	}
	o.setCookie(c, loginCookie, signed, 10*time.Minute)
	c.Redirect(nethttp.StatusFound, oc.AuthCodeURL(st.State, oidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier)))
}

// Callback completes the authorization-code flow (ported from
// internal/web/auth.go handleCallback): it verifies state, nonce and PKCE,
// applies the allow-list and opens a server-side session.
func (o *OIDC) Callback(c *gin.Context) {
	if e := c.Query("error"); e != "" {
		o.refuse(c, "The identity provider denied the login: "+e, nil)
		return
	}
	raw, err := c.Cookie(loginCookie)
	if err != nil {
		o.refuse(c, "Login session expired. Please start again.", err)
		return
	}
	o.setCookie(c, loginCookie, "", -time.Second)
	var st loginState
	if err := o.verify(raw, &st); err != nil || time.Now().Unix() > st.Exp {
		o.refuse(c, "Login session expired. Please start again.", err)
		return
	}
	if !hmac.Equal([]byte(c.Query("state")), []byte(st.State)) {
		o.refuse(c, "Login state mismatch. Please start again.", nil)
		return
	}

	provider, oc, err := o.client.get(c.Request.Context())
	if err != nil {
		o.refuse(c, "The identity provider is unreachable.", err)
		return
	}
	tok, err := oc.Exchange(c.Request.Context(), c.Query("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		o.refuse(c, "Could not complete login with the identity provider.", err)
		return
	}
	rawIDToken, _ := tok.Extra("id_token").(string)
	idt, err := provider.Verifier(&oidc.Config{ClientID: o.cfg.OIDCClientID}).Verify(c.Request.Context(), rawIDToken)
	if err != nil {
		o.refuse(c, "The identity token was not valid.", err)
		return
	}
	if !hmac.Equal([]byte(idt.Nonce), []byte(st.Nonce)) {
		o.refuse(c, "The identity token was not valid.", errors.New("nonce mismatch"))
		return
	}

	var claims map[string]any
	if err := idt.Claims(&claims); err != nil {
		o.refuse(c, "The identity token was not valid.", err)
		return
	}
	// Some providers only put email/groups in userinfo.
	if _, ok := claims["email"]; !ok || (len(o.cfg.OIDCAllowedGroups) > 0 && claims[o.cfg.OIDCGroupsClaim] == nil) {
		if ui, err := provider.UserInfo(c.Request.Context(), oauth2.StaticTokenSource(tok)); err == nil {
			var extra map[string]any
			if ui.Claims(&extra) == nil {
				for k, v := range extra {
					if _, exists := claims[k]; !exists {
						claims[k] = v
					}
				}
			}
		}
	}

	u, reason := o.authorize(claims)
	meta := appaudit.Meta{Actor: claimedEmail(claims), ActorType: audit.ActorUser, IP: c.ClientIP(),
		UserAgent: c.Request.UserAgent(), RequestID: middleware.RequestIDOf(c)}
	ctx := appaudit.WithMeta(c.Request.Context(), meta)
	if u == nil {
		o.audit.Record(ctx, appaudit.Entry{Action: "session.login_denied", TargetType: "session", Result: audit.ResultDenied, Detail: reason})
		o.refuse(c, "Your account is not allowed to use this panel: "+reason+".", nil)
		return
	}

	sessionID, _, err := o.sessions.Create(c.Request.Context(), u.Email, u.Name, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		o.refuse(c, "Could not create a session.", err)
		return
	}
	o.setCookie(c, o.cookie, sessionID, o.sessions.TTL())
	meta.Actor = u.Email
	o.audit.Record(appaudit.WithMeta(c.Request.Context(), meta), appaudit.Entry{Action: "session.login", TargetType: "session"})
	c.Redirect(nethttp.StatusFound, st.Next)
}

// claimedEmail returns the lower-cased email claim, else preferred_username,
// else sub, else "unknown" -- used only to label a refused sign-in attempt
// before we know whether the claims are trustworthy.
func claimedEmail(claims map[string]any) string {
	str := func(k string) string { v, _ := claims[k].(string); return v }
	if e := strings.ToLower(str("email")); e != "" {
		return e
	}
	if u := str("preferred_username"); u != "" {
		return u
	}
	if s := str("sub"); s != "" {
		return s
	}
	return "unknown"
}

// authorize applies OIDC_ALLOWED_EMAILS / OIDC_ALLOWED_GROUPS. A user passes
// if they match either list; with both lists empty, every user the identity
// provider lets through is accepted. An e-mail only counts when the provider
// marks it verified (unless OIDC_ALLOW_UNVERIFIED_EMAIL is set), otherwise
// anyone able to edit their own profile address could claim a listed one.
// The returned reason says exactly why a user was refused.
func (o *OIDC) authorize(claims map[string]any) (u *user, reason string) {
	str := func(k string) string { v, _ := claims[k].(string); return v }
	email := strings.ToLower(str("email"))
	u = &user{Email: email, Name: str("name")}
	if u.Email == "" {
		u.Email = str("preferred_username")
	}
	if u.Email == "" {
		u.Email = str("sub")
	}
	if u.Name == "" {
		u.Name = str("preferred_username")
	}
	emails, groups := o.cfg.OIDCAllowedEmails, o.cfg.OIDCAllowedGroups
	if len(emails) == 0 && len(groups) == 0 {
		return u, ""
	}

	verified, hasVerified := claims["email_verified"].(bool)
	if !hasVerified {
		verified = true // provider doesn't say; trust it as before
	}
	listed := false
	if email != "" {
		at := strings.LastIndexByte(email, '@')
		for _, e := range emails {
			if e == email || (strings.HasPrefix(e, "@") && at >= 0 && e == email[at:]) {
				listed = true
				break
			}
		}
	}
	if listed && (verified || o.cfg.OIDCAllowUnverifiedEmail) {
		return u, ""
	}

	var userGroups []string
	switch g := claims[o.cfg.OIDCGroupsClaim].(type) {
	case []any:
		for _, x := range g {
			if xs, ok := x.(string); ok {
				userGroups = append(userGroups, xs)
			}
		}
	case string:
		userGroups = strings.Fields(strings.ReplaceAll(g, ",", " "))
	}
	for _, g := range groups {
		if slices.Contains(userGroups, g) {
			return u, ""
		}
	}

	o.log.Warn("sso: sign-in refused", "email", email, "email_verified", claims["email_verified"],
		"groups_claim", o.cfg.OIDCGroupsClaim, "groups", userGroups)
	switch {
	case listed:
		return nil, email + " is in OIDC_ALLOWED_EMAILS, but your identity provider reports it as unverified (email_verified=false). " +
			"Allow a group with OIDC_ALLOWED_GROUPS, have the provider send verified addresses, or set OIDC_ALLOW_UNVERIFIED_EMAIL=true if users cannot change their own e-mail"
	case email == "" && len(emails) > 0:
		return nil, "your identity provider sent no e-mail address; check that the email scope is enabled for this client"
	case len(groups) > 0 && len(userGroups) == 0:
		return nil, "signed in as " + u.Email + ", which is not an allowed e-mail, and the token has no \"" + o.cfg.OIDCGroupsClaim + "\" claim to match OIDC_ALLOWED_GROUPS"
	}
	return nil, "signed in as " + u.Email + ", which is not in the allowed e-mails or groups"
}

// Logout revokes the session and tells the SPA where to go next: the
// identity provider's end-session page when it has one.
func (o *OIDC) Logout(c *gin.Context) {
	p := middleware.PrincipalOf(c)
	if err := o.sessions.Destroy(c.Request.Context(), p.SessionID); err != nil {
		middleware.Fail(c, fmt.Errorf("sign out: %w", err))
		return
	}
	o.setCookie(c, o.cookie, "", -time.Second)
	o.audit.Record(c.Request.Context(), appaudit.Entry{Action: "session.logout", TargetType: "session"})
	middleware.MarkAudited(c)

	redirect := "/signed-out"
	o.client.mu.Lock()
	end := o.client.endSess
	o.client.mu.Unlock()
	if end != "" {
		q := url.Values{"client_id": {o.cfg.OIDCClientID}, "post_logout_redirect_uri": {o.cfg.PublicURL.String() + "/"}}
		sep := "?"
		if strings.Contains(end, "?") {
			sep = "&"
		}
		redirect = end + sep + q.Encode()
	}
	c.JSON(nethttp.StatusOK, gin.H{"redirect_url": redirect})
}

func randomString() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// safeNext only allows same-site absolute paths as post-login redirects.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, `/\`) {
		return "/"
	}
	return next
}
