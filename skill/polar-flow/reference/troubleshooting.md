# Troubleshooting & setup

How to tell what went wrong, degrade safely, and install the skill. The
polar-flow-mcp server is self-hosted, so connectivity and credential problems
are normal failure modes — surface them honestly, never paper over them.

## Connectivity check (do this first)

Before any tool call, confirm the polar-flow-mcp tools appear in your available
tool list. If they don't, the server isn't connected:

1. Tell the user the polar-flow-mcp server isn't connected.
2. Point them at the project `README.md` for setup.
3. Do **not** fabricate tool calls or invent data.

## Reading errors

| Symptom | Likely cause | What to do |
|---------|--------------|------------|
| Tools missing from your tool list | Server not connected | See connectivity check above. |
| "Polar credentials rejected" / "no Polar credentials configured" | `POLAR_EMAIL` / `POLAR_PASSWORD` wrong or missing | Tell the user to fix `.env` (or container env) and restart the server. |
| `4xx` from the API | Malformed request body (bad units, missing field, time conflict) | Surface the message verbatim. Re-check units/required fields against `reference/training-targets.md`. Don't blindly retry. |
| `5xx` or transport error | Server-side or Polar-side fault | Surface verbatim; ask the user to check server logs. Don't retry silently. |
| `get_calendar_week_summary` range error | Span > 45 days or `from_date > to_date` | Shrink/flip the range and retry. |
| Empty/zero `get_progress_summary` | No sessions in range (not an error) | Report "no data in that window", widen the range if appropriate. |

## Safe degradation

- **Never fabricate results when a tool fails.** Honest failure beats
  hallucinated success.
- **Don't retry a failed write blindly.** A `4xx` usually means the body needs
  fixing, not resending. Diagnose first.
- **One write at a time.** If creating several targets (e.g. a week of
  sessions), create them one by one and report what landed, so a mid-sequence
  failure is visible and recoverable.
- **Re-read after a write** when correctness matters — `get_training_target`
  after an update, `list_training_sessions` after a log.

## Installation

The skill is the `polar-flow/` directory (this `SKILL.md` plus `reference/`).

### Claude Code / Claude Desktop (local skills directory)

1. Locate your skills directory — on Claude Code typically `~/.claude/skills/`;
   on Claude Desktop see the platform's skills documentation.
2. Copy the entire `polar-flow/` directory into it.
3. Restart Claude. The skill is auto-discovered.

### Claude.ai (project files)

1. Open or create a Claude.ai project.
2. Upload `SKILL.md` and the `reference/` files as project files.
3. The skill becomes active for chats in that project.

The skill is useful only when the polar-flow-mcp server is running and
connected as an MCP server — see the project `README.md` to stand it up.
