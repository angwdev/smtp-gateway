// Package web is the SSO-protected control panel for managing gateway
// credentials. It is plain server-rendered HTML with no client framework.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/proxy"
	"github.com/angwdev/smtp-gateway/internal/store"
)

//go:embed templates/*.html static/*
var assets embed.FS

type Server struct {
	cfg      *config.Config
	store    *store.Store
	upstream *proxy.Upstream
	oidc     *oidcClient
	key      []byte
	pages    map[string]*template.Template
	handler  http.Handler
}

func New(cfg *config.Config, st *store.Store, up *proxy.Upstream, sessionKey []byte) (*Server, error) {
	s := &Server{
		cfg:      cfg,
		store:    st,
		upstream: up,
		key:      sessionKey,
		oidc: &oidcClient{
			issuer:       cfg.OIDCIssuer,
			clientID:     cfg.OIDCClientID,
			clientSecret: cfg.OIDCClientSecret,
			redirectURL:  cfg.PublicURL.String() + "/auth/callback",
			scopes:       cfg.OIDCScopes,
		},
	}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}

	static, _ := fs.Sub(assets, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheForever(http.FileServerFS(static))))
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("GET /signed-out", s.handleSignedOut)
	mux.HandleFunc("POST /logout", s.protect(func(w http.ResponseWriter, r *http.Request, _ *user) { s.handleLogout(w, r) }))

	mux.HandleFunc("GET /{$}", s.protect(s.handleDashboard))
	mux.HandleFunc("POST /upstream/test", s.protect(s.handleUpstreamTest))
	mux.HandleFunc("GET /credentials/new", s.protect(s.handleNewCredential))
	mux.HandleFunc("POST /credentials", s.protect(s.handleCreateCredential))
	mux.HandleFunc("GET /credentials/{id}", s.protect(s.handleEditCredential))
	mux.HandleFunc("POST /credentials/{id}", s.protect(s.handleUpdateCredential))
	mux.HandleFunc("POST /credentials/{id}/rotate", s.protect(s.handleRotate))
	mux.HandleFunc("POST /credentials/{id}/toggle", s.protect(s.handleToggle))
	mux.HandleFunc("POST /credentials/{id}/delete", s.protect(s.handleDelete))
	mux.HandleFunc("GET /logs", s.protect(s.handleLogs))
	mux.HandleFunc("GET /audit", s.protect(s.handleAudit))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.renderError(w, r, http.StatusNotFound, "Page not found.")
	})
	// Logout POSTs redirect to the IdP's end-session endpoint, which
	// form-action must allow.
	formAction := "'self'"
	if iu, err := url.Parse(cfg.OIDCIssuer); err == nil && iu.Host != "" {
		formAction += " " + iu.Scheme + "://" + iu.Host
	}
	s.handler = securityHeaders(mux, formAction)
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.WebAddr,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    32 << 10,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	slog.Info("panel listening", "addr", s.cfg.WebAddr, "public_url", s.cfg.PublicURL.String())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// protect requires a valid session; POSTs additionally need a matching CSRF
// token and same-origin Origin header.
func (s *Server) protect(h func(http.ResponseWriter, *http.Request, *user)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.currentUser(r)
		if u == nil {
			if r.Method == http.MethodGet {
				http.Redirect(w, r, "/login?next="+urlQueryEscape(r.URL.RequestURI()), http.StatusFound)
				return
			}
			s.renderError(w, r, http.StatusUnauthorized, "Your session has expired. Please sign in again.")
			return
		}
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
			if o := r.Header.Get("Origin"); o != "" && o != "null" && !strings.EqualFold(o, s.origin()) {
				s.renderError(w, r, http.StatusForbidden, "Cross-origin request refused.")
				return
			}
			if r.PostFormValue("csrf") != s.csrfToken(u) {
				s.renderError(w, r, http.StatusForbidden, "Form expired. Go back, reload and try again.")
				return
			}
		}
		h(w, r, u)
	}
}

func (s *Server) origin() string { return s.cfg.PublicURL.Scheme + "://" + s.cfg.PublicURL.Host }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		http.Error(w, "db: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok\n"))
}

// ---- rendering -------------------------------------------------------------

type page struct {
	Title  string
	User   *user
	CSRF   string
	Nav    string
	Flash  string
	Error  string
	Cfg    *config.Config
	Data   any
	Status int
}

var funcs = template.FuncMap{
	"ts": func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
	"tsp": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.UTC().Format("2006-01-02 15:04 UTC")
	},
	"ago": func(t any) string {
		var tt time.Time
		switch v := t.(type) {
		case time.Time:
			tt = v
		case *time.Time:
			if v == nil {
				return "never"
			}
			tt = *v
		}
		d := time.Since(tt)
		switch {
		case d < time.Minute:
			return "just now"
		case d < time.Hour:
			return fmt.Sprintf("%dm ago", int(d.Minutes()))
		case d < 48*time.Hour:
			return fmt.Sprintf("%dh ago", int(d.Hours()))
		default:
			return fmt.Sprintf("%dd ago", int(d.Hours()/24))
		}
	},
	"join":       strings.Join,
	"list":       func(v ...string) []string { return v },
	"deref_bool": func(b *bool) bool { return b != nil && *b },
	"bytes": func(n int64) string {
		switch {
		case n >= 1<<20:
			return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
		case n >= 1<<10:
			return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
		}
		return fmt.Sprintf("%d B", n)
	},
	"deref": func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	},
}

func (s *Server) parseTemplates() error {
	s.pages = map[string]*template.Template{}
	names, err := fs.Glob(assets, "templates/*.html")
	if err != nil {
		return err
	}
	for _, n := range names {
		base := strings.TrimSuffix(strings.TrimPrefix(n, "templates/"), ".html")
		if base == "layout" {
			continue
		}
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(assets, "templates/layout.html", n)
		if err != nil {
			return fmt.Errorf("template %s: %w", n, err)
		}
		s.pages[base] = t
	}
	return nil
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, p page) {
	p.Cfg = s.cfg
	if p.User == nil {
		p.User = s.currentUser(r)
	}
	if p.User != nil {
		p.CSRF = s.csrfToken(p.User)
	}
	t := s.pages[name]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if p.Status != 0 {
		w.WriteHeader(p.Status)
	}
	if err := t.Execute(w, p); err != nil {
		slog.Error("render", "page", name, "err", err)
	}
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	s.render(w, r, "error", page{Title: http.StatusText(status), Error: msg, Status: status})
}

func (s *Server) handleSignedOut(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "signed_out", page{Title: "Signed out"})
}

func securityHeaders(h http.Handler, formAction string) http.Handler {
	csp := "default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self' data:; form-action " + formAction + "; frame-ancestors 'none'; base-uri 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", csp)
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "same-origin")
		hd.Set("X-Frame-Options", "DENY")
		h.ServeHTTP(w, r)
	})
}

func cacheForever(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		h.ServeHTTP(w, r)
	})
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}
