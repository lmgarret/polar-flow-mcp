---
title: Expose the server securely
description: Expose polar-flow-mcp to the public internet as a Claude.ai custom connector, protected by OAuth 2.1 and locked to one person (Caddy + Authelia).
sidebar:
  order: 3
---

This guide exposes the server to the public internet so you can use it as a
**Claude.ai custom connector** (web + mobile, and Claude Code), protected by
OAuth 2.1 and locked to a single person.

The server **is its own OAuth 2.1 Authorization Server**. It implements Dynamic
Client Registration (RFC 7591), so Claude self-registers when you paste the URL
— there is **no client id/secret to create or paste**. Browser login on the
consent step is delegated to a **forward-auth proxy** (Authelia in front of
Caddy): the proxy authenticates *you* and passes your identity in a trusted
header, and an **email allowlist** decides who may connect. Access tokens are
short-lived EdDSA JWTs the server signs and validates itself — no database, no
introspection round-trip.

:::tip[Only using Claude Code?]
If Claude.ai's connector UI is not in the picture and you are the only caller,
the [static API key](/reference/environment-variables/#inbound-api-key-static-shared-secret)
(`MCP_API_KEY`) secures the deployment with one env var and no Authelia — just
put a TLS reverse proxy in front. This guide's OAuth path is what you want when
several people connect, or when Claude.ai must drive the login itself.
:::

:::note[Which Claude clients work]
- **Claude.ai web + mobile** — yes.
- **Claude Desktop** — yes (same remote-connector path).
- **Claude Code (CLI)** — **yes.** Claude Code requires Dynamic Client
  Registration, which this server provides, so the same public URL works.
:::

## Why not Authelia forward-auth on `/mcp`?

Forward-auth authenticates by redirecting a **browser** to a login portal and
setting a session cookie. Claude's MCP clients call `/mcp` **server-to-server** —
no browser, no cookie — so a forward-auth cookie handshake can never complete for
them. They speak OAuth 2.1 Bearer instead.

The trick: the **only** browser-driven step in the whole flow is `/authorize`
(the consent popup). So forward-auth goes **only** on `/authorize`, where a real
browser hits it. Everything else (`/mcp`, the token/registration endpoints, the
discovery documents) is called by Claude's backend and must **not** be
forward-auth'd — it is guarded by the access tokens this server issues.

## Topology

Forward-auth guards **only** `/mcp/oauth/authorize` (the one browser step);
everything else is called server-to-server by Claude and guarded by the tokens
this server issues.

```d2
shape: sequence_diagram

claude: Claude.ai / Claude Code
caddy: Caddy (TLS)
authelia: Authelia
server: polar-flow-mcp

claude -> server: 1. GET /mcp
server -> claude: 401 + WWW-Authenticate
claude -> server: 2. discover /.well-known/oauth-*
claude -> server: 3. POST /mcp/oauth/register (DCR)
claude -> caddy: 4. browser → /mcp/oauth/authorize
caddy -> authelia: forward-auth (this path only) {
  style.stroke-dash: 3
}
authelia -> caddy: Remote-Email
caddy -> server: authorize → PKCE code
claude -> server: 5. POST /mcp/oauth/token (PKCE)
server -> claude: access + refresh JWT
claude -> server: 6. POST /mcp  Bearer <jwt> → tools
```

## polar-flow-mcp — enable OAuth

Setting `OAUTH_PUBLIC_URL` turns auth on. `OAUTH_ALLOWED_EMAIL` and
`OAUTH_TRUSTED_PROXIES` are then mandatory (the server refuses to start without
them — see the
[environment variables reference](/reference/environment-variables/)).

```bash
OAUTH_PUBLIC_URL=https://polar.example.com    # public origin = OAuth issuer
OAUTH_ALLOWED_EMAIL=you@example.com           # the ONLY identity allowed to connect
OAUTH_TRUSTED_PROXIES=172.18.0.0/16           # the network your proxy connects FROM
OAUTH_SIGNING_KEY_PATH=/data/polar-oauth-key.json   # persist across restarts

# Optional hardening:
# OAUTH_ALLOWED_GROUPS=polar                   # also require an Authelia group
# OAUTH_ALLOWED_ORIGINS=https://claude.ai      # DNS-rebinding defence on /mcp
```

:::danger[OAUTH_TRUSTED_PROXIES is the linchpin]
The server reads the authenticated email from a forward-auth header
(`Remote-Email`). It only trusts that header when the request's peer IP is in
`OAUTH_TRUSTED_PROXIES` — i.e. it came from *your* Caddy. Set this to the
network Caddy connects from (the Docker bridge subnet, or the proxy host IP).
If it is too broad, anyone who can reach the container could spoof an identity.
This replaces the static-client secret as the thing to get right.
:::

The public URL is the issuer; the MCP resource is `OAUTH_PUBLIC_URL` + `/mcp`.

## Identity provider — Authelia (forward-auth only)

Authelia is used here purely to **log you in on `/authorize`** — there is **no
OIDC client to register**. You only need an access-control rule so the consent
path requires a full login, and (if your Authelia has more than one user) so only
you pass.

```yaml
# authelia configuration.yml
access_control:
  rules:
    # The consent endpoint must require a real (ideally 2FA) login.
    - domain: 'polar.example.com'
      resources:
        - '^/mcp/oauth/authorize'
      policy: 'two_factor'
      subject: 'user:you'          # restrict to YOU if Authelia is multi-user
```

Authelia must pass the authenticated email back to the proxy. Its
`/api/authz/forward-auth` response sets `Remote-User`, `Remote-Email`,
`Remote-Name`, `Remote-Groups`; this server reads `Remote-Email` by default
(override with `OAUTH_FORWARD_AUTH_EMAIL_HEADER`).

## Reverse proxy — Caddy

Caddy terminates TLS, forward-auths **only** `/mcp/oauth/authorize`, and proxies
everything to the container. The `copy_headers` line is what carries the
authenticated email to polar-flow-mcp.

```text
polar.example.com {
    # Forward-auth ONLY the browser consent endpoint.
    @authorize path /mcp/oauth/authorize
    forward_auth @authorize authelia:9091 {
        uri /api/authz/forward-auth
        copy_headers Remote-User Remote-Email Remote-Name Remote-Groups
    }

    # Everything (including /authorize after the check) goes to the app.
    reverse_proxy polar-flow-mcp:8080
}
```

What any other proxy must replicate:

- **Forward-auth on `/mcp/oauth/authorize` only.** Never on `/mcp`,
  `/mcp/oauth/token`, `/mcp/oauth/register`, or `/.well-known/*` — those are
  server-to-server and would break.
- **Copy the identity header** (`Remote-Email`) from the auth response upstream.
- **Host preserved**, **SSE not buffered** on `/mcp`, **no stream timeout** —
  Caddy does these by default; nginx needs `proxy_set_header Host $host;`,
  `proxy_buffering off;`, and raised timeouts on `/mcp`.

:::danger[Exact-match the public URL]
`OAUTH_PUBLIC_URL` must equal the scheme+host Claude connects to. The MCP
endpoint is that URL + `/mcp`. A mismatch breaks token-audience validation.
:::

## Add the connector in Claude.ai

1. **Settings → Connectors → Add custom connector**.
2. **URL**: `https://polar.example.com/mcp`. Leave Advanced settings **empty** —
   no client id/secret is needed (the server self-registers Claude via DCR).
3. Save and connect. Claude opens a browser to Authelia; log in; you're returned
   to Claude and the tools (`get_user_info`, …) list.

For **Claude Code**, add the same URL as a remote MCP server; it performs the
same DCR + browser login.

## Verify

```bash
# Discovery metadata is public and points the AS at this server itself:
curl -s https://polar.example.com/.well-known/oauth-protected-resource | jq
curl -s https://polar.example.com/.well-known/oauth-authorization-server | jq

# Unauthenticated /mcp returns 401 pointing at the metadata:
curl -i -X POST https://polar.example.com/mcp
# → HTTP/1.1 401 Unauthorized
# → WWW-Authenticate: Bearer error="invalid_token", resource_metadata="https://polar.example.com/.well-known/oauth-protected-resource"
```

Then end-to-end: connect in Claude.ai, run `get_user_info`. Try logging in to
`/authorize` as a different Authelia user (if you have one) and confirm it is
refused — that proves the email allowlist works.

## Revoking access

Tokens are self-issued and stateless, so revocation is coarse-grained:

- **Short access lifetime** (`OAUTH_ACCESS_TTL_MINUTES`, default 60) limits a
  leaked access token's window.
- **Rotate the signing key** to invalidate *all* outstanding tokens at once:
  delete the file at `OAUTH_SIGNING_KEY_PATH` and restart. Every client must then
  re-authenticate.
- **Remove the email** from `OAUTH_ALLOWED_EMAIL` (and restart) to stop new
  tokens and refreshes for that identity — checked on every `/mcp` call and every
  refresh, so access stops within the access-token TTL.

## Security checklist

- [ ] Proxy serves a valid TLS cert; HTTP is redirected to HTTPS.
- [ ] Forward-auth is on `/mcp/oauth/authorize` **only** (not `/mcp`).
- [ ] `OAUTH_TRUSTED_PROXIES` is scoped to the proxy network (not `0.0.0.0/0`).
- [ ] `OAUTH_ALLOWED_EMAIL` lists only the identities that may connect.
- [ ] An Authelia rule locks `/authorize` to you (two_factor + subject).
- [ ] `OAUTH_SIGNING_KEY_PATH` is on a persistent, chmod-600 volume.
- [ ] A **dedicated** Polar account (not your main one) — see the
      [security model](/explanation/security-model/).
- [ ] Container bound to localhost/private, reachable only through the proxy.
