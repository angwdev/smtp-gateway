package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/angwdev/smtp-gateway/internal/acme"
	"github.com/angwdev/smtp-gateway/internal/store"
	"github.com/angwdev/smtp-gateway/internal/tlsutil"
)

func urlQueryEscape(s string) string { return url.QueryEscape(s) }

type dashboardData struct {
	Credentials []*store.Credential
	Active      int
	Counts      map[string]int
	Upstream    upstreamView
	Cert        certView
	ProbeOK     *bool
	ProbeMsg    string
}

type certView struct {
	tlsutil.Info
	Label    string // where it comes from, in words
	DaysLeft int
	Level    string // ok | warn | bad
	ACME     *acme.Status
}

var caNames = map[string]string{
	"https://acme-v02.api.letsencrypt.org/directory":         "Let's Encrypt",
	"https://acme-staging-v02.api.letsencrypt.org/directory": "Let's Encrypt staging",
	"https://acme.zerossl.com/v2/DV90":                       "ZeroSSL",
	"https://dv.acme-v02.api.pki.goog/directory":             "Google Trust Services",
}

var dnsNames = map[string]string{"cloudflare": "Cloudflare", "route53": "Route 53"}

func (s *Server) certView() certView {
	v := certView{Info: s.certs.Info()}
	v.DaysLeft = int(v.Remaining().Hours() / 24)
	switch {
	case v.Source == tlsutil.SourceSelfSigned:
		v.Label, v.Level = "Self-signed", "warn"
	case v.DaysLeft < 3:
		v.Level = "bad"
	case v.DaysLeft < 14:
		v.Level = "warn"
	default:
		v.Level = "ok"
	}
	if v.Source == tlsutil.SourceFile {
		v.Label = "Certificate file"
	}
	if s.acme != nil {
		st := s.acme.Status()
		v.ACME = &st
		ca := caNames[st.CA]
		if ca == "" {
			if u, err := url.Parse(st.CA); err == nil {
				ca = u.Host
			}
		}
		v.Label = ca + " via " + dnsNames[st.Provider] + " DNS"
		if st.LastError != "" && v.Level == "ok" {
			v.Level = "warn"
		}
	}
	return v
}

type upstreamView struct {
	Addr, TLS, Username, EnvelopeFrom string
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, u *user, p page) {
	creds, err := s.store.ListCredentials(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not load credentials.")
		return
	}
	counts, _ := s.store.StatusCounts(r.Context(), time.Now().Add(-24*time.Hour))
	uc := s.upstream.Config()
	d, _ := p.Data.(*dashboardData)
	if d == nil {
		d = &dashboardData{}
	}
	d.Credentials, d.Counts, d.Cert = creds, counts, s.certView()
	for _, c := range creds {
		if c.Enabled {
			d.Active++
		}
	}
	d.Upstream = upstreamView{Addr: uc.Addr(), TLS: uc.TLSMode, Username: uc.Username, EnvelopeFrom: uc.EnvelopeFrom}
	p.Title, p.Nav, p.User, p.Data = "Credentials", "credentials", u, d
	s.render(w, r, "dashboard", p)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, u *user) {
	s.dashboard(w, r, u, page{Flash: flashFromQuery(r)})
}

func (s *Server) handleUpstreamTest(w http.ResponseWriter, r *http.Request, u *user) {
	d, err := s.upstream.Probe(r.Context())
	ok := err == nil
	data := &dashboardData{ProbeOK: &ok}
	if ok {
		data.ProbeMsg = fmt.Sprintf("Connected and authenticated to %s in %s.", s.upstream.Config().Addr(), d.Round(time.Millisecond))
	} else {
		data.ProbeMsg = err.Error()
	}
	s.dashboard(w, r, u, page{Data: data})
}

func (s *Server) handleRenewCert(w http.ResponseWriter, r *http.Request, u *user) {
	if s.acme == nil {
		s.renderError(w, r, http.StatusNotFound, "Automatic certificates are not enabled (TLS_SOURCE=acme).")
		return
	}
	s.acme.RenewNow()
	_ = s.store.Audit(r.Context(), u.Email, "tls.renew", strings.Join(s.cfg.ACME.Domains, ","))
	http.Redirect(w, r, "/?flash=renewing", http.StatusSeeOther)
}

// ---- credentials -----------------------------------------------------------

type credForm struct {
	Cred    *store.Credential
	IsNew   bool
	Senders string
	Recent  []*store.Message
	Errors  map[string]string // field name -> message
}

// fieldError ties a validation message to the form field it is about.
type fieldError struct{ field, msg string }

func (e fieldError) Error() string { return e.msg }

func formErrors(err error) map[string]string {
	var fe fieldError
	if errors.As(err, &fe) {
		return map[string]string{fe.field: fe.msg}
	}
	if errors.Is(err, store.ErrDuplicate) {
		return map[string]string{"username": "That username is already taken."}
	}
	if errors.Is(err, store.ErrBadUsername) {
		return map[string]string{"username": "Use 1–64 letters, digits or . _ - + @"}
	}
	return map[string]string{"": "Could not save. " + err.Error()}
}

type secretData struct {
	Cred    *store.Credential
	Secret  string
	Rotated bool
}

func (s *Server) handleNewCredential(w http.ResponseWriter, r *http.Request, u *user) {
	s.render(w, r, "credential", page{Title: "New credential", Nav: "credentials", User: u,
		Data: &credForm{Cred: &store.Credential{Enabled: true}, IsNew: true}})
}

// parseCredForm reads every field into c before validating any of them, so a
// form re-rendered after an error keeps everything the user entered.
func parseCredForm(r *http.Request, c *store.Credential) (string, error) {
	c.Description = strings.TrimSpace(r.PostFormValue("description"))
	c.Enabled = r.PostFormValue("enabled") == "on"
	raw := r.PostFormValue("allowed_senders")
	c.AllowedSenders = nil
	var badSender string
	for _, l := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == ',' || r == ' ' || r == '\r' }) {
		if l != "*" && !strings.Contains(l, "@") && badSender == "" {
			badSender = l
		}
		c.AllowedSenders = append(c.AllowedSenders, strings.ToLower(l))
	}
	rate := strings.TrimSpace(r.PostFormValue("rate_per_hour"))
	n, rateErr := 0, error(nil)
	if rate != "" {
		n, rateErr = strconv.Atoi(rate)
	}
	if rateErr == nil && n >= 0 && n <= 1_000_000 {
		c.RatePerHour = n
	}

	switch {
	case len(c.Description) > 200:
		return raw, fieldError{"description", "Keep it under 200 characters."}
	case badSender != "":
		return raw, fieldError{"allowed_senders", fmt.Sprintf("%q is not a sender pattern. Use user@domain, @domain, @*.domain or *.", badSender)}
	case rateErr != nil || n < 0 || n > 1_000_000:
		return raw, fieldError{"rate_per_hour", "Enter a whole number, or 0 for no limit."}
	}
	return raw, nil
}

func (s *Server) handleCreateCredential(w http.ResponseWriter, r *http.Request, u *user) {
	c := &store.Credential{Username: strings.TrimSpace(r.PostFormValue("username")), CreatedBy: u.Email}
	raw, err := parseCredForm(r, c)
	var secret string
	if err == nil {
		secret, err = s.store.CreateCredential(r.Context(), c)
	}
	if err != nil {
		s.render(w, r, "credential", page{Title: "New credential", Nav: "credentials", User: u,
			Status: http.StatusUnprocessableEntity, Data: &credForm{Cred: c, IsNew: true, Senders: raw, Errors: formErrors(err)}})
		return
	}
	_ = s.store.Audit(r.Context(), u.Email, "credential.create", c.Username)
	s.render(w, r, "secret", page{Title: "Credential created", Nav: "credentials", User: u,
		Data: &secretData{Cred: c, Secret: secret}})
}

func (s *Server) loadCred(w http.ResponseWriter, r *http.Request) *store.Credential {
	id, ok := pathID(r)
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Credential not found.")
		return nil
	}
	c, err := s.store.GetCredential(r.Context(), id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Credential not found.")
		return nil
	}
	return c
}

func (s *Server) handleEditCredential(w http.ResponseWriter, r *http.Request, u *user) {
	c := s.loadCred(w, r)
	if c == nil {
		return
	}
	recent, _ := s.store.ListMessages(r.Context(), store.MessageFilter{CredentialID: c.ID, Limit: 15})
	s.render(w, r, "credential", page{Title: c.Username, Nav: "credentials", User: u, Flash: flashFromQuery(r),
		Data: &credForm{Cred: c, Senders: strings.Join(c.AllowedSenders, "\n"), Recent: recent}})
}

func (s *Server) handleUpdateCredential(w http.ResponseWriter, r *http.Request, u *user) {
	c := s.loadCred(w, r)
	if c == nil {
		return
	}
	raw, err := parseCredForm(r, c)
	if err == nil {
		err = s.store.UpdateCredential(r.Context(), c)
	}
	if err != nil {
		s.render(w, r, "credential", page{Title: c.Username, Nav: "credentials", User: u,
			Status: http.StatusUnprocessableEntity, Data: &credForm{Cred: c, Senders: raw, Errors: formErrors(err)}})
		return
	}
	_ = s.store.Audit(r.Context(), u.Email, "credential.update", fmt.Sprintf("%s senders=%q rate=%d enabled=%t",
		c.Username, strings.Join(c.AllowedSenders, ","), c.RatePerHour, c.Enabled))
	http.Redirect(w, r, fmt.Sprintf("/credentials/%d?flash=saved", c.ID), http.StatusSeeOther)
}

func (s *Server) handleRotate(w http.ResponseWriter, r *http.Request, u *user) {
	c := s.loadCred(w, r)
	if c == nil {
		return
	}
	secret, err := s.store.RotateSecret(r.Context(), c.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not rotate the password.")
		return
	}
	_ = s.store.Audit(r.Context(), u.Email, "credential.rotate", c.Username)
	s.render(w, r, "secret", page{Title: "Password rotated", Nav: "credentials", User: u,
		Data: &secretData{Cred: c, Secret: secret, Rotated: true}})
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request, u *user) {
	c := s.loadCred(w, r)
	if c == nil {
		return
	}
	if err := s.store.SetEnabled(r.Context(), c.ID, !c.Enabled); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not update the credential.")
		return
	}
	action := "credential.disable"
	if !c.Enabled {
		action = "credential.enable"
	}
	_ = s.store.Audit(r.Context(), u.Email, action, c.Username)
	http.Redirect(w, r, "/?flash="+strings.TrimPrefix(action, "credential."), http.StatusSeeOther)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, u *user) {
	c := s.loadCred(w, r)
	if c == nil {
		return
	}
	if r.PostFormValue("confirm") != c.Username {
		http.Redirect(w, r, fmt.Sprintf("/credentials/%d?flash=confirm", c.ID), http.StatusSeeOther)
		return
	}
	if err := s.store.DeleteCredential(r.Context(), c.ID); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not delete the credential.")
		return
	}
	_ = s.store.Audit(r.Context(), u.Email, "credential.delete", c.Username)
	http.Redirect(w, r, "/?flash=deleted", http.StatusSeeOther)
}

var flashes = map[string]string{
	"saved":    "Changes saved.",
	"deleted":  "Credential deleted.",
	"enable":   "Credential enabled.",
	"disable":  "Credential disabled. Open connections are refused at their next message.",
	"confirm":  "Type the username exactly to confirm deletion.",
	"renewing": "Certificate renewal started. It usually takes under a minute; refresh to see the result.",
}

func flashFromQuery(r *http.Request) string { return flashes[r.URL.Query().Get("flash")] }

// ---- logs ------------------------------------------------------------------

type logsData struct {
	Messages    []*store.Message
	Credentials []*store.Credential
	Filter      store.MessageFilter
	NextBefore  int64
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request, u *user) {
	q := r.URL.Query()
	f := store.MessageFilter{Status: q.Get("status"), Query: strings.TrimSpace(q.Get("q")), Limit: 100}
	f.CredentialID, _ = strconv.ParseInt(q.Get("cred"), 10, 64)
	f.BeforeID, _ = strconv.ParseInt(q.Get("before"), 10, 64)
	msgs, err := s.store.ListMessages(r.Context(), f)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not load the message log.")
		return
	}
	creds, _ := s.store.ListCredentials(r.Context())
	d := &logsData{Messages: msgs, Credentials: creds, Filter: f}
	if len(msgs) == f.Limit {
		d.NextBefore = msgs[len(msgs)-1].ID
	}
	s.render(w, r, "logs", page{Title: "Message log", Nav: "logs", User: u, Data: d})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request, u *user) {
	entries, err := s.store.ListAudit(r.Context(), 200)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not load the audit log.")
		return
	}
	s.render(w, r, "audit", page{Title: "Audit log", Nav: "audit", User: u, Data: entries})
}
