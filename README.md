# SMTP Gateway

[![CI](https://github.com/angwdev/smtp-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/angwdev/smtp-gateway/actions/workflows/ci.yml)

Image: `ghcr.io/angwdev/smtp-gateway` (linux/amd64, arm64, arm/v7)

A small, self-hosted, **credential-translating SMTP relay**. Every client, app or
server gets its own SMTP login, which you create in an SSO-protected web panel. The
gateway checks that login, applies per-login policy, and relays the mail to your real
mail server using **one** upstream credential.

```mermaid
flowchart LR
    subgraph clients["Apps and devices, one login each"]
        nc["Nextcloud<br/>login: nextcloud"]
        gr["Grafana<br/>login: grafana"]
        bk["Backups<br/>login: backups"]
    end
    gw["SMTP Gateway<br/>checks login, sender and rate<br/>logs every message"]
    up["Your mail server<br/>mail.example.com:587<br/>one account: relay@example.com"]
    db[("SQLite<br/>logins, message log,<br/>audit log")]
    admin(["Administrator"])
    script(["Scripts"])

    clients -->|SMTP, 587 STARTTLS or 465 TLS| gw
    gw -->|SMTP with the one upstream login| up
    gw --- db
    admin -->|Panel, OIDC sign-in| gw
    script -->|JSON API, bearer token| gw
```

- One static Go binary in a `scratch` image (about 11 MB compressed). It typically uses
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
- **Richer logs**: the message log keeps the subject and the full header block
  (never the body), and the audit log records who did what, when, from where, and
  (for panel changes) a before/after diff. SMTP sign-in failures and lockouts, and
  failed API tokens, are logged too.
- **HTTP API**: everything the panel can do is also available as a versioned JSON
  API under `/api/v1`, authenticated with a bearer token you create in the panel,
  scoped to read-only or read/write. Every call is audited.
- **Panel**: OIDC (authorization code + PKCE) with any provider (Authentik,
  Authelia, Keycloak, Zitadel, Pocket ID, Kanidm, Google, Entra ID…). You can limit
  access by e-mail, by domain or by group. It has CSRF protection, a strict CSP and
  is a small React app with no third-party network calls. It shows the message log
  (searchable, retention configurable), the audit log, API token management, the
  certificate's status with a "Renew now" button, and a button that tests the
  upstream connection. Light by default, with a dark theme too.

## Screenshots

| Messages, phone, light theme | Audit log, desktop, light theme |
|---|---|
| ![Messages list on a phone-sized screen, light theme](docs/img/messages-phone-light.png) | ![Audit log on a desktop-sized screen, light theme](docs/img/audit-desktop-light.png) |

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
3. **Sign in**, open **Logins**, click **New login** and copy the password shown.
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
- **Panel**: the Certificate section on Overview shows the issuer, expiry, renewal schedule and
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
| `WEB_TRUSTED_PROXIES` | | Reverse proxies whose `X-Forwarded-For` is trusted (IPs or CIDRs); see "Behind a reverse proxy" below |
| `LOG_RETENTION_DAYS` | `30` | For the message and audit logs |
| `LOG_MESSAGE_CONTENT` | `headers` | `headers`, `subject` or `none`; see "What is logged" below |
| `LOG_LEVEL` | `info` | |

If both allow-lists are empty, anyone your IdP authenticates for this client can
sign in. A warning is logged at startup in that case.

## How a message flows

```mermaid
sequenceDiagram
    autonumber
    participant C as App
    participant G as SMTP Gateway
    participant DB as SQLite
    participant U as Mail server
    C->>G: STARTTLS or implicit TLS, then AUTH PLAIN or LOGIN
    G->>DB: Find the login, compare the SHA-256 hash
    G-->>C: 235 accepted, or 535 (counts toward the IP lockout)
    C->>G: MAIL FROM
    G->>DB: Re-read the login: enabled, sender allowed, hourly rate
    G->>U: Connect or reuse, AUTH with the upstream login, MAIL FROM
    U-->>G: 250
    G-->>C: 250
    loop Every recipient
        C->>G: RCPT TO
        G->>U: RCPT TO
        U-->>G: Reply
        G-->>C: Same reply, unchanged
    end
    C->>G: DATA
    G->>G: Read the header block, check From: against the sender policy
    G->>U: Stream the message, nothing is written to disk
    U-->>G: 250 Ok: queued as 4F1A
    G->>DB: Log envelope, subject and headers in one transaction
    G-->>C: The upstream's final reply
```

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

## HTTP API

Everything the panel's UI can do is also available as a versioned JSON API under
`/api/v1`. The OpenAPI document is served at `/api/v1/openapi.yaml` (also in the
repo at [`api/openapi.yaml`](api/openapi.yaml)).

1. **Create a token** in the panel: *API tokens* → *New token*, choose a name and a
   scope, and copy the value shown — it starts with `sgw_` and is shown exactly
   once.
2. **Call the API** with it as a bearer token:

   ```sh
   curl -H "Authorization: Bearer sgw_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
        https://smtp-panel.example.com/api/v1/messages?limit=5
   ```

**How a request is checked**, in order:

```mermaid
flowchart TD
    req["Request to /api/v1"] --> co{"Unsafe cross-origin<br/>browser request?"}
    co -->|yes| e403a["403"]
    co -->|no| rl{"More than 20 per second<br/>from this IP?"}
    rl -->|yes| e429["429 RATE_LIMITED"]
    rl -->|no| bearer{"Authorization:<br/>Bearer sgw_...?"}
    bearer -->|yes| tok{"Token known, not expired,<br/>not revoked?"}
    tok -->|no| e401t["401 and token.auth_failed<br/>repeated failures from one IP: 429"]
    tok -->|yes| scope
    bearer -->|no| cookie{"Session cookie matches<br/>a server-side session?"}
    cookie -->|no| e401["401 UNAUTHORIZED"]
    cookie -->|yes| scope{"Scope allows<br/>this route?"}
    scope -->|no| e403["403 FORBIDDEN"]
    scope -->|yes| handler["Handler"]
    handler --> audit["Audit row: the specific action,<br/>or api.call for any other token request"]
```

**Scopes:**

| Scope | Can | Cannot |
|---|---|---|
| `read` | `GET` everything under `/api/v1` (overview, credentials, messages, audit) | Create, change or delete anything; manage tokens |
| `write` | Everything `read` can, plus create/update/rotate/delete credentials, test the upstream, renew the certificate | Create or revoke API tokens |
| session (signed-in person) | Everything `write` can, plus create and revoke API tokens | — |

A token can never manage other tokens, even with `write` scope — minting and
revoking tokens needs the panel session itself, so a leaked token can't be used to
mint itself a replacement.

**Errors** are a JSON body with a machine-readable `code`:

```json
{"code":"VALIDATION_ERROR","message":"username is required","details":{"field":"username"}}
```

`code` is one of `VALIDATION_ERROR` (400), `UNAUTHORIZED` (401), `FORBIDDEN` (403),
`NOT_FOUND` (404), `METHOD_NOT_ALLOWED` (405), `CONFLICT` (409), `RATE_LIMITED`
(429) or `INTERNAL_ERROR` (500).

## What is logged

Every message's log entry has the envelope (sender, recipients, size, the
credential and remote IP used), the result (`sent`, `rejected` or `failed`) and
detail, the subject, and — depending on `LOG_MESSAGE_CONTENT` — the headers:

| `LOG_MESSAGE_CONTENT` | Kept |
|---|---|
| `headers` (default) | Subject and the full header block, capped at 16 KiB |
| `subject` | Subject only |
| `none` | Neither subject nor headers |

**The message body is never stored**, whatever this setting is. Message and audit
rows are kept for `LOG_RETENTION_DAYS` (default 30) and pruned hourly after that.

## Audit log

Every sign-in, panel change, API token action and SMTP/API authentication failure
is recorded with who did it, when, the result, the IP and user agent, and — for
panel changes — a before/after diff. The actions are:

| Action | Recorded when |
|---|---|
| `session.login` | A person signs in through OIDC |
| `session.login_denied` | OIDC succeeded but the account isn't in `OIDC_ALLOWED_EMAILS`/`OIDC_ALLOWED_GROUPS` |
| `session.logout` | A person signs out |
| `credential.create` | A login is created |
| `credential.update` | A login's description, sender policy or rate limit changes |
| `credential.enable` | A login is turned back on |
| `credential.disable` | A login is turned off |
| `credential.rotate` | A login's password is replaced |
| `credential.delete` | A login is deleted |
| `token.create` | An API token is created |
| `token.revoke` | An API token is revoked |
| `token.auth_failed` | A request used a bearer token that isn't valid, has expired or was revoked |
| `certificate.renew` | A manual "Renew now" is requested |
| `upstream.test` | The "Test connection" button is used |
| `smtp.auth_failed` | An SMTP client's AUTH attempt is rejected |
| `smtp.lockout` | An IP is locked out after too many failed SMTP AUTH attempts (`SMTP_AUTH_FAIL_LIMIT`/`SMTP_AUTH_FAIL_WINDOW`) |
| `api.call` | Any other successful or failed API call made with a bearer token (so every token use leaves a trace even when it didn't trigger one of the specific actions above) |

Sign-in failures over both SMTP and the API are rate-limited in the log itself (a
token bucket per source), so a brute-force attempt produces a bounded number of
rows, not one per attempt; the count of rows dropped by that limit is logged once
a minute.

## Behind a reverse proxy

By default the gateway treats the direct TCP peer as the client's address for
rate limiting and audit `ip` values. Behind a reverse proxy that peer is the
proxy itself, which is useless for both. Set `WEB_TRUSTED_PROXIES` to the proxy's
address or subnet (comma separated, IPs or CIDRs) and the gateway reads
`X-Forwarded-For` from it instead — for example, behind Traefik on its default
Docker network:

```env
WEB_TRUSTED_PROXIES=172.18.0.0/16
```

Left empty (the default), `X-Forwarded-For` is never read, so a client can't spoof
its IP by sending that header directly — the socket address is always used
instead. Only widen this to networks you actually trust.

## Panel sign-in

```mermaid
sequenceDiagram
    autonumber
    actor B as Browser
    participant G as SMTP Gateway
    participant I as Identity provider
    participant DB as SQLite
    B->>G: GET /login
    G-->>B: Redirect to the provider with state, nonce and a PKCE challenge
    B->>I: Sign in
    I-->>B: Redirect to /auth/callback with a code
    B->>G: GET /auth/callback
    G->>I: Exchange the code with the PKCE verifier
    I-->>G: ID token: e-mail, email_verified, groups
    G->>G: Check OIDC_ALLOWED_EMAILS and OIDC_ALLOWED_GROUPS
    alt Allowed
        G->>DB: Store a session (only the SHA-256 of its id) and audit session.login
        G-->>B: Session cookie, open the panel
    else Not allowed
        G->>DB: Audit session.login_denied
        G-->>B: Page explaining why
    end
```

Signing out deletes the session row, so a copied cookie stops working at once.
The cookie is `__Host-sg_session` (`sg_session` when `PUBLIC_URL` is plain
http), `HttpOnly`, `Secure` and `SameSite=Lax`.

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

The panel self-hosts [Atkinson Hyperlegible Next and Atkinson Hyperlegible
Mono](https://github.com/ApplePieCorp/atkinson-hyperlegible) (SIL Open Font
License 1.1, via `@fontsource-variable/*`) and icons from
[Material Icons](https://mui.com/material-ui/material-icons/) (`@mui/icons-material`,
Apache 2.0). Nothing is loaded from third-party servers.

## Upgrading from 0.1

- **The database migrates itself on first start.** No manual step is needed:
  Goose migrations run before the server starts accepting connections.
- **Back up `gateway.db` first anyway** (see "Operations" above) — the usual
  advice for any upgrade that touches the schema.
- **Existing panel sessions end.** Sessions are now tracked server-side (so
  logout actually revokes them), which the old cookie format can't do. Everyone
  signs in again once; no action is needed beyond that.
- **The panel's URL paths changed** (it is now a single-page app with client-side
  routing instead of server-rendered pages), but old bookmarks to the previous
  paths redirect to their new equivalents.
- **SMTP clients need no change.** The listener addresses, ports, AUTH mechanisms
  and credentials are exactly as before; only the panel and its data model changed.

## Development

```sh
tools/go test -race ./...           # same as `make test`, but a single invocation
make check                          # vet, lint, tests with coverage, per-layer gates
make ui                             # build the React panel and embed it for `go run`
go run ./cmd/smtp-gateway            # reads the same env vars
cd ui && bun run shots              # screenshot suite (phone/tablet/desktop, light/dark)
# The ACME test runs only when Pebble is installed:
go install github.com/letsencrypt/pebble/v2/cmd/pebble@v2.10.1
go install github.com/letsencrypt/pebble/v2/cmd/pebble-challtestsrv@v2.10.1
```

### Code layout

The Go code follows a layered (domain-driven) layout. Each arrow is an allowed
import; the `depguard` linter rejects any other direction, and only
`internal/app` wires everything together.

```mermaid
flowchart TD
    main["cmd/smtp-gateway<br/>subcommands"] --> app["internal/app<br/>composition root"]
    app --> ifaces & appl & infra & dom
    ifaces["internal/interfaces<br/>SMTP server, Gin HTTP API,<br/>SPA serving, limiters"] --> appl["internal/application<br/>use cases, audit recorder,<br/>maintenance loop"]
    appl --> dom["internal/domain<br/>entities, rules, repository<br/>interfaces (standard library only)"]
    infra["internal/infrastructure<br/>GORM repositories, Goose migrations,<br/>upstream client, TLS, ACME"] --> dom
```

The panel is a React app in `ui/` (Vite, MUI and Tailwind), built into
`internal/interfaces/http/ui/dist` and embedded in the binary.

### Data model

SQLite in WAL mode, with one writer connection and a small read pool. Goose owns
the schema (`migrations/`).

```mermaid
erDiagram
    credentials |o--o{ messages : "sent with (kept if the login is deleted)"
    messages ||--o| message_headers : "header block"
    credentials {
        int id PK
        text username UK
        text secret_hash "SHA-256"
        text allowed_senders
        int rate_per_hour
        int enabled
        int sent_count
    }
    messages {
        int id PK
        int credential_id FK
        text mail_from
        text rcpt_to
        text status "sent, rejected or failed"
        text subject
    }
    message_headers {
        int message_id PK
        text raw "up to 16 KiB"
        int truncated
    }
    audit_events {
        int id PK
        text actor_type "user, token, cli, system or smtp"
        text action
        text target_label
        text result "ok, denied or error"
        text ip
        text changes "JSON before and after"
    }
    api_tokens {
        int id PK
        text name UK
        text token_hash UK "SHA-256"
        text scope "read or write"
        int expires_at
        int revoked_at
    }
    sessions {
        text id_hash PK "SHA-256"
        text email
        int expires_at
    }
    settings {
        text key PK
        text value
    }
```

### Toolchain

Go runs through `tools/go`, a thin wrapper around a `golang:1.27` container (the
build host needs no local Go install); set `SMTPGW_LOCAL_GO=1` to use a local
`go` on `PATH` instead. UI commands run with **bun**, not npm (`bun install
--frozen-lockfile`, `bun run <script>`; see `ui/package.json`).

Releases: push a tag like `v0.2.0`. CI runs the Go and UI gates, then builds and
publishes the multi-arch image to GHCR.
