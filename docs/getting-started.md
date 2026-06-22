# Getting Started

This walkthrough takes you from a fresh checkout to a running polar-flow-mcp
that Claude can talk to. Allow ~5 minutes.

> **Use a Polar test account if you can.** This server drives the unofficial
> reverse-engineered Polar Flow web API — your credentials are sent to
> `auth.polar.com` on every cold start and persisted as session cookies on
> disk.

## Prerequisites

- A Polar Flow account (email + password) — ideally a dedicated test account.
- One of the following:
    - **Go 1.26+** if you want to run from source.
    - **Docker** if you prefer the container path (recommended for anything
      long-running).
- A Claude client (Claude Code CLI, Claude Desktop, or another MCP-capable
  client).

## 1. Clone

```bash
git clone https://github.com/lmgarret/polar-flow-mcp.git
cd polar-flow-mcp
```

## 2. Configure

Copy the example env file and fill it in:

```bash
cp .env.example .env
$EDITOR .env
```

Only two values are required:

```dotenv
POLAR_EMAIL=you+polartest@example.com
POLAR_PASSWORD=correct-horse-battery-staple
```

Optional defaults you might want to override:

```dotenv
TRANSPORT=stdio                   # or http (default)
COOKIE_JAR_PATH=./polar-cookies.json
BIND_ADDRESS=127.0.0.1            # http transport only
PORT=8080
LOG_LEVEL=info                    # info | debug
```

See [Environment Variables](reference/env-vars.md) for the full reference.

## 3. Run

### From source

```bash
go run ./cmd/polar-flow-mcp
```

On the **first** run, the server performs the headless OAuth login chain
against `auth.polar.com`, persists `FLOW_SESSION` + `remember-me` to
`polar-cookies.json` (mode 0600), and starts the MCP server.

On **subsequent** runs, the server re-uses the cookie jar and skips the
password step entirely. Polar's `remember-me` cookie rolls forward on each
use, so an actively-used server never re-prompts.

### With Docker

```bash
docker compose up -d
```

See the [Docker Compose page](deployment/docker-compose.md) for the
compose file structure and volume layout.

## 4. Wire it into Claude

### Claude Code (stdio)

Add to `~/.claude/mcp_servers.json` (or per-project `.mcp.json`):

```json
{
  "mcpServers": {
    "polar-flow": {
      "command": "/absolute/path/to/polar-flow-mcp",
      "env": {
        "TRANSPORT": "stdio",
        "POLAR_EMAIL": "you+polartest@example.com",
        "POLAR_PASSWORD": "correct-horse-battery-staple",
        "COOKIE_JAR_PATH": "/absolute/path/to/polar-cookies.json"
      }
    }
  }
}
```

### Claude Desktop / HTTP MCP

Run with `TRANSPORT=http` (the default) and point Claude at
`http://127.0.0.1:8080/mcp`.

To reach it from **Claude.ai web/mobile (and Claude Code)** over the public
internet, set `OAUTH_PUBLIC_URL` to turn the server into its own OAuth 2.1
Authorization Server, and front it with a TLS reverse proxy — see
[Exposing Securely](deployment/exposing-securely.md).

## 5. Try it out

In a Claude conversation:

```
You: who's linked to polar-flow?
Claude: [calls get_user_info]
Claude: Linked Polar account: you+polartest@example.com (FR).

You: what's on the calendar this week?
Claude: [calls list_training_targets with today..+7d]

You: schedule a 5x1km threshold session for Thursday at 18:00.
Claude: [calls create_training_target with the appropriate phase tree]
```

If something failed, check `LOG_LEVEL=debug` for the request / refresh trace.

## What's next

- [Usage guide](usage.md) — phase vocabulary and worked examples
- [MCP Tools reference](reference/mcp-tools.md) — full argument schemas
- [Exposing Securely](deployment/exposing-securely.md) — public access for Claude.ai (Caddy + Authelia)
- [Security model](security.md) — what's protected and what isn't
