// Package config loads gateway settings from environment variables.
//
// Every secret-bearing variable FOO also accepts FOO_FILE, pointing at a file
// whose contents are used instead (Docker secrets friendly).
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DataDir string

	// SMTP side (what clients connect to).
	Hostname          string
	SubmissionAddr    string // STARTTLS listener, empty disables
	SMTPSAddr         string // implicit TLS listener, empty disables
	TLSSource         string // file | acme | self-signed
	TLSCertFile       string
	TLSKeyFile        string
	ACME              ACME
	AllowInsecureAuth bool
	MaxMessageBytes   int64
	MaxRecipients     int
	MaxLineLength     int
	AuthFailLimit     int
	AuthFailWindow    time.Duration

	// Shown in the panel next to new credentials (the host-side ports).
	PublicSubmissionPort string
	PublicSMTPSPort      string

	// Upstream (the real mail server, one credential).
	Upstream Upstream

	// Web panel.
	WebAddr           string
	PublicURL         *url.URL
	SessionSecret     string
	SessionTTL        time.Duration
	OIDCIssuer        string
	OIDCClientID      string
	OIDCClientSecret  string
	OIDCScopes        []string
	OIDCAllowedEmails []string
	OIDCAllowedGroups []string
	// Accept listed e-mails even when the IdP sends email_verified=false.
	OIDCAllowUnverifiedEmail bool
	OIDCGroupsClaim          string

	LogRetention time.Duration
	LogLevel     string
}

type Upstream struct {
	Host          string
	Port          int
	TLSMode       string // starttls | tls | none
	TLSSkipVerify bool
	Username      string
	Password      string
	EnvelopeFrom  string // optional fixed MAIL FROM sent upstream
	HeloName      string
	Timeout       time.Duration
}

// ACME configures automatic certificates (TLS_SOURCE=acme).
type ACME struct {
	Email       string
	Domains     []string
	CAURL       string
	DNSProvider string // cloudflare | route53
	KeyType     string // lego key type: EC256, EC384, RSA2048, RSA3072, RSA4096
	EABKid      string
	EABHMAC     string
	Resolvers   []string
	// Only check the recursive resolvers for the TXT record, not the zone's
	// authoritative servers (for networks that block outbound DNS to them).
	SkipAuthoritative bool
}

func (u Upstream) Addr() string { return fmt.Sprintf("%s:%d", u.Host, u.Port) }

func Load() (*Config, error) {
	var errs []error
	get := func(key, def string) string {
		if v := clean(os.Getenv(key)); v != "" {
			return v
		}
		return def
	}
	secret := func(key string) string {
		if p := os.Getenv(key + "_FILE"); p != "" {
			b, err := os.ReadFile(p)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s_FILE: %w", key, err))
				return ""
			}
			return clean(string(b))
		}
		return clean(os.Getenv(key))
	}
	boolean := func(key string, def bool) bool {
		v := get(key, "")
		if v == "" {
			return def
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", key, err))
		}
		return b
	}
	integer := func(key string, def int) int {
		v := get(key, "")
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", key, err))
		}
		return n
	}
	duration := func(key string, def time.Duration) time.Duration {
		v := get(key, "")
		if v == "" {
			return def
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", key, err))
		}
		return d
	}

	host, _ := os.Hostname()
	c := &Config{
		DataDir:           get("DATA_DIR", "/data"),
		Hostname:          get("SMTP_HOSTNAME", host),
		SubmissionAddr:    get("SMTP_SUBMISSION_ADDR", ":2587"),
		SMTPSAddr:         get("SMTP_SMTPS_ADDR", ":2465"),
		TLSSource:         strings.ToLower(get("TLS_SOURCE", "")),
		TLSCertFile:       get("TLS_CERT_FILE", ""),
		TLSKeyFile:        get("TLS_KEY_FILE", ""),
		AllowInsecureAuth: boolean("SMTP_ALLOW_INSECURE_AUTH", false),
		MaxMessageBytes:   int64(integer("SMTP_MAX_MESSAGE_MB", 25)) << 20,
		MaxRecipients:     integer("SMTP_MAX_RECIPIENTS", 100),
		MaxLineLength:     integer("SMTP_MAX_LINE_LENGTH", 128<<10),
		AuthFailLimit:     integer("SMTP_AUTH_FAIL_LIMIT", 10),
		AuthFailWindow:    duration("SMTP_AUTH_FAIL_WINDOW", 15*time.Minute),

		PublicSubmissionPort: get("SMTP_PUBLIC_SUBMISSION_PORT", "587"),
		PublicSMTPSPort:      get("SMTP_PUBLIC_SMTPS_PORT", "465"),

		Upstream: Upstream{
			Host:          get("UPSTREAM_HOST", ""),
			Port:          integer("UPSTREAM_PORT", 587),
			TLSMode:       strings.ToLower(get("UPSTREAM_TLS", "starttls")),
			TLSSkipVerify: boolean("UPSTREAM_TLS_SKIP_VERIFY", false),
			Username:      get("UPSTREAM_USERNAME", ""),
			Password:      secret("UPSTREAM_PASSWORD"),
			EnvelopeFrom:  get("UPSTREAM_ENVELOPE_FROM", ""),
			HeloName:      get("UPSTREAM_HELO", ""),
			Timeout:       duration("UPSTREAM_TIMEOUT", time.Minute),
		},

		WebAddr:           get("WEB_ADDR", ":8080"),
		SessionSecret:     secret("SESSION_SECRET"),
		SessionTTL:        duration("SESSION_TTL", 8*time.Hour),
		OIDCIssuer:        get("OIDC_ISSUER", ""),
		OIDCClientID:      get("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:  secret("OIDC_CLIENT_SECRET"),
		OIDCScopes:        list(get("OIDC_SCOPES", "openid,email,profile")),
		OIDCAllowedEmails: lower(list(get("OIDC_ALLOWED_EMAILS", ""))),
		OIDCAllowedGroups: list(get("OIDC_ALLOWED_GROUPS", "")),

		OIDCAllowUnverifiedEmail: boolean("OIDC_ALLOW_UNVERIFIED_EMAIL", false),
		OIDCGroupsClaim:          get("OIDC_GROUPS_CLAIM", "groups"),

		LogRetention: time.Duration(integer("LOG_RETENTION_DAYS", 30)) * 24 * time.Hour,
		LogLevel:     get("LOG_LEVEL", "info"),
	}
	if c.SubmissionAddr == "" {
		c.PublicSubmissionPort = ""
	}
	if c.SMTPSAddr == "" {
		c.PublicSMTPSPort = ""
	}
	if c.Upstream.HeloName == "" {
		c.Upstream.HeloName = c.Hostname
	}

	if c.Upstream.Host == "" {
		errs = append(errs, errors.New("UPSTREAM_HOST is required"))
	}
	switch c.Upstream.TLSMode {
	case "starttls", "tls", "none":
	default:
		errs = append(errs, fmt.Errorf("UPSTREAM_TLS must be starttls, tls or none (got %q)", c.Upstream.TLSMode))
	}
	if c.TLSSource == "" {
		c.TLSSource = "self-signed"
		if c.TLSCertFile != "" {
			c.TLSSource = "file"
		}
	}
	switch c.TLSSource {
	case "self-signed":
	case "file":
		if c.TLSCertFile == "" || c.TLSKeyFile == "" {
			errs = append(errs, errors.New("TLS_SOURCE=file needs TLS_CERT_FILE and TLS_KEY_FILE"))
		}
	case "acme":
		c.ACME = ACME{
			Email:       get("ACME_EMAIL", ""),
			Domains:     lower(list(get("ACME_DOMAINS", c.Hostname))),
			DNSProvider: strings.ToLower(get("ACME_DNS_PROVIDER", "")),
			KeyType:     strings.ToUpper(get("ACME_KEY_TYPE", "EC256")),
			EABKid:      get("ACME_EAB_KID", ""),
			EABHMAC:     secret("ACME_EAB_HMAC"),
			Resolvers:   list(get("ACME_DNS_RESOLVERS", "")),

			SkipAuthoritative: boolean("ACME_DNS_SKIP_AUTHORITATIVE", false),
		}
		ca := get("ACME_CA", "letsencrypt")
		if strings.Contains(ca, "://") {
			c.ACME.CAURL = ca
		} else if u, ok := acmeCAs[strings.ToLower(ca)]; ok {
			c.ACME.CAURL = u
		} else {
			errs = append(errs, fmt.Errorf("ACME_CA must be letsencrypt, letsencrypt-staging, zerossl, googletrust or a directory URL (got %q)", ca))
		}
		if !boolean("ACME_ACCEPT_TOS", false) {
			errs = append(errs, errors.New("TLS_SOURCE=acme needs ACME_ACCEPT_TOS=true (you accept the CA's subscriber agreement)"))
		}
		if c.ACME.Email == "" {
			errs = append(errs, errors.New("TLS_SOURCE=acme needs ACME_EMAIL"))
		}
		switch c.ACME.DNSProvider {
		case "cloudflare", "route53":
		default:
			errs = append(errs, fmt.Errorf("ACME_DNS_PROVIDER must be cloudflare or route53 (got %q)", c.ACME.DNSProvider))
		}
		switch c.ACME.KeyType {
		case "EC256", "EC384", "RSA2048", "RSA3072", "RSA4096":
		default:
			errs = append(errs, fmt.Errorf("ACME_KEY_TYPE must be ec256, ec384, rsa2048, rsa3072 or rsa4096 (got %q)", c.ACME.KeyType))
		}
		if (c.ACME.EABKid == "") != (c.ACME.EABHMAC == "") {
			errs = append(errs, errors.New("ACME_EAB_KID and ACME_EAB_HMAC must be set together"))
		}
		if len(c.ACME.Domains) == 0 {
			errs = append(errs, errors.New("ACME_DOMAINS (or SMTP_HOSTNAME) must name at least one domain"))
		}
	default:
		errs = append(errs, fmt.Errorf("TLS_SOURCE must be self-signed, file or acme (got %q)", c.TLSSource))
	}
	if c.SubmissionAddr == "" && c.SMTPSAddr == "" {
		errs = append(errs, errors.New("at least one of SMTP_SUBMISSION_ADDR or SMTP_SMTPS_ADDR must be set"))
	}

	if c.WebAddr != "" {
		raw := get("PUBLIC_URL", "")
		if raw == "" {
			errs = append(errs, errors.New("PUBLIC_URL is required when the panel is enabled (set WEB_ADDR= to disable it)"))
		} else if u, err := url.Parse(strings.TrimRight(raw, "/")); err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("PUBLIC_URL is not an absolute URL: %q", raw))
		} else {
			c.PublicURL = u
		}
		for k, v := range map[string]string{"OIDC_ISSUER": c.OIDCIssuer, "OIDC_CLIENT_ID": c.OIDCClientID, "OIDC_CLIENT_SECRET": c.OIDCClientSecret} {
			if v == "" {
				errs = append(errs, fmt.Errorf("%s is required when the panel is enabled", k))
			}
		}
	}

	return c, errors.Join(errs...)
}

// clean strips surrounding whitespace and one pair of matching quotes, which
// env files and copy-paste often leave behind (`KEY="value"` is passed through
// literally by `docker run --env-file`). A stray space or quote in a client ID
// or password otherwise fails in confusing ways far from here.
func clean(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = strings.TrimSpace(v[1 : len(v)-1])
	}
	return v
}

// Well-known ACME directories, so users can write a name instead of a URL.
var acmeCAs = map[string]string{
	"letsencrypt":         "https://acme-v02.api.letsencrypt.org/directory",
	"letsencrypt-staging": "https://acme-staging-v02.api.letsencrypt.org/directory",
	"zerossl":             "https://acme.zerossl.com/v2/DV90",
	"googletrust":         "https://dv.acme-v02.api.pki.goog/directory",
}

func list(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func lower(in []string) []string {
	for i := range in {
		in[i] = strings.ToLower(in[i])
	}
	return in
}
