package main

import (
	"strings"
	"testing"
)

const profile = `mode: atomic
github.com/angwdev/smtp-gateway/internal/domain/shared/errors.go:10.2,12.3 2 1
github.com/angwdev/smtp-gateway/internal/domain/shared/errors.go:14.2,15.3 1 0
github.com/angwdev/smtp-gateway/internal/domain/shared/errors.go:14.2,15.3 1 4
github.com/angwdev/smtp-gateway/internal/application/message/handler.go:5.2,9.3 4 0
github.com/angwdev/smtp-gateway/internal/application/message/handler.go:11.2,12.3 96 1
github.com/angwdev/smtp-gateway/internal/domain/audit/mock_repository.go:1.1,2.2 50 0
github.com/angwdev/smtp-gateway/internal/infrastructure/acme/acme.go:1.1,2.2 50 0
github.com/angwdev/smtp-gateway/cmd/smtp-gateway/main.go:1.1,2.2 10 0
`

func TestCheck(t *testing.T) {
	results, err := check(strings.NewReader(profile))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]result{}
	for _, r := range results {
		got[r.layer] = r
	}
	// The same block seen from two test binaries counts once, covered if either hit it.
	if r := got["domain"]; r.total != 3 || r.covered != 3 || !r.ok {
		t.Errorf("domain = %+v", r)
	}
	// 96 of 100 statements is below the 99% the application layer needs.
	if r := got["application"]; r.total != 100 || r.covered != 96 || r.ok {
		t.Errorf("application = %+v", r)
	}
	// Mocks, the moved ACME code and cmd/ are not gated.
	if r := got["infrastructure"]; r.total != 0 {
		t.Errorf("infrastructure = %+v", r)
	}
}

func TestCheckRejectsGarbage(t *testing.T) {
	if _, err := check(strings.NewReader("mode: atomic\nnot a profile line\n")); err == nil {
		t.Fatal("want an error")
	}
}
