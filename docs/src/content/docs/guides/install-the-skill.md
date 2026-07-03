---
title: Install the polar-flow skill
description: Install the polar-flow skill so Claude knows which tool to call and the exact parameter shapes, and connect your Claude client to the server.
sidebar:
  order: 4
---

The `polar-flow` skill tells Claude how to drive the polar-flow-mcp tools
correctly — which tool does what, the exact parameter shapes, the units the API
expects, and how to handle failures — so you can speak in plain language rather
than remembering API parameters. It ships in the repository under
`skill/polar-flow/`: a `SKILL.md` entry point plus a `reference/` directory of
on-demand detail pages.

:::note
The `polar-flow` skill covers only the *mechanics* of calling the tools. It
deliberately does **not** prescribe training (periodization, warm-up/cool-down
choices, weekly load) — that is left to a separate, brand-agnostic sport-coaching
skill.
:::

This guide covers installing the skill **and** connecting your Claude client to
the running server — they are two separate steps.

## Install the skill

### Option 1: Claude Desktop or Claude Code

Works when you run Claude Desktop or Claude Code on your local machine:

1. Locate your Claude skills directory. On Claude Code this is typically
   `~/.claude/skills/`.
2. Copy (or symlink) the whole `polar-flow/` directory into it (the `reference/`
   files must travel with `SKILL.md`):

   ```bash
   cp -r skill/polar-flow ~/.claude/skills/
   ```

3. Restart Claude. The skill is auto-discovered and active for all
   conversations.

### Option 2: Claude.ai project file upload

Works for Claude.ai conversations via the Projects feature:

1. Open your Claude.ai project (or create one for training management).
2. Upload the files under `skill/polar-flow/` (`SKILL.md` and the `reference/`
   pages) as project files.
3. The skill becomes active for all chats in that project — no restart needed.

Both methods give Claude identical guidance and tool-calling behaviour.

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

See the [HTTP endpoints reference](/reference/http-endpoints/) for details
on the `/mcp` endpoint, and [Expose the server securely](/guides/expose-securely/) for
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
[Getting started](/tutorials/getting-started/) for the full walkthrough.
