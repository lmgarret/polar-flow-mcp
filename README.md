# polar-flow-mcp

Single-user MCP server that drives the **reverse-engineered Polar Flow web API**
(`flow.polar.com`) from Claude — including creating and deleting training targets,
which the official AccessLink API does not allow.

[![CI](https://img.shields.io/github/actions/workflow/status/lmgarret/polar-flow-mcp/ci.yml?branch=main&label=CI)](https://github.com/lmgarret/polar-flow-mcp/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/lmgarret/polar-flow-mcp)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/lmgarret/polar-flow-mcp)](https://github.com/lmgarret/polar-flow-mcp/releases/latest)

> **Not the official Polar AccessLink API.** The web API is reverse-engineered
> from browser traffic. Use a test account where possible. See `internal/flow/README.md`.

## Quickstart

```bash
git clone https://github.com/lmgarret/polar-flow-mcp.git
cd polar-flow-mcp
cp .env.example .env
# Edit .env — set POLAR_EMAIL and POLAR_PASSWORD
go run ./cmd/polar-flow-mcp
```

The server logs into Polar Flow once on cold start, persists the session
cookies to `./polar-cookies.json` (chmod 600), and re-uses them on subsequent
runs. The password is held in memory only.

## Tools

| Tool | Description |
|------|-------------|
| `get_user_info` | Linked Polar account identity + profile basics |
| `create_training_target` | Schedule a target (warmup / repeat / cooldown phases) |
| `list_training_targets` | Targets in a date range |
| `delete_training_target` | Delete a target by ID |
| `get_calendar_events` | Raw calendar events in a date range |
| `list_training_sessions` | Completed sessions in a date range |
| `get_training_session_summary` | Summary view of one session |
| `get_training_session_details` | Lap and sample detail of one session |

## Configuration

See [`.env.example`](.env.example). Required:

- `POLAR_EMAIL`, `POLAR_PASSWORD` — credentials for the Polar Flow account

Optional:

- `COOKIE_JAR_PATH` — where to persist session cookies (default `./polar-cookies.json`)
- `TRANSPORT` — `stdio` or `http` (default `http`)
- `BIND_ADDRESS`, `PORT` — HTTP listen address
- `LOG_LEVEL`, `LOG_FILE`

## Multi-user

This server is single-user by design: it acts as exactly one Polar account.
To serve more than one account, run multiple instances (e.g. one container per
user). The previous OAuth / proxy-auth machinery was removed because the Polar
Flow web API requires email/password and per-user credential storage is a
materially heavier risk profile than per-user OAuth tokens.

## How it works

`internal/flow/` wraps an [ogen](https://github.com/ogen-go/ogen)-generated
Go client against a vendored copy of the OpenAPI spec from the sibling
[polar-openapi-maker](https://github.com/lmgarret/polar-openapi-maker) repo.
The wrapper:

- Performs Polar's internal OAuth login chain headlessly (see `internal/flow/login.go`).
- Persists `FLOW_SESSION` + `remember-me` to a chmod-600 cookie jar.
- Injects `X-Requested-With: XMLHttpRequest` on every `/api/*` mutation
  (Play's CSRF filter requires it).
- Detects `401 {"error":"NotAuthenticated"}` and runs a 3-hop silent refresh
  before retrying the request once.

## License

MIT — see [LICENSE](LICENSE).
