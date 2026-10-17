# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
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
