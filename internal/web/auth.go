package web

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
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	sessionCookie = "sg_session"
	loginCookie   = "sg_login"
)

type user struct {
	Email string `json:"e"`
	Name  string `json:"n"`
	Exp   int64  `json:"x"`
}

func (u *user) Display() string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

// ---- signed cookies --------------------------------------------------------

func (s *Server) sign(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(b)
	return payload + "." + s.mac(payload), nil
}

func (s *Server) verify(raw string, v any) error {
	payload, sig, ok := strings.Cut(raw, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.mac(payload))) {
		return errors.New("bad signature")
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Server) mac(data string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *Server) setCookie(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.PublicURL.Scheme == "https",
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	s.setCookie(w, name, "", -time.Second)
}

func (s *Server) currentUser(r *http.Request) *user {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	var u user
	if s.verify(c.Value, &u) != nil || time.Now().Unix() > u.Exp {
		return nil
	}
	return &u
}

func (s *Server) csrfToken(u *user) string {
	return s.mac(fmt.Sprintf("csrf|%s|%d", u.Email, u.Exp))
}

// ---- OIDC ------------------------------------------------------------------

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

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	_, oc, err := s.oidc.get(r.Context())
	if err != nil {
		slog.Error("login", "err", err)
		s.renderError(w, r, http.StatusBadGateway, "The identity provider is unreachable. Try again shortly.")
		return
	}
	st := loginState{
		State:    randomString(),
		Nonce:    randomString(),
		Verifier: oauth2.GenerateVerifier(),
		Next:     safeNext(r.URL.Query().Get("next")),
		Exp:      time.Now().Add(10 * time.Minute).Unix(),
	}
	v, err := s.sign(st)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not start login.")
		return
	}
	s.setCookie(w, loginCookie, v, 10*time.Minute)
	http.Redirect(w, r, oc.AuthCodeURL(st.State, oidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier)), http.StatusFound)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, msg string, err error) {
		slog.Warn("sso callback rejected", "reason", msg, "err", err)
		s.renderError(w, r, status, msg)
	}
	if e := r.URL.Query().Get("error"); e != "" {
		fail(http.StatusForbidden, "The identity provider denied the login: "+e, nil)
		return
	}
	c, err := r.Cookie(loginCookie)
	if err != nil {
		fail(http.StatusBadRequest, "Login session expired. Please start again.", err)
		return
	}
	s.clearCookie(w, loginCookie)
	var st loginState
	if err := s.verify(c.Value, &st); err != nil || time.Now().Unix() > st.Exp {
		fail(http.StatusBadRequest, "Login session expired. Please start again.", err)
		return
	}
	if !hmac.Equal([]byte(r.URL.Query().Get("state")), []byte(st.State)) {
		fail(http.StatusBadRequest, "Login state mismatch. Please start again.", nil)
		return
	}

	provider, oc, err := s.oidc.get(r.Context())
	if err != nil {
		fail(http.StatusBadGateway, "The identity provider is unreachable.", err)
		return
	}
	tok, err := oc.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		fail(http.StatusBadGateway, "Could not complete login with the identity provider.", err)
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := provider.Verifier(&oidc.Config{ClientID: s.cfg.OIDCClientID}).Verify(r.Context(), rawID)
	if err != nil {
		fail(http.StatusForbidden, "The identity token was not valid.", err)
		return
	}
	if !hmac.Equal([]byte(idt.Nonce), []byte(st.Nonce)) {
		fail(http.StatusForbidden, "The identity token was not valid.", errors.New("nonce mismatch"))
		return
	}

	var claims map[string]any
	if err := idt.Claims(&claims); err != nil {
		fail(http.StatusForbidden, "The identity token was not valid.", err)
		return
	}
	// Some providers only put email/groups in userinfo.
	if _, ok := claims["email"]; !ok || (len(s.cfg.OIDCAllowedGroups) > 0 && claims[s.cfg.OIDCGroupsClaim] == nil) {
		if ui, err := provider.UserInfo(r.Context(), oauth2.StaticTokenSource(tok)); err == nil {
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

	u, reason := s.authorize(claims)
	if u == nil {
		fail(http.StatusForbidden, "Your account is not allowed to use this panel: "+reason+".", nil)
		return
	}
	u.Exp = time.Now().Add(s.cfg.SessionTTL).Unix()
	v, err := s.sign(u)
	if err != nil {
		fail(http.StatusInternalServerError, "Could not create session.", err)
		return
	}
	s.setCookie(w, sessionCookie, v, s.cfg.SessionTTL)
	_ = s.store.Audit(r.Context(), u.Email, "login", "")
	slog.Info("panel login", "user", u.Email)
	http.Redirect(w, r, st.Next, http.StatusFound)
}

// authorize applies OIDC_ALLOWED_EMAILS / OIDC_ALLOWED_GROUPS. A user passes
// if they match either list; with both lists empty, every user the identity
// provider lets through is accepted. An e-mail only counts when the provider
// marks it verified (unless OIDC_ALLOW_UNVERIFIED_EMAIL is set), otherwise
// anyone able to edit their own profile address could claim a listed one.
// The returned reason says exactly why a user was refused.
func (s *Server) authorize(claims map[string]any) (*user, string) {
	str := func(k string) string { v, _ := claims[k].(string); return v }
	email := strings.ToLower(str("email"))
	u := &user{Email: email, Name: str("name")}
	if u.Email == "" {
		u.Email = str("preferred_username")
	}
	if u.Email == "" {
		u.Email = str("sub")
	}
	if u.Name == "" {
		u.Name = str("preferred_username")
	}
	emails, groups := s.cfg.OIDCAllowedEmails, s.cfg.OIDCAllowedGroups
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
	if listed && (verified || s.cfg.OIDCAllowUnverifiedEmail) {
		return u, ""
	}

	var userGroups []string
	switch g := claims[s.cfg.OIDCGroupsClaim].(type) {
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

	slog.Warn("sso: sign-in refused", "email", email, "email_verified", claims["email_verified"],
		"groups_claim", s.cfg.OIDCGroupsClaim, "groups", userGroups)
	switch {
	case listed:
		return nil, email + " is in OIDC_ALLOWED_EMAILS, but your identity provider reports it as unverified (email_verified=false). " +
			"Allow a group with OIDC_ALLOWED_GROUPS, have the provider send verified addresses, or set OIDC_ALLOW_UNVERIFIED_EMAIL=true if users cannot change their own e-mail"
	case email == "" && len(emails) > 0:
		return nil, "your identity provider sent no e-mail address; check that the email scope is enabled for this client"
	case len(groups) > 0 && len(userGroups) == 0:
		return nil, "signed in as " + u.Email + ", which is not an allowed e-mail, and the token has no \"" + s.cfg.OIDCGroupsClaim + "\" claim to match OIDC_ALLOWED_GROUPS"
	}
	return nil, "signed in as " + u.Email + ", which is not in the allowed e-mails or groups"
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if u := s.currentUser(r); u != nil {
		_ = s.store.Audit(r.Context(), u.Email, "logout", "")
	}
	s.clearCookie(w, sessionCookie)
	s.oidc.mu.Lock()
	end := s.oidc.endSess
	s.oidc.mu.Unlock()
	if end != "" {
		q := url.Values{"client_id": {s.cfg.OIDCClientID}, "post_logout_redirect_uri": {s.cfg.PublicURL.String() + "/"}}
		sep := "?"
		if strings.Contains(end, "?") {
			sep = "&"
		}
		http.Redirect(w, r, end+sep+q.Encode(), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/signed-out", http.StatusFound)
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
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}
