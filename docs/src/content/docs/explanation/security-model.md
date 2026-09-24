---
title: Security model
description: The threat model behind polar-flow-mcp — what is and isn't protected, what happens if credentials leak, and the recommended deployment posture.
sidebar:
  order: 3
---

polar-flow-mcp drives an **unofficial, reverse-engineered API** with
session-cookie auth. This page documents the threat model, what is and isn't
protected, and the operational practices to follow.

:::caution[This is not the official Polar AccessLink API]
Use a Polar test account when possible. No warranty.
:::

## Threat model

```d2
direction: down

trusted: Trusted — the host you run on {
  env: .env (POLAR_PASSWORD, plaintext)
  proc: polar-flow-mcp (password in memory only)
  jar: cookie jar — chmod 600 (FLOW_SESSION + remember-me)
}

untrusted: Untrusted {
  others: other users / processes on the host
  net: network path to *.polar.com
}

polar: Polar (flow.polar.com / auth.polar.com)

trusted.env -> trusted.proc: read at startup
trusted.proc -> trusted.jar: persist session
trusted.proc -> polar: HTTPS (TLS)
untrusted.net -> polar: mitigated by TLS {
  style.stroke-dash: 3
}
untrusted.others -> trusted.jar: blocked by file perms {
  style.stroke-dash: 3
}
```

### Trusted

- The host running the server (process memory, the cookie-jar file).
- The operator's environment file (`.env`).
- Polar's TLS stack (`flow.polar.com`, `auth.polar.com`).

### Untrusted

- Anyone with access to the host filesystem outside of the process owner.
- Anyone who can read the binary's environment (other users on a shared host,
  processes that can inspect `/proc/<pid>/environ`).
- The network path to `*.polar.com` (mitigated by TLS).

### Out of scope

- MFA / captcha — the test accounts used during development have neither. An
  MFA-enabled account will fail the headless login chain; document this to your
  users.
- Polar's own session security — we treat `FLOW_SESSION` as an opaque bearer
  token issued by Polar.

## What's protected

- **The Polar password never touches disk.** It lives only in the process
  environment + memory, and is used only to perform the initial login.
- **The cookie-jar file is chmod 0600.** Only the running user can read or write
  it. We write atomically (`.tmp` then rename) to avoid partial state.
- **`/healthz`** intentionally returns no information about session state — it
  does not let an unauthenticated observer infer whether the server is logged in.

## What's *not* protected

- **The cookie jar is plaintext.** It contains `FLOW_SESSION` (JWT) and
  `remember-me`. Either is a bearer credential equivalent to a short-term
  password for the Polar account. Treat the file like an SSH key: protect it with
  filesystem permissions and back it up only encrypted.
- **The `.env` file is whatever you make it.** It contains the Polar password in
  plaintext by design. Use restrictive permissions and avoid committing it.
- **The HTTP transport has no auth *by default*.** With neither `MCP_API_KEY` nor
  `OAUTH_PUBLIC_URL` set,
  bind to localhost or put it behind a trusted-network barrier (Tailscale, VPN).
  A warning is logged at startup if `BIND_ADDRESS` is non-localhost and inbound
  auth is off. To expose it publicly, enable either the static API key or the
  OAuth Authorization Server (below).

## Exposing publicly: static API key

The low-ceremony option. Set `MCP_API_KEY` to a secret of at least 24 characters
and every `/mcp` request must present it as `Authorization: Bearer <key>`. The
server stores only the SHA-256 of the configured key and compares digests in
constant time, so neither the value nor its length leaks through timing; the key
never appears in a log line or a response body.

What this posture does **not** give you, and the reason OAuth still exists here:

- **No identity.** Every caller is the same anonymous holder of one secret. Logs
  cannot attribute a request to a person.
- **No expiry, coarse revocation.** The key lives until you change it, and
  changing it means a restart that breaks every existing client at once. OAuth's
  access tokens expire on their own and can be cut off by rotating the signing key.
- **Full compromise on leak.** A key pasted into the wrong shell history, CI log,
  or client config grants everything the MCP surface exposes — including the write
  tools. OAuth confines a leaked access token to its TTL.
- **Not a Claude.ai connector.** Claude.ai's connector UI drives an OAuth flow;
  it has nowhere to put a static header. This posture is for Claude Code and
  scripted clients.

It is a reasonable trade when you are the only caller, the deployment is behind
TLS, and standing up Authelia is more machinery than the situation warrants.
`MCP_API_KEY` and `OAUTH_PUBLIC_URL` are mutually exclusive: running both would
let the weaker mechanism answer for the stronger one, so the server refuses to
start.

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

The full walkthrough — including why forward-auth goes on `/authorize` only — is
in [Expose the server securely](/guides/expose-securely/).

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

## What happens if the cookie jar leaks

An attacker with the jar can act as the Polar account until either:

- The user changes their Polar password (invalidates `session_id` / `remember-me`
  on `auth.polar.com`).
- The user logs out everywhere from Polar's UI.
- Polar's `remember-me` cookie naturally expires (~14 days from last use).

There is no way to revoke a single `FLOW_SESSION` short of those actions — this
is Polar's design, not ours. The mitigation is to keep the jar file strictly
readable by the running user and avoid keeping backups.

## What happens if `.env` leaks

The attacker has full Polar account access — they can change the password, which
then locks the legitimate user out and gives the attacker control of all health
data and settings. This is **strictly worse** than a cookie-jar leak. Protect
`.env` accordingly.

## Recommended deployment posture

- Run as a non-root user.
- Mount the cookie jar on a volume that the host's other users / containers can't
  read.
- For local/private use, bind the HTTP transport to `127.0.0.1` and consume via
  SSH tunnel or Tailscale.
- For public use, front the server with a TLS reverse proxy and enable inbound
  auth: `MCP_API_KEY` when you are the only caller, or the OAuth Authorization
  Server for Claude.ai and multi-user access — see
  [Expose the server securely](/guides/expose-securely/).
- Use a Polar account dedicated to MCP use, not your main one.

## Reporting issues

Security issues: open a private security advisory via the GitHub Security tab on
the repo. Do not file public issues for live credentials or session bypasses.
