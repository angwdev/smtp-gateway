package acme

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-acme/lego/v5/challenge/dns01"

	"github.com/angwdev/smtp-gateway/internal/config"
	"github.com/angwdev/smtp-gateway/internal/tlsutil"
)

// TestPebble runs the full DNS-01 flow against Pebble (Let's Encrypt's test
// CA) and pebble-challtestsrv (a scriptable DNS server). It is skipped unless
// both binaries are on PATH:
//
//	go install github.com/letsencrypt/pebble/v2/cmd/pebble@latest
//	go install github.com/letsencrypt/pebble/v2/cmd/pebble-challtestsrv@latest
func TestPebble(t *testing.T) {
	pebbleBin, err1 := exec.LookPath("pebble")
	challBin, err2 := exec.LookPath("pebble-challtestsrv")
	if err1 != nil || err2 != nil {
		t.Skip("pebble and pebble-challtestsrv not on PATH")
	}
	dir := t.TempDir()
	dnsAddr, mgmtAddr, acmeAddr, pebbleMgmt := freeAddr(t, "udp"), freeAddr(t, "tcp"), freeAddr(t, "tcp"), freeAddr(t, "tcp")

	// Pebble serves its directory over HTTPS; trust our own CA for it.
	caFile, certFile, keyFile := writePebbleTLS(t, dir)
	t.Setenv("LEGO_CA_CERTIFICATES", caFile)
	t.Setenv("PEBBLE_VA_NOSLEEP", "1")
	t.Setenv("PEBBLE_WFE_NONCEREJECT", "0")

	cfgJSON, _ := json.Marshal(map[string]any{"pebble": map[string]any{
		"listenAddress": acmeAddr, "managementListenAddress": pebbleMgmt,
		"certificate": certFile, "privateKey": keyFile, "httpPort": 5002, "tlsPort": 5001,
		"externalAccountBindingRequired": false,
	}})
	pebbleCfg := filepath.Join(dir, "pebble.json")
	os.WriteFile(pebbleCfg, cfgJSON, 0o644)

	start(t, challBin, "-dnsserver", dnsAddr, "-management", mgmtAddr, "-http01", "", "-https01", "", "-tlsalpn01", "", "-doh", "")
	start(t, pebbleBin, "-config", pebbleCfg, "-dnsserver", dnsAddr)
	waitHTTP(t, "https://"+acmeAddr+"/dir", caFile)

	cfg := config.ACME{
		Email: "ops@example.com", Domains: []string{"smtp.example.test"},
		CAURL: "https://" + acmeAddr + "/dir", DNSProvider: "cloudflare", KeyType: "EC256",
		Resolvers: []string{dnsAddr},
		// challtestsrv is not authoritative for the zone; checking it as a
		// recursive resolver is enough here.
		SkipAuthoritative: true,
	}
	data := t.TempDir()
	loader, err := tlsutil.NewSelfSigned(data, "smtp.example.test")
	if err != nil {
		t.Fatal(err)
	}
	newMgr := func() *Manager {
		m, err := newManager(cfg, data, loader, &challtest{mgmt: "http://" + mgmtAddr})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. First issuance.
	m := newMgr()
	if wait := m.check(ctx, false); m.Status().LastError != "" {
		t.Fatalf("issue failed: %s", m.Status().LastError)
	} else if wait != checkInterval {
		t.Fatalf("next check = %v", wait)
	}
	info := loader.Info()
	if info.Source != tlsutil.SourceACME || len(info.Names) != 1 || info.Names[0] != "smtp.example.test" || !strings.Contains(info.Issuer, "Pebble") {
		t.Fatalf("served cert = %+v", info)
	}
	first := info.NotBefore

	// 2. A fresh certificate is not due; Pebble supplies ARI.
	m = newMgr()
	cert, _, err := m.loadCert()
	if err != nil {
		t.Fatal(err)
	}
	client, err := m.client(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if due, _, ari := m.renewalDue(ctx, client, cert.Leaf); due || !ari {
		t.Fatalf("fresh cert: due=%v ari=%v", due, ari)
	}
	if m.check(ctx, false); m.Status().LastAttempt != (time.Time{}) {
		t.Fatal("renewed a certificate that was not due")
	}

	// 3. Forced renewal (panel button) replaces it, reusing the account.
	time.Sleep(1100 * time.Millisecond) // distinct NotBefore
	m.check(ctx, true)
	if s := m.Status(); s.LastError != "" || s.LastSuccess.IsZero() {
		t.Fatalf("forced renewal: %+v", s)
	}
	if !loader.Info().NotBefore.After(first) {
		t.Fatal("certificate was not replaced")
	}
	if accts, _ := filepath.Glob(filepath.Join(data, "acme", "accounts", "*", "account.json")); len(accts) != 1 {
		t.Fatalf("accounts on disk = %v", accts)
	}

	// 4. Changing the domains triggers a new order.
	cfg.Domains = []string{"smtp.example.test", "mx.example.test"}
	m = newMgr()
	m.check(ctx, false)
	if got := loader.Info().Names; m.Status().LastError != "" || len(got) != 2 {
		t.Fatalf("after domain change: names=%v err=%s", got, m.Status().LastError)
	}
}

func TestLifetimeRenewAt(t *testing.T) {
	day := 24 * time.Hour
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct{ lifetime, want time.Duration }{{90 * day, 60 * day}, {45 * day, 30 * day}, {6 * day, 4 * day}} {
		leaf := &x509.Certificate{NotBefore: base, NotAfter: base.Add(c.lifetime)}
		if got := lifetimeRenewAt(leaf).Sub(base); got != c.want {
			t.Errorf("lifetime %v: renew after %v, want %v", c.lifetime, got, c.want)
		}
	}
}

func TestFailureBackoff(t *testing.T) {
	m := &Manager{}
	var got []time.Duration
	for i := 0; i < 7; i++ {
		got = append(got, m.failed(fmt.Errorf("boom")))
	}
	if got[0] != 5*time.Minute || got[4] != 6*time.Hour || got[6] != 6*time.Hour || m.Status().LastError != "boom" {
		t.Fatalf("backoff = %v, status = %+v", got, m.Status())
	}
}

// challtest is a DNS provider backed by pebble-challtestsrv's management API.
type challtest struct{ mgmt string }

func (c *challtest) Present(ctx context.Context, domain, _, keyAuth string) error {
	info := dns01.GetChallengeInfo(ctx, domain, keyAuth)
	return c.post("/set-txt", map[string]string{"host": info.EffectiveFQDN, "value": info.Value})
}

func (c *challtest) CleanUp(ctx context.Context, domain, _, keyAuth string) error {
	info := dns01.GetChallengeInfo(ctx, domain, keyAuth)
	return c.post("/clear-txt", map[string]string{"host": info.EffectiveFQDN})
}

func (c *challtest) post(path string, body any) error {
	b, _ := json.Marshal(body)
	resp, err := http.Post(c.mgmt+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("challtestsrv %s: %s", path, resp.Status)
	}
	return nil
}

func start(t *testing.T, bin string, args ...string) {
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		if t.Failed() {
			t.Logf("%s output:\n%s", filepath.Base(bin), out.String())
		}
	})
}

func waitHTTP(t *testing.T, url, caFile string) {
	pool := x509.NewCertPool()
	b, _ := os.ReadFile(caFile)
	pool.AppendCertsFromPEM(b)
	c := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: tlsConfig(pool)}}
	for i := 0; i < 100; i++ {
		if resp, err := c.Get(url); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("pebble did not come up at %s", url)
}

func freeAddr(t *testing.T, network string) string {
	if network == "udp" {
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().String()
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func writePebbleTLS(t *testing.T, dir string) (caFile, certFile, keyFile string) {
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test root"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true,
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	ca, _ := x509.ParseCertificate(caDER)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"},
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	keyDER, _ := x509.MarshalECPrivateKey(key)
	caFile, certFile, keyFile = filepath.Join(dir, "ca.pem"), filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o644)
	os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
	return
}

func tlsConfig(pool *x509.CertPool) *tls.Config { return &tls.Config{RootCAs: pool} }
