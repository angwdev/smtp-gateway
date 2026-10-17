// Package tlsutil serves the SMTP certificate: from files on disk (picked up
// again when renewed), a persisted self-signed one, or one set at runtime by
// the ACME manager.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Certificate sources, as shown in the panel.
const (
	SourceFile       = "file"
	SourceSelfSigned = "self-signed"
	SourceACME       = "acme"
)

// Info describes the certificate currently being served.
type Info struct {
	Source    string
	Names     []string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
}

func (i Info) Remaining() time.Duration { return time.Until(i.NotAfter) }

type Loader struct {
	mu   sync.Mutex
	cert *tls.Certificate
	info Info

	// Set for SourceFile: the files are re-read when they change.
	certFile, keyFile string
	modTime           time.Time
	checked           time.Time
}

// NewFile serves certFile/keyFile and reloads them when they change.
func NewFile(certFile, keyFile string) (*Loader, error) {
	l := &Loader{certFile: certFile, keyFile: keyFile}
	if err := l.reloadFile(); err != nil {
		return nil, err
	}
	return l, nil
}

// NewSelfSigned serves a self-signed certificate for hostname, created once
// in dataDir and reused afterwards.
func NewSelfSigned(dataDir, hostname string) (*Loader, error) {
	certFile := filepath.Join(dataDir, "selfsigned.crt")
	keyFile := filepath.Join(dataDir, "selfsigned.key")
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		if err := writeSelfSigned(certFile, keyFile, hostname); err != nil {
			return nil, err
		}
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load self-signed certificate: %w", err)
	}
	l := &Loader{}
	l.set(&cert, SourceSelfSigned)
	return l, nil
}

// Set replaces the served certificate (used by the ACME manager).
func (l *Loader) Set(cert *tls.Certificate, source string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.set(cert, source)
}

func (l *Loader) set(cert *tls.Certificate, source string) {
	l.cert = cert
	l.info = Info{Source: source}
	if leaf, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
		cert.Leaf = leaf
		l.info.Names = leaf.DNSNames
		if len(l.info.Names) == 0 && leaf.Subject.CommonName != "" {
			l.info.Names = []string{leaf.Subject.CommonName}
		}
		l.info.Issuer = leaf.Issuer.CommonName
		if len(leaf.Issuer.Organization) > 0 {
			l.info.Issuer = leaf.Issuer.Organization[0] + " " + leaf.Issuer.CommonName
		}
		l.info.NotBefore, l.info.NotAfter = leaf.NotBefore, leaf.NotAfter
	}
}

func (l *Loader) Info() Info {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.info
}

func (l *Loader) Config() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return l.get(), nil
		},
	}
}

func (l *Loader) get() *tls.Certificate {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.certFile != "" && time.Since(l.checked) > time.Minute {
		l.checked = time.Now()
		if st, err := os.Stat(l.certFile); err == nil && !st.ModTime().Equal(l.modTime) {
			if err := l.reloadFileLocked(); err != nil {
				slog.Error("tls reload failed, keeping previous certificate", "err", err)
			} else {
				slog.Info("tls certificate reloaded", "file", l.certFile)
			}
		}
	}
	return l.cert
}

func (l *Loader) reloadFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reloadFileLocked()
}

func (l *Loader) reloadFileLocked() error {
	st, err := os.Stat(l.certFile)
	if err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(l.certFile, l.keyFile)
	if err != nil {
		return fmt.Errorf("load tls keypair: %w", err)
	}
	l.set(&cert, SourceFile)
	l.modTime, l.checked = st.ModTime(), time.Now()
	return nil
}

func writeSelfSigned(certFile, keyFile, hostname string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname},
		Issuer:       pkix.Name{CommonName: hostname},
		DNSNames:     []string{hostname},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}
