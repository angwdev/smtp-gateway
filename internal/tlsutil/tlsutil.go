// Package tlsutil provides a certificate source that picks up renewed
// certificates from disk, or falls back to a persisted self-signed one.
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

type Loader struct {
	certFile, keyFile string

	mu      sync.Mutex
	cert    *tls.Certificate
	modTime time.Time
	checked time.Time
}

// New returns a loader for certFile/keyFile. When both are empty a
// self-signed certificate for hostname is created in dataDir (once).
func New(certFile, keyFile, dataDir, hostname string) (*Loader, error) {
	if certFile == "" {
		certFile = filepath.Join(dataDir, "selfsigned.crt")
		keyFile = filepath.Join(dataDir, "selfsigned.key")
		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			slog.Warn("no TLS_CERT_FILE set, generating a self-signed certificate", "host", hostname)
			if err := writeSelfSigned(certFile, keyFile, hostname); err != nil {
				return nil, err
			}
		}
	}
	l := &Loader{certFile: certFile, keyFile: keyFile}
	if err := l.reload(); err != nil {
		return nil, err
	}
	return l, nil
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
	if time.Since(l.checked) > time.Minute {
		l.checked = time.Now()
		if st, err := os.Stat(l.certFile); err == nil && !st.ModTime().Equal(l.modTime) {
			if err := l.reloadLocked(); err != nil {
				slog.Error("tls reload failed, keeping previous certificate", "err", err)
			} else {
				slog.Info("tls certificate reloaded", "file", l.certFile)
			}
		}
	}
	return l.cert
}

func (l *Loader) reload() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reloadLocked()
}

func (l *Loader) reloadLocked() error {
	st, err := os.Stat(l.certFile)
	if err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(l.certFile, l.keyFile)
	if err != nil {
		return fmt.Errorf("load tls keypair: %w", err)
	}
	l.cert, l.modTime, l.checked = &cert, st.ModTime(), time.Now()
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
