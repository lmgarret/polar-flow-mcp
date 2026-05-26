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
- **The HTTP transport has no auth.** Bind to localhost or put it behind a
  trusted-network barrier (Tailscale, VPN, reverse proxy with auth). A
  warning is logged at startup if `BIND_ADDRESS` is non-localhost.

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
- Bind the HTTP transport to `127.0.0.1` and consume via SSH tunnel,
  Tailscale, or a Unix-socket proxy — not a public reverse proxy.
- Use a Polar account dedicated to MCP use, not your main one.

## Reporting issues

Security issues: open a private security advisory via the GitHub Security
tab on the repo. Do not file public issues for live credentials or session
bypasses.
