# SMTP Gateway

[![CI](https://github.com/angwdev/smtp-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/angwdev/smtp-gateway/actions/workflows/ci.yml)

Image: `ghcr.io/angwdev/smtp-gateway` (linux/amd64, arm64, arm/v7)

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

- One static Go binary in a `scratch` image (about 8 MB compressed). It typically uses
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
- **TLS**: STARTTLS on 587 and implicit TLS (SMTPS) on 465. The certificate can be
  **issued automatically with ACME** ([lego](https://go-acme.github.io/lego/), DNS-01
  via Cloudflare or AWS Route 53), come from your own files (re-read when renewed),
  or be self-signed. AUTH without TLS is refused unless you turn it on explicitly.
- **AUTH PLAIN and LOGIN** for clients. Upstream can use STARTTLS, implicit TLS or
  plain, with PLAIN or LOGIN auth. You can optionally force a fixed envelope sender
  upstream (`UPSTREAM_ENVELOPE_FROM`).
- **Panel**: OIDC (authorization code + PKCE) with any provider (Authentik,
  Authelia, Keycloak, Zitadel, Pocket ID, Kanidm, Google, Entra ID…). You can limit
  access by e-mail, by domain or by group. It has CSRF protection, a strict CSP and
  no JavaScript framework. It shows the message log (searchable, retention
  configurable), an audit log of every sign-in and change, the certificate's status
  with a "Renew now" button, and a button that tests the upstream connection. It is
  dark by default, with light and system themes.

## Quick start

```sh
curl -fsSLO https://raw.githubusercontent.com/angwdev/smtp-gateway/main/docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/angwdev/smtp-gateway/main/.env.example -o .env
$EDITOR .env            # fill in UPSTREAM_*, PUBLIC_URL and OIDC_*
docker compose up -d
docker compose logs -f
```

Image tags: `latest` (and `edge`) follow `main`. `X.Y.Z` / `X.Y` are
published for release tags, and `sha-<commit>` for every build. Each image has a GitHub build-provenance
attestation (`gh attestation verify oci://ghcr.io/angwdev/smtp-gateway:latest -R angwdev/smtp-gateway`).

1. **Register an OIDC client** in your identity provider:
   - Redirect URI: `${PUBLIC_URL}/auth/callback`
   - Post-logout redirect URI (optional): `${PUBLIC_URL}/`
   - Type: confidential (client secret). Scopes: `openid email profile`, plus
     `groups` if you use `OIDC_ALLOWED_GROUPS` and your IdP needs that scope.

   **Authentik:** under *Applications → Providers*, create an *OAuth2/OpenID
   Provider* (client type *Confidential*, redirect URI as above, *Strict*
   matching). Then create an *Application* that uses this provider. From the
   provider's page, copy:

   | `.env` | Authentik field |
   |---|---|
   | `OIDC_ISSUER` | *OpenID Configuration Issuer*, e.g. `https://auth.example.com/application/o/smtp-gateway/` |
   | `OIDC_CLIENT_ID` | *Client ID*: the generated random string, **not** the application slug |
   | `OIDC_CLIENT_SECRET` | *Client Secret* |

   To choose who can sign in, prefer **`OIDC_ALLOWED_GROUPS`**, for example
   `smtp-admins` (Authentik sends a `groups` claim by default), or bind a group or
   policy to the Application. `OIDC_ALLOWED_EMAILS` only matches addresses the IdP
   marks as verified, and since Authentik 2025.10 the default mapping sends
   `email_verified: false`. To use e-mails there, either add a
   [verified-email mapping](https://docs.goauthentik.io/add-secure-apps/providers/oauth2/verify-email-address/)
   to the provider, or set `OIDC_ALLOW_UNVERIFIED_EMAIL=true`. Only do the latter
   if users can't change their own e-mail address in Authentik.
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

### Automatic certificates (ACME)

Set `TLS_SOURCE=acme` and the gateway gets and renews its own certificate from
Let's Encrypt (or ZeroSSL, Google Trust Services, any ACME CA) using
[lego](https://go-acme.github.io/lego/). It uses the **DNS-01** challenge, so no
inbound port 80 or 443 is needed, which suits a mail host. Two DNS providers are
built in:

```env
TLS_SOURCE=acme
ACME_ACCEPT_TOS=true
ACME_EMAIL=you@example.com
ACME_DOMAINS=smtp.example.com           # defaults to SMTP_HOSTNAME

# Cloudflare: API token with Zone:Read and DNS:Edit for the zone
ACME_DNS_PROVIDER=cloudflare
CLOUDFLARE_DNS_API_TOKEN=...

# or AWS Route 53 (env keys, shared config or an IAM role)
# ACME_DNS_PROVIDER=route53
# AWS_ACCESS_KEY_ID=...
# AWS_SECRET_ACCESS_KEY=...
# AWS_REGION=us-east-1
# AWS_HOSTED_ZONE_ID=...                # optional
```

How it behaves:

- **Never blocks mail.** Until the first certificate is issued, and whenever the CA
  or DNS provider is unreachable, the gateway keeps serving its current (or a
  self-signed) certificate. Failed attempts back off from 5 minutes to 6 hours,
  well inside Let's Encrypt's rate limits.
- **Renewal** follows the CA's ACME Renewal Information (RFC 9773) when offered.
  Otherwise it renews once a third of the lifetime remains, which works for both
  90-day and shorter-lived certificates. Renewals tell the CA which certificate
  they replace.
- **State** lives in `/data/acme/`: the account key (per CA) and the current
  certificate. Changing `ACME_DOMAINS`, `ACME_CA` or `ACME_KEY_TYPE` triggers a new
  order on the next start.
- **Panel**: the Certificate card shows the issuer, expiry, renewal schedule and
  the last error, and has a **Renew now** button.
- **Debugging**: with `LOG_LEVEL=debug`, every failed propagation check is logged
  with its reason. Set `ACME_DNS_SKIP_AUTHORITATIVE=true` if your network blocks
  outbound DNS to the zone's authoritative servers. `ACME_DNS_RESOLVERS` picks the
  recursive resolvers used for the check.

Try `ACME_CA=letsencrypt-staging` first if you're experimenting; staging has much
higher rate limits (its certificates aren't trusted by clients).

### Using your own certificate files

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
| `TLS_SOURCE` | `self-signed` (`file` if `TLS_CERT_FILE` is set) | `self-signed`, `file` or `acme` |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | | For `TLS_SOURCE=file` |
| `ACME_ACCEPT_TOS` | `false` | Must be `true` for `acme` |
| `ACME_EMAIL` | *(required for acme)* | Account contact at the CA |
| `ACME_DOMAINS` | `SMTP_HOSTNAME` | Comma-separated names on the certificate |
| `ACME_DNS_PROVIDER` | *(required for acme)* | `cloudflare` or `route53` (credentials via `CLOUDFLARE_*` / `AWS_*`) |
| `ACME_CA` | `letsencrypt` | `letsencrypt`, `letsencrypt-staging`, `zerossl`, `googletrust` or a directory URL |
| `ACME_KEY_TYPE` | `ec256` | `ec256`, `ec384`, `rsa2048`, `rsa3072`, `rsa4096` |
| `ACME_EAB_KID` / `ACME_EAB_HMAC` | | External account binding (ZeroSSL, Google) |
| `ACME_DNS_RESOLVERS` | system | Recursive resolvers for the propagation check |
| `ACME_DNS_SKIP_AUTHORITATIVE` | `false` | Skip checking the zone's authoritative servers |
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
| `OIDC_ALLOW_UNVERIFIED_EMAIL` | `false` | Also accept listed e-mails sent with `email_verified: false`; only safe if users can't edit their e-mail |
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

## Troubleshooting sign-in

At startup the gateway logs the exact values it will send to your identity
provider:

```
level=INFO msg="sso: identity provider found; register redirect_uri exactly as logged" issuer=… client_id=… redirect_uri=https://smtp-panel.example.com/auth/callback
```

| What you see | Cause |
|---|---|
| Authentik: *"Client ID Error: The client identifier (client_id) is missing or invalid"* | `OIDC_CLIENT_ID` doesn't match the provider's *Client ID*. Usually the application slug was used instead, or the value has a typo. Copy it from the provider page and restart. |
| *"Redirect URI Error"* | The provider's redirect URI isn't exactly the logged `redirect_uri`. Check scheme, host and trailing path, and that `PUBLIC_URL` is the URL you open in the browser. |
| `sso: identity provider not reachable yet` | The gateway can't fetch `${OIDC_ISSUER}.well-known/openid-configuration` (wrong issuer, DNS, or TLS). Sign-in retries on each attempt. |
| *"… is in OIDC_ALLOWED_EMAILS, but your identity provider reports it as unverified"* | Authentik 2025.10+ sends `email_verified: false` by default. Use `OIDC_ALLOWED_GROUPS`, add a verified-email mapping in Authentik, or set `OIDC_ALLOW_UNVERIFIED_EMAIL=true`. |
| *"signed in as …, which is not in the allowed e-mails or groups"* | The account isn't listed. The container log (`sso: sign-in refused`) shows the e-mail, `email_verified` and groups the IdP sent. |

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

## Third-party assets

The panel embeds [IBM Plex Sans and Plex Mono](https://github.com/IBM/plex) (SIL Open
Font License 1.1, `internal/web/static/fonts/OFL.txt`) and icons from
[Phosphor](https://phosphoricons.com) (MIT, `internal/web/static/PHOSPHOR-LICENSE.txt`).
Nothing is loaded from third-party servers.

## Development

```sh
docker compose build                # after uncommenting `build: .` in docker-compose.yml
go test -race ./...                 # end-to-end tests with a fake upstream and a mock OIDC provider
# The ACME test runs only when Pebble is installed:
go install github.com/letsencrypt/pebble/v2/cmd/pebble@v2.10.1
go install github.com/letsencrypt/pebble/v2/cmd/pebble-challtestsrv@v2.10.1
go run ./cmd/smtp-gateway           # reads the same env vars
```

Releases: push a tag like `v0.2.0`. CI runs the tests, then builds and publishes
the multi-arch image to GHCR.
