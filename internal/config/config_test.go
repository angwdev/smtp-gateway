package config

import "testing"

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
