---
title: HTTP endpoints
description: The HTTP surface polar-flow-mcp exposes under the http transport, and its authentication postures.
sidebar:
  order: 3
---

When `TRANSPORT=http` (the default), polar-flow-mcp exposes the following
HTTP endpoints on `BIND_ADDRESS:PORT`.

## Endpoints

All `/.well-known/*` and `/mcp/oauth/*` endpoints are **only mounted when OAuth
is enabled** (`OAUTH_PUBLIC_URL` set).

| Path | Method | Auth | Purpose |
|------|--------|------|---------|
| `/healthz` | GET | none | Liveness probe. Returns `200 ok` if the process is up. |
| `/mcp`, `/mcp/` | POST + SSE | Bearer if OAuth enabled, else none | Streamable HTTP MCP endpoint. When OAuth is enabled, requires a valid `Authorization: Bearer` access token (an EdDSA JWT this server signed). |
| `/.well-known/oauth-protected-resource` | GET | none | RFC 9728 protected-resource metadata. Advertises `resource` and `authorization_servers` (which point back at this server). |
| `/.well-known/oauth-authorization-server` | GET | none | RFC 8414 authorization-server metadata: the authorize/token/registration endpoints and `S256` PKCE support. |
| `/mcp/oauth/register` | POST | none | RFC 7591 Dynamic Client Registration. Issues a stateless `client_id` for a public PKCE client; redirect URIs are restricted to the Claude callbacks + loopback. |
| `/mcp/oauth/authorize` | GET | forward-auth | Browser consent endpoint. **Must be forward-auth'd by your proxy** — the proxy logs the user in and sets the identity header; the server checks the email allowlist and issues a PKCE-bound code. |
| `/mcp/oauth/token` | POST | none (PKCE) | Token endpoint: `authorization_code` (with PKCE verification) and `refresh_token` grants. Returns the signed access + refresh JWTs. |

## What's gone

The previous OAuth flow (`/oauth/login`, `/oauth/callback`) and the
proxy-auth contract (`X-Proxy-Secret` header, `Remote-User` injection) no
longer exist. The Polar Flow web API uses cookie-based session auth — there
is no per-user OAuth handshake to host.

## Authentication

There are two supported postures:

- **Local / trusted-network (default).** With `OAUTH_PUBLIC_URL` unset, `/mcp` has
  no built-in auth. Run it on `127.0.0.1` for Claude Code / Claude Desktop on the
  same machine, or behind a trusted-network barrier (Tailscale, VPN). If
  `BIND_ADDRESS` is non-localhost **and** OAuth is off, the server logs a warning
  at startup.
- **Public, OAuth-protected.** Set `OAUTH_PUBLIC_URL` (+ `OAUTH_ALLOWED_EMAIL` +
  `OAUTH_TRUSTED_PROXIES`) to turn the server into its own OAuth 2.1 Authorization
  Server. Unauthenticated requests get `401` with a `WWW-Authenticate: Bearer
  resource_metadata="…"` header; valid Bearer tokens (EdDSA JWTs the server
  signed) are verified locally on every call. This path serves **Claude.ai
  web/mobile and Claude Code** — see
  [Expose the server securely](/guides/expose-securely/).

Because the server provides Dynamic Client Registration, **Claude Code (CLI)
works over the public OAuth path too** — no separate transport needed.

## Health-check semantics

`/healthz` returns `200 ok` as long as the process is alive. It does not
verify that Polar is reachable or that the cookie jar is still valid — a
401 on the first MCP call after a cookie expiry triggers the silent-refresh
path automatically, so a hard "Polar reachable" check would either be noisy
(every refresh window) or stale.

If you need a deeper probe, call `get_user_info` from your MCP client; it
exercises the full request → refresh-if-needed → response path.
