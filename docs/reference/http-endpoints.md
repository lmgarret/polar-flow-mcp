# HTTP Endpoints

When `TRANSPORT=http` (the default), polar-flow-mcp exposes the following
HTTP endpoints on `BIND_ADDRESS:PORT`.

## Endpoints

| Path | Method | Auth | Purpose |
|------|--------|------|---------|
| `/healthz` | GET | none | Liveness probe. Returns `200 ok` if the process is up. |
| `/mcp`, `/mcp/` | POST + SSE | none | Streamable HTTP MCP endpoint mounted at this path. |

## What's gone

The previous OAuth flow (`/oauth/login`, `/oauth/callback`) and the
proxy-auth contract (`X-Proxy-Secret` header, `Remote-User` injection) no
longer exist. The Polar Flow web API uses cookie-based session auth — there
is no per-user OAuth handshake to host.

## No built-in authentication

This server expects to be either:

- Run **locally** (default `BIND_ADDRESS=127.0.0.1`) and consumed by Claude
  Desktop / Claude Code on the same machine, or
- Placed **behind a trusted proxy** (e.g. a Tailscale-only listener, a VPN,
  or a reverse proxy that handles authentication).

If `BIND_ADDRESS` is anything other than localhost, the server logs a warning
at startup. Single-user MCP servers are not designed for unprotected
exposure to the public internet.

## Health-check semantics

`/healthz` returns `200 ok` as long as the process is alive. It does not
verify that Polar is reachable or that the cookie jar is still valid — a
401 on the first MCP call after a cookie expiry triggers the silent-refresh
path automatically, so a hard "Polar reachable" check would either be noisy
(every refresh window) or stale.

If you need a deeper probe, call `get_user_info` from your MCP client; it
exercises the full request → refresh-if-needed → response path.
