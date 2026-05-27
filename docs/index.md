# polar-flow-mcp

A single-user MCP (Model Context Protocol) server that drives the
reverse-engineered **Polar Flow web API** (`flow.polar.com`) from Claude —
including creating, listing, and deleting training targets.

> **"Schedule a 5×1km threshold session for Thursday."**
> Claude calls `create_training_target` and the workout appears in your Polar
> Flow diary — no app, no UI navigation.

![Claude using create_training_target](images/claude-tool-call.png)

---

## Why the Flow web API and not AccessLink?

The official Polar AccessLink API is read-only: it can list training history
but cannot create or delete training targets. The web API at `flow.polar.com`
— the one the Polar Flow browser app uses — supports the full set of mutating
operations.

The OpenAPI spec is maintained as a sibling repo,
[polar-openapi-maker](https://github.com/lmgarret/polar-openapi-maker),
reverse-engineered from browser traffic. polar-flow-mcp vendors the spec,
generates a Go client with [ogen](https://github.com/ogen-go/ogen), and wraps
it with the headless login chain and cookie-jar refresh logic that the API
requires.

> **This is not an official Polar API.** Use a test account where possible.

---

## What it does

| Tool | Purpose |
|------|---------|
| `get_user_info` | Confirm the linked Polar account |
| `create_training_target` | Schedule a structured workout (warmup / repeat / cooldown) |
| `list_training_targets` | List upcoming scheduled targets |
| `delete_training_target` | Delete a target by ID |
| `get_training_target` | Read one target by ID |
| `update_training_target` | Full-replace edit of one target by ID |
| `get_calendar_events` | Raw calendar events in a date range |
| `get_calendar_week_summary` | Per-ISO-week totals strip (≤45-day range) |
| `list_training_sessions` | Completed training sessions in a date range |
| `get_training_session_summary` | Summary view of one completed session |
| `get_training_session_details` | Lap/sample-level detail of one session |
| `get_progress_summary` | Aggregated training totals over a range |

See the [MCP Tools reference](reference/mcp-tools.md) for argument details.

---

## Key design choices

- **Single-user** — the server logs into exactly one Polar account
  (`POLAR_EMAIL` / `POLAR_PASSWORD`). To serve more than one account, run more
  than one instance (e.g. one container per user). The Flow web API requires
  email + password; per-user credential storage is materially heavier on risk
  than per-user OAuth tokens.
- **Cookies-only persistence** — the password is held in memory only. The
  server logs in once, persists `FLOW_SESSION` + `remember-me` to a chmod-600
  JSON jar, and re-uses them on later runs. Polar's rolling 14-day
  `remember-me` cookie means an active server never has to re-prompt.
- **Automatic session refresh** — when Polar issues `401 NotAuthenticated`,
  the wrapper runs Polar's 3-hop silent refresh and retries the request once,
  transparently to the MCP caller.
- **No database** — no SQLite, no migrations, no encrypted token store. Just
  the cookie jar file.
- **Generated client** — every wire call is type-checked Go from the OpenAPI
  spec, not hand-rolled HTTP.

---

## Tech stack

| Component | Choice |
|-----------|--------|
| Language | Go 1.26 (CGO disabled) |
| MCP server | [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) (stdio + streamable HTTP) |
| OpenAPI client | [`ogen-go/ogen`](https://github.com/ogen-go/ogen) |
| Spec source | [polar-openapi-maker](https://github.com/lmgarret/polar-openapi-maker) (vendored) |
| Persistence | Plain JSON cookie jar (chmod 600) |
| Image | `FROM scratch` (~14 MB) |

---

## Getting started

1. [Get up and running](getting-started.md) (5-minute walkthrough)
2. [Configure environment variables](reference/env-vars.md)
3. [Deploy with Docker Compose](deployment/docker-compose.md)
4. [Use the tools from Claude](usage.md)
