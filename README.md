# SMTP Gateway

A small, self-hosted, **credential-translating SMTP relay**. Every client, app or
server gets its own SMTP login, which you create in an SSO-protected web panel. The
gateway checks that login, applies per-login policy, and relays the mail to your real
mail server using **one** upstream credential.

```
 Nextcloud ─┐  user: nextcloud / pw: …                     ┌───────────────────────┐
 Grafana  ──┼─ user: grafana   / pw: …  ──►  SMTP Gateway  ──►  mail.example.com:587 │
 Backups  ──┘  user: backups   / pw: …      (checks login,   │  user: relay@example.com
                                             sender, rate)   └───────────────────────┘
                       ▲
         Panel (OIDC SSO): create / rotate / disable logins, message log, audit log
```

- One static Go binary in a `scratch` image (about 6 MB compressed). It typically uses
  under 25 MB of RAM and almost no CPU when idle.
- Mail is **streamed** to the upstream server and never queued on disk. The client
  gets the upstream's own reply, so a message is either accepted by your mail server
  or the client knows it failed and retries.
- SQLite holds credentials, a message log and an audit log. Nothing else is needed:
  no Redis, no Postgres, no Postfix.

## Why this design

| Option | Per-client logins | Panel + SSO | Containers | Notes |
|---|---|---|---|---|
| Postfix/Exim relay images (`docker-exim-relay`, `zknt/smtprelay`, …) | ✗ (IP allow-lists) | ✗ | 1 | One upstream login, but clients are trusted by network, not identity |
| `decke/smtprelay` | ✓ (bcrypt users file) | ✗ | 1 | No panel, no SSO, no per-user rate limits or logs |
| Postfix + SASL + SQL + a separate panel + OAuth proxy | ✓ | DIY | 3–5 | Works, but heavy and fiddly |
| Postal / Mailcow / Mailu | ✓ | partial | many | Full mail platforms; far more than a relay needs |
| **This gateway** | ✓ | ✓ (OIDC built in) | **1** | Per-login sender rules, rate limits, logs, instant revoke |

## Features

- **Per-identity credentials** with server-generated 32-character passwords. A
  password is shown once, can be rotated, and a credential can be disabled or
  deleted on its own. Disabling also refuses the next message on connections that
  are already open.
- **Sender policy per credential**: exact addresses, `@domain`, `@*.domain` or `*`.
  It is checked against both the envelope `MAIL FROM` and the `From:` header, so a
  login can't spoof another sender.
- **Rate limit per credential** (messages per hour).
- **Brute-force lockout** per client IP.
- **TLS**: STARTTLS on 587 and implicit TLS on 465. Use your own certificate, which
  is reloaded automatically when renewed, or let the gateway generate a self-signed
  one. AUTH without TLS is refused unless you turn it on explicitly.
- **AUTH PLAIN and LOGIN** for clients. Upstream can use STARTTLS, implicit TLS or
  plain, with PLAIN or LOGIN auth. You can optionally force a fixed envelope sender
  upstream (`UPSTREAM_ENVELOPE_FROM`).
- **Panel**: OIDC (authorization code + PKCE) with any provider (Authentik,
  Authelia, Keycloak, Zitadel, Pocket ID, Kanidm, Google, Entra ID…). You can limit
  access by e-mail, by domain or by group. It has CSRF protection, a strict CSP and
  no JavaScript framework. It shows the message log (searchable, retention
  configurable), an audit log of every sign-in and change, and a button that tests
  the upstream connection.

## Quick start

```sh
cp .env.example .env    # fill in UPSTREAM_*, PUBLIC_URL and OIDC_*
docker compose up -d --build
docker compose logs -f
```

1. **Register an OIDC client** in your identity provider:
   - Redirect URI: `${PUBLIC_URL}/auth/callback`
   - Post-logout redirect URI (optional): `${PUBLIC_URL}/`
   - Type: confidential (client secret). Scopes: `openid email profile`, plus
     `groups` if you use `OIDC_ALLOWED_GROUPS` and your IdP needs that scope.
2. **Expose the panel** (container port 8080, bound to `127.0.0.1:8080` by default)
   through your reverse proxy at `PUBLIC_URL`. Example for Caddy:
   `smtp-panel.example.com { reverse_proxy 127.0.0.1:8080 }`.
3. **Sign in**, click **New credential** and copy the password shown.
4. **Point your clients** at the gateway: host = `SMTP_HOSTNAME`, port 587 (STARTTLS)
   or 465 (SSL/TLS), with the username and password you just created.

You can also create a credential without the panel (for bootstrap or scripts):

```sh
docker compose exec smtp-gateway /smtp-gateway add-credential grafana alerts@example.com
```

### Clients on the same Docker host

Put the client container on the same Docker network and use
`smtp-gateway:2587` with STARTTLS. With the self-signed certificate, tell the
client not to verify it. Alternatively, keep the gateway reachable only on an
internal network and set `SMTP_ALLOW_INSECURE_AUTH=true` for devices that can't do
TLS, such as old printers and NAS boxes.

### Using a real certificate

Mount the certificate and set the paths. The files are re-read within a minute of
changing, so no restart is needed on renewal:

```yaml
    volumes:
      - gateway-data:/data
      - ./certs:/certs:ro     # fullchain.pem + privkey.pem
```
```env
TLS_CERT_FILE=/certs/fullchain.pem
TLS_KEY_FILE=/certs/privkey.pem
```

The container runs as UID 65532, so the key must be readable by that user. For
example, use a certbot `--deploy-hook` that copies the files into `./certs` with
`chmod 640` and `chown 65532`.

## Configuration

Everything is set with environment variables (see [`.env.example`](.env.example)).
Variables that hold secrets (`UPSTREAM_PASSWORD`, `OIDC_CLIENT_SECRET`,
`SESSION_SECRET`) also accept a `_FILE` suffix for Docker secrets.

| Variable | Default | |
|---|---|---|
| `UPSTREAM_HOST` | *(required)* | Your real mail server |
| `UPSTREAM_PORT` | `587` | |
| `UPSTREAM_TLS` | `starttls` | `starttls`, `tls` (port 465) or `none` |
| `UPSTREAM_USERNAME` / `UPSTREAM_PASSWORD` | | The single upstream credential (empty username = no auth) |
| `UPSTREAM_ENVELOPE_FROM` | | Force this `MAIL FROM` upstream; the `From:` header is untouched |
| `UPSTREAM_TLS_SKIP_VERIFY` | `false` | |
| `UPSTREAM_HELO` | `SMTP_HOSTNAME` | |
| `UPSTREAM_TIMEOUT` | `1m` | Connect and per-command timeout |
| `SMTP_HOSTNAME` | container hostname | Greeting name and self-signed cert name; shown in the panel |
| `SMTP_SUBMISSION_ADDR` | `:2587` | STARTTLS listener (empty = off) |
| `SMTP_SMTPS_ADDR` | `:2465` | Implicit-TLS listener (empty = off) |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | | Self-signed cert in `/data` if unset |
| `SMTP_ALLOW_INSECURE_AUTH` | `false` | Allow AUTH over plaintext |
| `SMTP_MAX_MESSAGE_MB` | `25` | |
| `SMTP_MAX_RECIPIENTS` | `100` | Per message |
| `SMTP_MAX_LINE_LENGTH` | `131072` | Larger than RFC 5321's 1000 because many apps send unwrapped HTML |
| `SMTP_AUTH_FAIL_LIMIT` / `SMTP_AUTH_FAIL_WINDOW` | `10` / `15m` | Per-IP lockout |
| `SMTP_PUBLIC_SUBMISSION_PORT` / `SMTP_PUBLIC_SMTPS_PORT` | `587` / `465` | Only used for display in the panel |
| `WEB_ADDR` | `:8080` | Panel listener (empty = panel off) |
| `PUBLIC_URL` | *(required with panel)* | External URL of the panel |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | *(required with panel)* | |
| `OIDC_SCOPES` | `openid,email,profile` | |
| `OIDC_ALLOWED_EMAILS` | | Comma-separated; `@domain` entries allowed; needs a verified e-mail |
| `OIDC_ALLOWED_GROUPS` | | Matched against `OIDC_GROUPS_CLAIM` (default `groups`) |
| `SESSION_TTL` | `8h` | |
| `SESSION_SECRET` | auto | Generated and stored in the database if unset |
| `LOG_RETENTION_DAYS` | `30` | For the message and audit logs |
| `LOG_LEVEL` | `info` | |

If both allow-lists are empty, anyone your IdP authenticates for this client can
sign in. A warning is logged at startup in that case.

## How a message flows

1. The client connects and authenticates (STARTTLS/TLS, then AUTH PLAIN or LOGIN).
   The password is checked against a SHA-256 hash in SQLite. Passwords are always
   random 190-bit secrets, so a fast hash is safe here, just like with API tokens.
2. On `MAIL FROM`, the credential is re-read (so a disable takes effect at once).
   Then the envelope sender is checked against the allowed list and the hourly
   limit is checked. The gateway then opens or reuses an authenticated upstream
   connection.
3. Each `RCPT TO` is forwarded, and the upstream's answer is returned to the client
   as-is.
4. During `DATA`, the gateway reads the header block, checks the `From:` addresses,
   then streams the message upstream. The upstream's final reply (for example
   `250 2.0.0 Ok: queued as …`) is logged and returned to the client.
5. If anything goes wrong mid-transfer, the upstream connection is dropped before
   the final dot, so a truncated message is never delivered. Network problems
   return `4xx`, so clients retry instead of bouncing.

## Operations

- **Backup**: the whole state is `/data/gateway.db` (SQLite in WAL mode). Copy it
  with `sqlite3 gateway.db ".backup out.db"`, or stop the container and copy the
  volume.
- **Health**: the image's `HEALTHCHECK` runs `/smtp-gateway healthcheck`, which calls
  `/healthz` on the panel (or checks the SMTP port when the panel is off).
- **Logs**: structured logs go to stderr. Failed logins are logged as
  `msg="auth failed" ip=…`, which you can feed to fail2ban or CrowdSec.
- **Resources**: the compose file sets `mem_limit: 64m`, `cpus: 0.5`,
  `GOMEMLIMIT=48MiB`, a read-only root filesystem, all capabilities dropped and
  `no-new-privileges`.

## Development

```sh
go test -race ./...                 # end-to-end tests with a fake upstream and a mock OIDC provider
go run ./cmd/smtp-gateway           # reads the same env vars
```
