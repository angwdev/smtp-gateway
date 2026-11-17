# syntax=docker/dockerfile:1
# Both build stages run natively and cross-compile (pure Go, no cgo), so
# multi-arch images build fast without QEMU.

FROM --platform=$BUILDPLATFORM oven/bun:1.3.14-alpine AS ui
WORKDIR /ui
COPY ui/package.json ui/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile
COPY ui/ ./
RUN bun run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=ui /ui/dist/ internal/interfaces/http/ui/dist/
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -tags=nomsgpack -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/smtp-gateway ./cmd/smtp-gateway \
 && mkdir -p /out/data

# Final image: one static binary + CA roots, no shell, no package manager.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/smtp-gateway /smtp-gateway
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
ENV DATA_DIR=/data
VOLUME /data
EXPOSE 2587 2465 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/smtp-gateway", "healthcheck"]
ENTRYPOINT ["/smtp-gateway"]
CMD ["serve"]
