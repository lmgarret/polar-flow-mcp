---
title: Environment variables
description: Every environment variable polar-flow-mcp reads, with defaults and behaviour notes.
sidebar:
  order: 2
---

polar-flow-mcp is configured entirely through environment variables (loaded
from `.env` at startup via [`godotenv`](https://github.com/joho/godotenv)).
The server is fail-closed: missing required variables cause it to refuse to
start with a descriptive error.

## Required

| Variable | Description |
|----------|-------------|
| `POLAR_EMAIL` | Email of the Polar Flow account this server will act as. |
| `POLAR_PASSWORD` | Password for that account. Held in memory only — never written to disk. |

## Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `TRANSPORT` | `http` | `http` (streamable HTTP at `/mcp`) or `stdio` (MCP over stdin/stdout). |
| `COOKIE_JAR_PATH` | `./polar-cookies.json` | Path to the chmod-600 JSON file holding `FLOW_SESSION` + `remember-me`. Use an absolute path in production. |
| `BIND_ADDRESS` | `127.0.0.1` | TCP address the HTTP server binds to. Anything other than localhost emits a warning at startup. |
| `PORT` | `8080` | TCP port for the HTTP server. |
| `LOG_LEVEL` | `info` | `info` or `debug`. |
| `LOG_FILE` | *(empty — stderr)* | If set, slog output is appended to this file (chmod 0600). Useful when stderr is unavailable, e.g. when running under Claude Code's stdio transport. |

## Inbound OAuth (app-as-Authorization-Server + DCR)

Setting `OAUTH_PUBLIC_URL` turns the server into its **own** OAuth 2.1
Authorization Server. Clients self-register via Dynamic Client Registration
(RFC 7591) — nothing to pre-create — and every `/mcp` request must carry a valid
`Authorization: Bearer` access token the server itself signed (EdDSA JWT,
validated locally). Browser login on `/mcp/oauth/authorize` is delegated to a
forward-auth proxy. When `OAUTH_PUBLIC_URL` is unset, none of these apply and
`/mcp` is unauthenticated (the historical behaviour). See
[Expose the server securely](/guides/expose-securely/) for the full Caddy +
Authelia + Claude.ai walkthrough.

:::note
When `OAUTH_PUBLIC_URL` **is** set, the server is fail-closed:
`OAUTH_ALLOWED_EMAIL` and `OAUTH_TRUSTED_PROXIES` are mandatory.
:::

| Variable | Default | Description |
|----------|---------|-------------|
| `OAUTH_PUBLIC_URL` | *(unset → auth off)* | Public origin and OAuth issuer, e.g. `https://polar.example.com`. The MCP resource (token audience) is this URL + `/mcp`. Setting this **enables** inbound OAuth and must match the host Claude connects to. |
| `OAUTH_ALLOWED_EMAIL` | *(required if auth on)* | Comma-separated allowlist of forward-auth identities permitted to consent. Matching is case-insensitive. This is the "lock to me" control. |
| `OAUTH_TRUSTED_PROXIES` | *(required if auth on)* | Comma-separated CIDRs/IPs whose forward-auth identity header is trusted on `/authorize`. An identity header from any other peer is ignored, so this is what stops header spoofing. Scope it to your proxy/container network. |

### Optional OAuth tuning

| Variable | Default | Description |
|----------|---------|-------------|
| `OAUTH_ALLOWED_GROUPS` | *(unset)* | Comma-separated group allowlist. When set, the forward-auth groups header must also intersect this set to consent. |
| `OAUTH_ALLOWED_ORIGINS` | *(unset)* | Comma-separated `Origin` allowlist on `/mcp` (DNS-rebinding defence). Enforced only when set; requests with no `Origin` (Claude's server-to-server calls) are always allowed. |
| `OAUTH_FORWARD_AUTH_EMAIL_HEADER` | `Remote-Email` | Forward-auth header carrying the authenticated email (Authelia default). |
| `OAUTH_FORWARD_AUTH_GROUPS_HEADER` | `Remote-Groups` | Forward-auth header carrying the user's groups. |
| `OAUTH_EXTRA_REDIRECT_URIS` | *(unset)* | Comma-separated extra exact redirect URIs accepted at registration, beyond the built-in Claude callbacks and loopback. Normally empty. |
| `OAUTH_SIGNING_KEY_PATH` | `./polar-oauth-key.json` | chmod-600 JSON file holding the EdDSA signing key. Created on first start; persist it (e.g. on a named volume) so issued tokens survive restarts. |
| `OAUTH_ACCESS_TTL_MINUTES` | `60` | Access-token lifetime in minutes. |
| `OAUTH_REFRESH_TTL_HOURS` | `720` | Refresh-token lifetime in hours (default 30 days). |

## Removed (no longer used)

The following env vars existed before the migration to the Polar Flow web
API. They are now ignored — you can delete them from `.env`:

`POLAR_CLIENT_ID`, `POLAR_CLIENT_SECRET`, `POLAR_REDIRECT_URL`,
`ENCRYPTION_KEY`, `ENCRYPTION_KEY_FILE`, `KEY_PROVIDER`, `DATABASE_PATH`,
`AUTH_PROXY`, `PROXY_SHARED_SECRET`, `IDENTITY_HEADER`,
`DEV_MODE`, `DEV_USER_ID`.

The OAuth flow, SQLite store, encryption layer, and reverse-proxy auth
contract no longer exist. See the [security model](/explanation/security-model/)
for the current design.
