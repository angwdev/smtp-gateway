package config

import (
	"os"
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"abc":        "abc",
		"  abc \r\n": "abc",
		`"abc"`:      "abc",
		`'abc'`:      "abc",
		` "a b" `:    "a b",
		`"abc`:       `"abc`,
		`a"b"`:       `a"b"`,
		`""`:         "",
		"pass#word":  "pass#word",
	} {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadTrimsQuotedOIDCValues(t *testing.T) {
	t.Setenv("UPSTREAM_HOST", "mail.example.com")
	t.Setenv("PUBLIC_URL", "https://panel.example.com/")
	t.Setenv("OIDC_ISSUER", " https://auth.example.com/application/o/smtp/ ")
	t.Setenv("OIDC_CLIENT_ID", `"AbC123xyz"`)
	t.Setenv("OIDC_CLIENT_SECRET", "s3cret\r")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.OIDCClientID != "AbC123xyz" || c.OIDCClientSecret != "s3cret" || c.OIDCIssuer != "https://auth.example.com/application/o/smtp/" {
		t.Fatalf("got id=%q secret=%q issuer=%q", c.OIDCClientID, c.OIDCClientSecret, c.OIDCIssuer)
	}
	if c.PublicURL.String() != "https://panel.example.com" {
		t.Fatalf("public url = %q", c.PublicURL)
	}
}

func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range []string{"WEB_ADDR", "SMTP_SUBMISSION_ADDR", "SMTP_SMTPS_ADDR", "WEB_TRUSTED_PROXIES", "LOG_MESSAGE_CONTENT", "TLS_SOURCE"} {
		t.Setenv(k, "")
		os.Unsetenv(k) //nolint:errcheck // restored by t.Setenv's cleanup
	}
	t.Setenv("UPSTREAM_HOST", "mail.example.com")
	t.Setenv("PUBLIC_URL", "https://panel.example.com")
	t.Setenv("OIDC_ISSUER", "https://id.example.com")
	t.Setenv("OIDC_CLIENT_ID", "panel")
	t.Setenv("OIDC_CLIENT_SECRET", "s")
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoadV2Settings(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		check   func(t *testing.T, c *Config)
	}{
		{"defaults", nil, "", func(t *testing.T, c *Config) {
			if len(c.TrustedProxies) != 0 || c.MessageContent != "headers" || c.WebAddr != ":8080" {
				t.Fatalf("got %+v %q %q", c.TrustedProxies, c.MessageContent, c.WebAddr)
			}
		}},
		{"proxies and policy", map[string]string{"WEB_TRUSTED_PROXIES": "172.23.1.0/24, fd94:a92:bc8b::/64 10.0.0.1", "LOG_MESSAGE_CONTENT": "Subject"}, "", func(t *testing.T, c *Config) {
			if len(c.TrustedProxies) != 3 || c.MessageContent != "subject" {
				t.Fatalf("got %+v %q", c.TrustedProxies, c.MessageContent)
			}
		}},
		{"bad proxy", map[string]string{"WEB_TRUSTED_PROXIES": "traefik"}, "WEB_TRUSTED_PROXIES", nil},
		{"bad policy", map[string]string{"LOG_MESSAGE_CONTENT": "body"}, "LOG_MESSAGE_CONTENT", nil},
		{"empty WEB_ADDR turns the panel off", map[string]string{"WEB_ADDR": "", "PUBLIC_URL": ""}, "", func(t *testing.T, c *Config) {
			if c.WebAddr != "" {
				t.Fatalf("WebAddr = %q", c.WebAddr)
			}
		}},
		{"empty submission listener is off", map[string]string{"SMTP_SUBMISSION_ADDR": ""}, "", func(t *testing.T, c *Config) {
			if c.SubmissionAddr != "" || c.SMTPSAddr != ":2465" || c.PublicSubmissionPort != "" {
				t.Fatalf("got %q %q %q", c.SubmissionAddr, c.SMTPSAddr, c.PublicSubmissionPort)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.env)
			c, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to mention %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, c)
		})
	}
}
