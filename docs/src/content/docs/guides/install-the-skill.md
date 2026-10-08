---
title: Install the polar-flow skill
description: Install the polar-flow skill so Claude picks the right tool, follows the safe-write rules and chains multi-step edits, and connect your Claude client to the server.
sidebar:
  order: 4
---

The `polar-flow` skill tells Claude how to use the polar-flow-mcp tools
together — which tool fits a request, the rules that span tools (full-replace
updates, never inventing session history, confirming writes, turning a pace or
wattage into a zone), and multi-step workflows such as editing a planned
workout. Per-tool parameters, units and limits live in the tools' own
descriptions, which every MCP client receives with or without the skill. It
ships in the repository under `skill/polar-flow/`: a `SKILL.md` entry point
plus a `reference/` directory of on-demand detail pages.

:::note
The `polar-flow` skill covers only the *mechanics* of calling the tools. It
deliberately does **not** prescribe training (periodization, warm-up/cool-down
choices, weekly load) — that is left to a separate, brand-agnostic sport-coaching
skill.
:::

This guide covers installing the skill **and** connecting your Claude client to
the running server — they are two separate steps.

## Install the skill

### Option 1: Claude Code plugin marketplace

The repository is also a Claude Code plugin marketplace. Add it and install
the `polar-flow` plugin, which carries the skill:

```bash
/plugin marketplace add lmgarret/polar-flow-mcp
/plugin install polar-flow@polar-flow-mcp
```

The skill then loads as `polar-flow:polar-flow` and follows `main` whenever
you run `/plugin marketplace update`. To stay on the skill matching your
server release instead, add the marketplace at that tag:
`/plugin marketplace add lmgarret/polar-flow-mcp@v1.2.3`.

The plugin installs only the skill — connect the server separately (below).

### Option 2: `.skill` file from a release

Every [GitHub release](https://github.com/lmgarret/polar-flow-mcp/releases)
carries a `polar-flow.skill` asset — the skill for that version, as a zip with
`polar-flow/SKILL.md` at its root. The rolling `edge` pre-release carries one
built from `main`.

- **Claude.ai / Claude Desktop**: upload `polar-flow.skill` in the Skills
  section of the settings.
- **Claude Code without the marketplace**: unzip it into your skills
  directory:

  ```bash
  unzip polar-flow.skill -d ~/.claude/skills/
  ```

To build it from a checkout, run `make skill` (writes `bin/polar-flow.skill`).

### Option 3: Copy from a checkout

Copy (or symlink) the whole `polar-flow/` directory into `~/.claude/skills/`
— the `reference/` files must travel with `SKILL.md` — then restart Claude:

```bash
cp -r skill/polar-flow ~/.claude/skills/
```

All options give Claude identical guidance and tool-calling behaviour.

## Connect Claude to your server

The skill tells Claude *what* tools exist and *how* to use them. Separately, you
must point your Claude client at the polar-flow-mcp server.

### HTTP mode (production)

Add your server URL (`https://<your-host>/mcp`) as an MCP server endpoint. The
server uses StreamableHTTP transport — no WebSocket or SSE setup needed. When the
server is exposed publicly, the MCP connection goes through your reverse proxy,
which handles OAuth.

```bash
# Claude Code (project-scoped)
claude mcp add --transport http polar-flow-mcp --scope project https://<your-host>/mcp
```

See the [HTTP endpoints reference](/polar-flow-mcp/reference/http-endpoints/) for details
on the `/mcp` endpoint, and [Expose the server securely](/polar-flow-mcp/guides/expose-securely/) for
the public OAuth setup.

### Stdio mode (local development)

For local development, run the server in `stdio` mode. Claude Code launches the
binary directly and communicates over stdin/stdout.

```bash
claude mcp add --transport stdio polar-flow-mcp -- /absolute/path/to/polar-flow-mcp
```

The binary reads its configuration from the environment. Set at minimum:

```bash
TRANSPORT=stdio
POLAR_EMAIL=you+polartest@example.com
POLAR_PASSWORD=correct-horse-battery-staple
COOKIE_JAR_PATH=/absolute/path/to/polar-cookies.json
LOG_FILE=/tmp/polar-flow-mcp.log   # required — logs cannot go to stderr in stdio mode
```

Place these in a `.env` file in the working directory, or pass them via the
`env` block in your MCP server configuration. See
[Getting started](/polar-flow-mcp/tutorials/getting-started/) for the full walkthrough.
