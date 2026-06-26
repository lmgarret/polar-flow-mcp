# Security

polar-flow-mcp drives an **unofficial reverse-engineered API** with
session-cookie auth. This page documents the threat model, what is and
isn't protected, and operational practices to follow.

> **This is not the official Polar AccessLink API.** Use a Polar test
> account when possible. No warranty.

## Threat model

### Trusted

- The host running the server (process memory, the cookie-jar file).
- The operator's environment file (`.env`).
- Polar's TLS stack (`flow.polar.com`, `auth.polar.com`).

### Untrusted

- Anyone with access to the host filesystem outside of the process owner.
- Anyone who can read the binary's environment (other users on a shared
  host, processes that can inspect `/proc/<pid>/environ`).
- The network path to `*.polar.com` (mitigated by TLS).

### Out of scope

- MFA / captcha — the test accounts used during development have neither.
  An MFA-enabled account will fail the headless login chain; document this
  to your users.
- Polar's own session security — we treat `FLOW_SESSION` as an opaque
  bearer token issued by Polar.

## What's protected

- **The Polar password never touches disk.** It lives only in the process
  environment + memory, and is used only to perform the initial login.
- **The cookie-jar file is chmod 0600.** Only the running user can read or
  write it. We write atomically (`.tmp` then rename) to avoid partial state.
- **`/healthz`** intentionally returns no information about session state —
  it does not let an unauthenticated observer infer whether the server is
  logged in.

## What's *not* protected

- **The cookie jar is plaintext.** It contains `FLOW_SESSION` (JWT) and
  `remember-me`. Either of these is a bearer credential equivalent to a
  short-term password for the Polar account. Treat the file like an SSH
  key: protect it with filesystem permissions and back it up only
  encrypted.
- **The `.env` file is whatever you make it.** It contains the Polar
  password in plaintext by design. Use restrictive permissions and avoid
  committing it.
- **The HTTP transport has no auth *by default*.** With `OAUTH_PUBLIC_URL` unset,
  bind to localhost or put it behind a trusted-network barrier (Tailscale, VPN).
  A warning is logged at startup if `BIND_ADDRESS` is non-localhost and OAuth is
  off. To expose it publicly, enable the OAuth Authorization Server (below).

## Exposing publicly: app-as-Authorization-Server

To reach the server from **Claude.ai web/mobile and Claude Code**, enable inbound
OAuth by setting `OAUTH_PUBLIC_URL` (+ `OAUTH_ALLOWED_EMAIL` +
`OAUTH_TRUSTED_PROXIES`). The server then *becomes its own OAuth 2.1
Authorization Server*:

- Serves RFC 9728 / RFC 8414 discovery metadata pointing clients back at itself.
- Implements Dynamic Client Registration (RFC 7591), so Claude self-registers —
  nothing to pre-create or paste.
- Delegates browser login on `/mcp/oauth/authorize` to a forward-auth proxy
  (Authelia), reads the authenticated email from a trusted header, and checks the
  email allowlist before issuing a PKCE-bound code.
- Returns `401` + `WWW-Authenticate` to unauthenticated `/mcp` callers and
  validates every Bearer token **locally** (EdDSA JWT it signed).

The full Caddy + Authelia + Claude.ai walkthrough — including why forward-auth
goes on `/authorize` only — is in
[Exposing Securely](deployment/exposing-securely.md).

### The controls that matter

- **`OAUTH_TRUSTED_PROXIES`** — the linchpin. The forward-auth identity header is
  honoured only from these peers, so a too-broad value lets a network neighbour
  spoof an identity. Scope it to your proxy/container network.
- **`OAUTH_ALLOWED_EMAIL`** — the allowlist of who may consent; checked again on
  every `/mcp` call and refresh, so removing an email cuts access within the
  access-token TTL.
- **Redirect URIs are restricted** to the Claude callbacks + loopback even though
  registration is open, so a rogue client can't point authorization codes at an
  attacker URL.
- **`OAUTH_ALLOWED_ORIGINS`** — opt-in DNS-rebinding defence (no-Origin requests
  are always allowed, so it never breaks Claude's server-to-server calls).
- **Revocation** is coarse: short `OAUTH_ACCESS_TTL_MINUTES`, or rotate the
  signing key (delete `OAUTH_SIGNING_KEY_PATH` + restart) to invalidate every
  outstanding token at once.

## What the server does on every request

1. The ogen-generated client builds the HTTP request.
2. A custom `http.RoundTripper` injects:
   - `X-Requested-With: XMLHttpRequest` on every `/api/*` mutation (Play's
     CSRF filter requires this).
   - A browser-ish `User-Agent` (Polar serves different responses to
     non-browser UAs on adjacent endpoints).
   - The current `FLOW_SESSION` cookie via Go's `net/http/cookiejar`.
3. If Polar responds `401 {"error":"NotAuthenticated"}`, the wrapper runs
   the 3-hop silent refresh (`/flowSso/login → /oauth/authorize?continue →
   /flowSso/redirect`). On success it replaces the stale `FLOW_SESSION`
   cookie on the original request and retries once. On failure it falls
   back to a full email/password login.
4. The updated cookie jar is persisted to disk best-effort (failures are
   logged but don't fail the API call).

## What happens if the cookie jar leaks

An attacker with the jar can act as the Polar account until either:

- The user changes their Polar password (invalidates `session_id` /
  `remember-me` on `auth.polar.com`).
- The user logs out everywhere from Polar's UI.
- Polar's `remember-me` cookie naturally expires (~14 days from last use).

There is no way to revoke a single `FLOW_SESSION` short of those actions —
this is Polar's design, not ours. The mitigation is to keep the jar file
strictly readable by the running user and avoid keeping backups.

## What happens if `.env` leaks

The attacker has full Polar account access — they can change the password,
which then locks the legitimate user out and gives the attacker control of
all health data and settings. This is **strictly worse** than a cookie-jar
leak. Protect `.env` accordingly.

## Recommended deployment posture

- Run as a non-root user.
- Mount the cookie jar on a volume that the host's other users / containers
  can't read.
- For local/private use, bind the HTTP transport to `127.0.0.1` and consume via
  SSH tunnel or Tailscale.
- For public use (Claude.ai), enable the OAuth Authorization Server and front it
  with a TLS reverse proxy — see [Exposing Securely](deployment/exposing-securely.md).
- Use a Polar account dedicated to MCP use, not your main one.

## Reporting issues

Security issues: open a private security advisory via the GitHub Security
tab on the repo. Do not file public issues for live credentials or session
bypasses.
