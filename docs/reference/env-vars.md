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

## Removed (no longer used)

The following env vars existed before the migration to the Polar Flow web
API. They are now ignored — you can delete them from `.env`:

`POLAR_CLIENT_ID`, `POLAR_CLIENT_SECRET`, `POLAR_REDIRECT_URL`,
`ENCRYPTION_KEY`, `ENCRYPTION_KEY_FILE`, `KEY_PROVIDER`, `DATABASE_PATH`,
`AUTH_PROXY`, `PROXY_SHARED_SECRET`, `IDENTITY_HEADER`,
`DEV_MODE`, `DEV_USER_ID`.

The OAuth flow, SQLite store, encryption layer, and reverse-proxy auth
contract no longer exist. See [Security](../security.md) for the new model.
