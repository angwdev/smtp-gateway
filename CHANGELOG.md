# Changelog

## 0.2.0

### Added

- **HTTP API** under `/api/v1`, versioned and documented by an OpenAPI 3.1
  document served at `/api/v1/openapi.yaml`. Every panel feature (overview,
  credentials, messages, audit, upstream test, certificate renewal) is now also
  reachable as JSON.
- **API tokens**, created and revoked in the panel, sent as
  `Authorization: Bearer sgw_…`, scoped to `read` or `write`. Tokens cannot create
  or revoke other tokens, even with `write` scope. Tokens are hashed at rest and
  shown once.
- **Message subject and full header block** added to the message log
  (`subject`, and `message_headers` kept separate from the hot `messages`
  table), controlled by the new `LOG_MESSAGE_CONTENT` setting
  (`headers`/`subject`/`none`). The body is still never stored.
- **Richer audit log**: `audit_events` replaces `audit` with `actor_type`,
  `target_type`/`target_id`/`target_label`, `result`, `ip`, `user_agent`,
  `request_id` and a `changes` diff. SMTP AUTH failures and lockouts
  (`smtp.auth_failed`, `smtp.lockout`), bad API tokens (`token.auth_failed`) and
  audited API calls (`api.call`) are now logged too — 17 actions in total.
- **`WEB_TRUSTED_PROXIES`**: an explicit, empty-by-default allow-list of reverse
  proxies whose `X-Forwarded-For` is trusted, so audit `ip` values and rate
  limiting are correct behind Traefik/Caddy/nginx without ever trusting a
  header from an untrusted peer.
- A React (MUI + Tailwind v4) panel replaces the server-rendered pages, built
  with Vite and embedded in the binary; same OIDC login, same URLs for `/login`
  and `/auth/callback`.

### Changed

- **Rebuilt internals** as a DDD-layered Gin + GORM + Goose service
  (`internal/domain` / `application` / `infrastructure` / `interfaces`), on top
  of the pure-Go `modernc.org/sqlite` driver (no cgo; the image stays a `scratch`
  image with one static binary).
- **Separate reader/writer DB pools**: one writer (`_txlock=immediate`) and four
  readers (`query_only`), so a slow panel or API query no longer queues behind
  SMTP AUTH or message logging.
- **Message writes are one transaction** (message row, header row, `sent_count`
  bump), instead of two unguarded statements.
- **`last_used_at`** on a credential is now written at most once a minute per
  credential (or when the IP changes), instead of on every AUTH.
- **Panel sessions are server-tracked** (`sessions` table, `sha256` of an opaque
  id in the cookie), so logout actually revokes a session instead of only
  clearing the cookie.
- **Panel routes are now a client-side single-page app**; the old
  server-rendered paths redirect to their new equivalents.

### Security

- **Secret generation uses rejection sampling**, removing the old modulo bias
  in password and token generation.
- **CSRF** is enforced on every `/api/v1` request with Go 1.25's
  `http.CrossOriginProtection` (no token, no custom header needed).
- **Stricter CSP** with a per-request nonce (`style-src-elem 'self'
  'nonce-…'`), plus `X-Content-Type-Options`, `Referrer-Policy`,
  `X-Frame-Options: DENY`, `Cross-Origin-Opener-Policy`,
  `Cross-Origin-Resource-Policy`, `Permissions-Policy`, and HSTS when
  `PUBLIC_URL` is https.
- **Rate limiting** added on the API (20 req/s, burst 60, per IP), on
  `/login`/`/auth/callback` (10/minute per IP), and a token bucket on audit
  rows themselves so a brute-force attempt can't flood the audit log.
- **Supply-chain scanning** (`govulncheck` for Go, `bun audit
  --audit-level=high` for the UI) now gates CI.

### Upgrade notes

- The database migrates itself on first start (Goose runs before the server
  accepts connections); back up `gateway.db` first regardless.
- Everyone's panel session ends on upgrade (sign in again); SMTP clients need
  no change at all — same hosts, ports, AUTH mechanisms and credentials.
- See the README's "Upgrading from 0.1" and "Behind a reverse proxy" sections
  for the full detail.

## 0.1.0

Initial release: credential-translating SMTP relay with an OIDC-protected panel,
per-credential sender policy and rate limits, per-IP brute-force lockout, ACME
(DNS-01) or file-based TLS certificates, and a SQLite-backed message and audit
log.
