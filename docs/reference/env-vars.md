# Environment Variables

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

## Inbound OAuth (Resource Server)

Setting `OIDC_ISSUER` turns the HTTP transport into an OAuth 2.1 Resource
Server: every request to `/mcp` must carry a valid `Authorization: Bearer`
token, validated by RFC 7662 introspection against the issuer. When
`OIDC_ISSUER` is unset, none of these apply and `/mcp` is unauthenticated (the
historical behaviour). See [Exposing Securely](../deployment/exposing-securely.md)
for the full Caddy + Authelia + Claude.ai walkthrough.

When `OIDC_ISSUER` **is** set, the server is fail-closed: `MCP_RESOURCE` and the
introspection credentials are mandatory.

| Variable | Default | Description |
|----------|---------|-------------|
| `OIDC_ISSUER` | *(unset → auth off)* | OIDC issuer base URL, e.g. `https://auth.example.com`. Its `/.well-known/openid-configuration` is fetched to discover the introspection endpoint. Setting this **enables** inbound OAuth. |
| `MCP_RESOURCE` | *(required if auth on)* | This server's exact public `/mcp` URL, e.g. `https://polar.example.com/mcp`. Must byte-match the URL Claude calls; published in the protected-resource metadata. |
| `OIDC_INTROSPECTION_CLIENT_ID` | *(required if auth on)* | Client id the server uses to authenticate to the issuer's introspection endpoint. |
| `OIDC_INTROSPECTION_CLIENT_SECRET` | *(required if auth on)* | Secret for the introspection client. |

### Recommended hardening (each enforced only when set; all ANDed)

| Variable | Mitigates | Description |
|----------|-----------|-------------|
| `AUTH_ALLOWED_CLIENT_IDS` | Confused deputy (RFC 8707) | Comma-separated `client_id` allowlist — reject tokens not minted for your connector client. **Recommended.** |
| `AUTH_ALLOWED_AUDIENCES` | Token replay across services | Comma-separated `aud` allowlist (typically your `MCP_RESOURCE`). |
| `AUTH_ALLOWED_SUBJECTS` | Other users on your IdP | Comma-separated `sub` allowlist — restrict to your identity. Authelia's `sub` is a UUID; the server logs the observed value on each rejection. |
| `AUTH_ALLOWED_GROUPS` | Other users on your IdP | Comma-separated group allowlist — alternative to `AUTH_ALLOWED_SUBJECTS` for multi-user IdPs. |
| `AUTH_ALLOWED_ORIGINS` | DNS rebinding | Comma-separated `Origin` allowlist. Enforced **only** when set; requests with no `Origin` (e.g. Claude's server-to-server calls) are always allowed. |

## Removed (no longer used)

The following env vars existed before the migration to the Polar Flow web
API. They are now ignored — you can delete them from `.env`:

`POLAR_CLIENT_ID`, `POLAR_CLIENT_SECRET`, `POLAR_REDIRECT_URL`,
`ENCRYPTION_KEY`, `ENCRYPTION_KEY_FILE`, `KEY_PROVIDER`, `DATABASE_PATH`,
`AUTH_PROXY`, `PROXY_SHARED_SECRET`, `IDENTITY_HEADER`,
`DEV_MODE`, `DEV_USER_ID`.

The OAuth flow, SQLite store, encryption layer, and reverse-proxy auth
contract no longer exist. See [Security](../security.md) for the new model.
