---
title: Why the Flow web API (not AccessLink)
description: The reasoning behind driving the reverse-engineered Polar Flow web API instead of the official AccessLink API, and how the pieces fit together.
sidebar:
  order: 1
---

polar-flow-mcp drives the reverse-engineered **Polar Flow web API**
(`flow.polar.com`) — the one the browser app uses — rather than Polar's official
AccessLink API. This page explains why, and what that choice implies.

## The problem with AccessLink

The official Polar AccessLink API is **read-only**: it can list training history
but cannot create or delete training targets. The whole point of this project —
letting Claude *schedule* structured workouts in your diary — is impossible on
AccessLink.

The web API at `flow.polar.com` — the one the Polar Flow browser app uses —
supports the full set of mutating operations. So that is what this server
targets.

:::caution[This is not an official Polar API]
It is reverse-engineered from browser traffic and unsupported by Polar. It can
break when Polar changes its web app. Use a Polar test account where you can, and
expect no warranty.
:::

## How the pieces fit together

The OpenAPI spec is maintained as a sibling repo,
[polar-openapi-maker](https://github.com/lmgarret/polar-openapi-maker),
reverse-engineered from browser traffic. polar-flow-mcp:

1. **Vendors the spec** under `internal/flow/openapi.yaml`.
2. **Generates a Go client** from it with
   [ogen](https://github.com/ogen-go/ogen).
3. **Wraps the client** with the headless login chain, a cookie-jar with silent
   refresh, and the header/CSRF quirks the web API requires.

Every wire call is therefore type-checked Go generated from the spec, not
hand-rolled HTTP. See [Design decisions](/explanation/design-decisions/) for the details of
the wrapper and why each quirk exists.

```d2
direction: down

claude: Claude client

server: polar-flow-mcp {
  transport: MCP transport (stdio / HTTP)
  auth: OAuth 2.1 AS (optional) {
    style.stroke-dash: 3
  }
  tools: MCP tools
  flow: Flow client {
    ogen: ogen client (generated)
    tr: custom transport (uTLS + CSRF)
    jar: cookie jar (chmod 600)
    login: login + silent refresh
  }
}

polar: Polar {
  waf: flow.polar.com (WAF)
  authp: auth.polar.com
}

claude -> server.transport: MCP JSON-RPC
server.auth -> server.transport: Bearer JWT (if public) {
  style.stroke-dash: 3
}
server.transport -> server.tools
server.tools -> server.flow.ogen
server.flow.ogen -> server.flow.tr
server.flow.tr -> server.flow.jar: session cookie
server.flow.tr -> polar.waf: HTTPS
server.flow.login -> polar.authp: login / refresh
```

The dashed OAuth 2.1 Authorization Server is **optional** — it is off by default
and only turns on when you set `OAUTH_PUBLIC_URL` to expose the server publicly.
With it unset, `/mcp` has no inbound auth (run it on localhost or behind a
trusted-network barrier).

| Capability | Tool(s) |
|------------|---------|
| Confirm the linked account | `get_user_info` |
| Schedule a structured workout | `create_training_target` |
| List / read / edit / delete targets | `list_training_targets`, `get_training_target`, `update_training_target`, `delete_training_target` |
| Read the calendar & weekly totals | `get_calendar_events`, `get_calendar_week_summary` |
| Read completed sessions | `list_training_sessions`, `get_training_session_summary`, `get_training_session_details` |
| Aggregate progress over a range | `get_progress_summary` |
| Log a manual session | `create_training_session` |

The write operations — creating, editing, and deleting targets, and logging
manual sessions — are exactly the ones AccessLink cannot do. See the
[MCP tools reference](/reference/mcp-tools/) for full argument schemas.

## Tech stack

| Component | Choice |
|-----------|--------|
| Language | Go 1.26 (CGO disabled) |
| MCP server | [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) (stdio + streamable HTTP) |
| OpenAPI client | [`ogen-go/ogen`](https://github.com/ogen-go/ogen) |
| Spec source | [polar-openapi-maker](https://github.com/lmgarret/polar-openapi-maker) (vendored) |
| Persistence | Plain JSON cookie jar (chmod 600) |
| Image | `FROM scratch` (~14 MB) |
