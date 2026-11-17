GO   ?= tools/go
# GNU Make has a built-in default LINT=lint (implicit lint rules), so plain
# ?= never takes effect for this name; undefine it first to get our default.
undefine LINT
LINT ?= tools/lint

.PHONY: fmt vet lint test cover generate build bench vuln ui ui-test check

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet -tags=integration ./...

lint:
	$(LINT) run --build-tags=integration ./...

# -coverpkg=./... lets integration tests in one package count for the others.
test:
	$(GO) test -race -count=1 -tags=integration -coverpkg=./... -coverprofile=coverage.out ./...

cover: test
	$(GO) run ./tools/covercheck coverage.out

generate:
	$(GO) generate ./internal/domain/...

build:
	CGO_ENABLED=0 $(GO) build -trimpath -tags=nomsgpack -ldflags "-s -w" -o bin/smtp-gateway ./cmd/smtp-gateway

bench:
	$(GO) test -run '^$$' -bench . -benchmem -tags=integration ./internal/...

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

ui:
	cd ui && bun install --frozen-lockfile && bun run build
	cp -R ui/dist/. internal/interfaces/http/ui/dist/

ui-test:
	cd ui && bun run typecheck && bun run lint && bun run test

check: vet lint cover
