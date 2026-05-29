# HTTP Endpoints

When `TRANSPORT=http` (the default), polar-flow-mcp exposes the following
HTTP endpoints on `BIND_ADDRESS:PORT`.

## Endpoints

| Path | Method | Auth | Purpose |
|------|--------|------|---------|
| `/healthz` | GET | none | Liveness probe. Returns `200 ok` if the process is up. |
| `/mcp`, `/mcp/` | POST + SSE | Bearer if OAuth enabled, else none | Streamable HTTP MCP endpoint. When `OIDC_ISSUER` is set, requires a valid `Authorization: Bearer` token (validated by introspection). |
| `/.well-known/oauth-protected-resource` | GET | none | RFC 9728 protected-resource metadata. **Only mounted when OAuth is enabled.** Advertises the `resource` and `authorization_servers` so clients can discover the issuer. |

## What's gone

The previous OAuth flow (`/oauth/login`, `/oauth/callback`) and the
proxy-auth contract (`X-Proxy-Secret` header, `Remote-User` injection) no
longer exist. The Polar Flow web API uses cookie-based session auth — there
is no per-user OAuth handshake to host.

## Authentication

There are two supported postures:

- **Local / trusted-network (default).** With `OIDC_ISSUER` unset, `/mcp` has no
  built-in auth. Run it on `127.0.0.1` for Claude Code / Claude Desktop on the
  same machine, or behind a trusted-network barrier (Tailscale, VPN). If
  `BIND_ADDRESS` is non-localhost **and** OAuth is off, the server logs a warning
  at startup.
- **Public, OAuth-protected.** Set `OIDC_ISSUER` (+ `MCP_RESOURCE` +
  introspection credentials) to turn the server into an OAuth 2.1 Resource
  Server. Unauthenticated requests get `401` with a `WWW-Authenticate: Bearer
  resource_metadata="…"` header; valid Bearer tokens are checked by RFC 7662
  introspection on every call. This is the path for **Claude.ai web/mobile** —
  see [Exposing Securely](../deployment/exposing-securely.md).

Note: Claude **Code (CLI)** cannot use the public OAuth path (it forces Dynamic
Client Registration); keep it on the local stdio / `127.0.0.1` transport.

## Health-check semantics

`/healthz` returns `200 ok` as long as the process is alive. It does not
verify that Polar is reachable or that the cookie jar is still valid — a
401 on the first MCP call after a cookie expiry triggers the silent-refresh
path automatically, so a hard "Polar reachable" check would either be noisy
(every refresh window) or stale.

If you need a deeper probe, call `get_user_info` from your MCP client; it
exercises the full request → refresh-if-needed → response path.
