// Package acme obtains and renews the SMTP certificate with lego, using the
// DNS-01 challenge (Cloudflare or Route 53), so the gateway needs no inbound
// HTTP port. Renewal follows the CA's ACME Renewal Information (RFC 9773)
// when offered, otherwise it starts once a third of the lifetime remains.
package acme

import (
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/acme/api"
	"github.com/go-acme/lego/v5/certcrypto"
	"github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/challenge/dns01"
	"github.com/go-acme/lego/v5/lego"
	"github.com/go-acme/lego/v5/providers/dns/cloudflare"
	"github.com/go-acme/lego/v5/providers/dns/route53"
	"github.com/go-acme/lego/v5/registration"

	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/tlsutil"
)

// Providers lists the supported DNS providers.
var Providers = []string{"cloudflare", "route53"}

// How often to re-evaluate renewal when nothing is due.
const checkInterval = 12 * time.Hour

// Status is what the panel shows about certificate management.
type Status struct {
	Provider    string
	CA          string
	Domains     []string
	Renewing    bool
	LastAttempt time.Time
	LastSuccess time.Time
	LastError   string
	NextCheck   time.Time
	ARI         bool // renewal window came from the CA
}

type Manager struct {
	cfg    config.ACME
	dir    string
	loader *tlsutil.Loader

	provider challenge.Provider

	trigger chan struct{}

	mu       sync.Mutex
	status   Status
	failures int
}

// New prepares a manager and validates the DNS provider credentials. If a
// previously issued certificate is on disk it is served immediately.
func New(cfg config.ACME, dataDir string, loader *tlsutil.Loader) (*Manager, error) {
	p, err := newDNSProvider(cfg.DNSProvider)
	if err != nil {
		return nil, err
	}
	return newManager(cfg, dataDir, loader, p)
}

func newManager(cfg config.ACME, dataDir string, loader *tlsutil.Loader, p challenge.Provider) (*Manager, error) {
	m := &Manager{
		cfg:      cfg,
		dir:      filepath.Join(dataDir, "acme"),
		loader:   loader,
		provider: p,
		trigger:  make(chan struct{}, 1),
		status:   Status{Provider: cfg.DNSProvider, CA: cfg.CAURL, Domains: cfg.Domains},
	}
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return nil, err
	}
	if len(cfg.Resolvers) > 0 {
		dns01.SetDefaultClient(dns01.NewClient(&dns01.Options{RecursiveNameservers: cfg.Resolvers}))
	}
	if cert, meta, err := m.loadCert(); err == nil && meta.matches(cfg) {
		loader.Set(cert, tlsutil.SourceACME)
		slog.Info("acme: serving stored certificate", "domains", strings.Join(meta.Domains, ","), "expires", cert.Leaf.NotAfter.Format(time.DateOnly))
	}
	return m, nil
}

func newDNSProvider(name string) (challenge.Provider, error) {
	switch name {
	case "cloudflare":
		p, err := cloudflare.NewDNSProvider()
		if err != nil {
			return nil, fmt.Errorf("acme dns provider: %w (set CLOUDFLARE_DNS_API_TOKEN with Zone:Read and DNS:Edit)", err)
		}
		return p, nil
	case "route53":
		p, err := route53.NewDNSProvider()
		if err != nil {
			return nil, fmt.Errorf("acme dns provider: %w (set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_REGION or an IAM role)", err)
		}
		return p, nil
	}
	return nil, fmt.Errorf("ACME_DNS_PROVIDER must be one of %s (got %q)", strings.Join(Providers, ", "), name)
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	s.Domains = slices.Clone(s.Domains)
	return s
}

// RenewNow asks the manager to renew as soon as possible (panel button).
func (m *Manager) RenewNow() {
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

// Run keeps the certificate current until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	force := false
	for {
		wait := m.check(ctx, force)
		force = false
		m.mu.Lock()
		m.status.NextCheck = time.Now().Add(wait)
		m.mu.Unlock()
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-m.trigger:
			t.Stop()
			force = true
		case <-t.C:
		}
	}
}

// check renews when due and returns how long to wait before the next check.
func (m *Manager) check(ctx context.Context, force bool) time.Duration {
	cert, meta, err := m.loadCert()
	current := err == nil && meta.matches(m.cfg)

	var client *lego.Client
	reason := "forced from the panel"
	if !force {
		if !current {
			reason = "no certificate for the configured domains yet"
		} else {
			client, err = m.client(ctx)
			if err != nil {
				return m.failed(err)
			}
			due, wait, ari := m.renewalDue(ctx, client, cert.Leaf)
			m.mu.Lock()
			m.status.ARI = ari
			m.mu.Unlock()
			if !due {
				return wait
			}
			reason = "renewal window reached"
		}
	}

	slog.Info("acme: requesting certificate", "domains", strings.Join(m.cfg.Domains, ","), "reason", reason)
	m.mu.Lock()
	m.status.Renewing, m.status.LastAttempt = true, time.Now()
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.status.Renewing = false
		m.mu.Unlock()
	}()

	if client == nil {
		if client, err = m.client(ctx); err != nil {
			return m.failed(err)
		}
	}
	var prev *x509.Certificate
	if current {
		prev = cert.Leaf
	}
	if err := m.obtain(ctx, client, prev); err != nil {
		return m.failed(err)
	}
	m.mu.Lock()
	m.failures, m.status.LastError, m.status.LastSuccess = 0, "", time.Now()
	m.mu.Unlock()
	return checkInterval
}

// renewalDue decides whether leaf should be renewed now. ARI from the CA
// wins; without it, renew when a third of the lifetime remains.
func (m *Manager) renewalDue(ctx context.Context, client *lego.Client, leaf *x509.Certificate) (due bool, wait time.Duration, ari bool) {
	now := time.Now()
	if info, err := client.Certificate.GetRenewalInfo(ctx, leaf); err == nil {
		at := info.ShouldRenewAt(now, checkInterval)
		if at == nil {
			return false, checkInterval, true
		}
		if !at.After(now) {
			return true, 0, true
		}
		return false, at.Sub(now), true
	} else if !errors.Is(err, api.ErrNoARI) {
		slog.Debug("acme: renewal info unavailable, using lifetime", "err", err)
	}
	renewAt := lifetimeRenewAt(leaf)
	if !renewAt.After(now) {
		return true, 0, false
	}
	return false, min(renewAt.Sub(now), checkInterval), false
}

// lifetimeRenewAt is the fallback renewal time when the CA offers no ARI:
// once a third of the lifetime remains (day 60 of 90, day 30 of 45).
func lifetimeRenewAt(leaf *x509.Certificate) time.Time {
	return leaf.NotAfter.Add(-leaf.NotAfter.Sub(leaf.NotBefore) / 3)
}

func (m *Manager) failed(err error) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures++
	m.status.LastError = err.Error()
	// Stay well inside Let's Encrypt's failed-validation limits.
	backoff := []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour}
	wait := backoff[min(m.failures, len(backoff))-1]
	slog.Error("acme: certificate request failed", "err", err, "retry_in", wait)
	return wait
}

func (m *Manager) obtain(ctx context.Context, client *lego.Client, prev *x509.Certificate) error {
	// Surface why a propagation check is still failing (LOG_LEVEL=debug);
	// lego otherwise only reports it when the overall timeout expires.
	opts := []dns01.ChallengeOption{dns01.WrapPreCheck(func(ctx context.Context, domain, fqdn, value string, check dns01.PreCheckFunc) (bool, error) {
		ok, err := check(ctx, fqdn, value)
		if err != nil {
			slog.Debug("acme: dns record not visible yet", "fqdn", fqdn, "reason", err)
		}
		return ok, err
	})}
	if m.cfg.SkipAuthoritative {
		opts = append(opts, dns01.DisableAuthoritativeNssPropagationRequirement())
	}
	if err := client.Challenge.SetDNS01Provider(m.provider, opts...); err != nil {
		return err
	}
	req := certificate.ObtainRequest{Domains: m.cfg.Domains, KeyType: certcrypto.KeyType(m.cfg.KeyType), Bundle: true}
	if prev != nil {
		if id, err := api.MakeARICertID(prev); err == nil {
			req.ReplacesCertID = id
		}
	}
	res, err := client.Certificate.Obtain(ctx, req)
	if err != nil && req.ReplacesCertID != "" {
		// A CA may refuse the "replaces" hint (e.g. already replaced); a
		// plain order still gets a certificate.
		slog.Warn("acme: order with replaces failed, retrying without it", "err", err)
		req.ReplacesCertID = ""
		res, err = client.Certificate.Obtain(ctx, req)
	}
	if err != nil {
		return err
	}
	cert, err := tls.X509KeyPair(res.Certificate, res.PrivateKey)
	if err != nil {
		return fmt.Errorf("issued certificate unusable: %w", err)
	}
	meta := certMeta{Domains: m.cfg.Domains, CA: m.cfg.CAURL, KeyType: m.cfg.KeyType, CertURL: res.CertURL}
	if err := m.saveCert(res.Certificate, res.PrivateKey, meta); err != nil {
		return err
	}
	m.loader.Set(&cert, tlsutil.SourceACME)
	slog.Info("acme: certificate installed", "domains", strings.Join(m.cfg.Domains, ","), "expires", m.loader.Info().NotAfter.Format(time.DateOnly))
	return nil
}

// ---- account ---------------------------------------------------------------

type user struct {
	Email        string                `json:"email"`
	Registration *acme.ExtendedAccount `json:"registration"`
	key          crypto.Signer
}

func (u *user) GetEmail() string                       { return u.Email }
func (u *user) GetRegistration() *acme.ExtendedAccount { return u.Registration }
func (u *user) GetPrivateKey() crypto.Signer           { return u.key }

// accountDir is per CA, so switching between staging and production keeps
// both accounts.
func (m *Manager) accountDir() string {
	sum := sha256.Sum256([]byte(m.cfg.CAURL))
	return filepath.Join(m.dir, "accounts", hex.EncodeToString(sum[:6]))
}

func (m *Manager) client(ctx context.Context) (*lego.Client, error) {
	dir := m.accountDir()
	u := &user{Email: m.cfg.Email}
	if keyPEM, err := os.ReadFile(filepath.Join(dir, "account.key")); err == nil {
		k, err := certcrypto.ParsePEMPrivateKey(keyPEM)
		if err != nil {
			return nil, fmt.Errorf("account key: %w", err)
		}
		u.key = k
		if b, err := os.ReadFile(filepath.Join(dir, "account.json")); err == nil {
			_ = json.Unmarshal(b, u)
			u.Email = m.cfg.Email
		}
	} else {
		k, err := certcrypto.GeneratePrivateKey(certcrypto.EC256)
		if err != nil {
			return nil, err
		}
		u.key = k
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		if err := writeFileAtomic(filepath.Join(dir, "account.key"), certcrypto.PEMEncode(k), 0o600); err != nil {
			return nil, err
		}
	}

	lc := lego.NewConfig(u)
	lc.CADirURL = m.cfg.CAURL
	lc.UserAgent = "smtp-gateway"
	client, err := lego.NewClient(lc)
	if err != nil {
		return nil, fmt.Errorf("acme client: %w", err)
	}

	if u.Registration == nil {
		var reg *acme.ExtendedAccount
		if m.cfg.EABKid != "" {
			reg, err = client.Registration.RegisterWithExternalAccountBinding(ctx, registration.RegisterEABOptions{
				TermsOfServiceAgreed: true, Kid: m.cfg.EABKid, HmacEncoded: m.cfg.EABHMAC,
			})
		} else {
			reg, err = client.Registration.Register(ctx, registration.RegisterOptions{TermsOfServiceAgreed: true})
		}
		if err != nil {
			return nil, fmt.Errorf("acme account registration: %w", err)
		}
		u.Registration = reg
		b, _ := json.MarshalIndent(u, "", "  ")
		if err := writeFileAtomic(filepath.Join(dir, "account.json"), b, 0o600); err != nil {
			return nil, err
		}
		slog.Info("acme: account registered", "ca", m.cfg.CAURL, "email", m.cfg.Email)
	}
	return client, nil
}

// ---- certificate storage ---------------------------------------------------

type certMeta struct {
	Domains []string `json:"domains"`
	CA      string   `json:"ca"`
	KeyType string   `json:"keyType"`
	CertURL string   `json:"certUrl"`
}

// matches reports whether the stored certificate was issued for the current
// configuration; any change triggers a new order.
func (c certMeta) matches(cfg config.ACME) bool {
	return slices.Equal(c.Domains, cfg.Domains) && c.CA == cfg.CAURL && c.KeyType == cfg.KeyType
}

func (m *Manager) loadCert() (*tls.Certificate, certMeta, error) {
	var meta certMeta
	b, err := os.ReadFile(filepath.Join(m.dir, "cert.json"))
	if err != nil {
		return nil, meta, err
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, meta, err
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(m.dir, "cert.pem"), filepath.Join(m.dir, "key.pem"))
	if err != nil {
		return nil, meta, err
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, meta, err
	}
	cert.Leaf = leaf
	return &cert, meta, nil
}

func (m *Manager) saveCert(certPEM, keyPEM []byte, meta certMeta) error {
	b, _ := json.MarshalIndent(meta, "", "  ")
	// Key first: a crash between writes leaves a mismatched pair that fails
	// to load and is simply re-issued, never a cert served with a wrong key.
	for _, f := range []struct {
		name string
		data []byte
		mode os.FileMode
	}{{"key.pem", keyPEM, 0o600}, {"cert.pem", certPEM, 0o644}, {"cert.json", b, 0o644}} {
		if err := writeFileAtomic(filepath.Join(m.dir, f.name), f.data, f.mode); err != nil {
			return err
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
