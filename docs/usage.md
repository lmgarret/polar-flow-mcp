# Usage

This guide covers the four MCP tools exposed by polar-flow-mcp, with worked examples,
HR zone vocabulary, and instructions for installing the `polar-coach` skill in Claude.

---

## Overview

Once polar-flow-mcp is deployed and your Polar account is linked, you interact with it
through Claude conversations. The `polar-coach` skill guides Claude on when and how to
call each tool, so you can use plain language rather than remembering API parameters.

The four available tools are:

| Tool | Purpose |
|------|---------|
| `get_user_info` | Confirm the linked Polar account |
| `create_training_target` | Create a structured workout in Polar Flow |
| `list_training_targets` | List upcoming scheduled workouts |
| `delete_training_target` | Delete a workout by its ID |

![Claude tool call](images/claude-tool-call.png)

---

## HR zones and intensity vocabulary

Polar Flow uses five heart-rate zones (Z1–Z5). When creating training targets, you
specify intensity using either a **label** (preferred) or a numeric zone.

| Label | Zone | Coaching meaning |
|-------|------|-----------------|
| `easy` | Z1 | Recovery pace. Conversational, fully aerobic. Use for shakeout runs, warm-ups, recovery jogs. |
| `aerobic` | Z2 | Easy aerobic / base building. Comfortable, sustainable for long runs. Bulk of weekly volume. |
| `tempo` | Z3 | Steady-state, comfortably hard. Marathon to half-marathon effort. |
| `threshold` | Z4 | Lactate threshold. Hard, sustainable for ~30–60 min. Classic 1km repeats, cruise intervals. |
| `vo2max` | Z5 | Very hard. Short repeats (1–5 min) at near-maximal aerobic effort. |

Always use labels in conversation — they are more readable and match coaching vocabulary.
Use numeric zone numbers (`hr_zone: 4`) only when a user explicitly says "zone 4" or "Z3".

---

## The four tools

### `get_user_info`

Confirms which Polar account is linked to your identity. Call this first if you are
unsure whether your Polar account is connected.

**Ask Claude:**

> "What Polar account is linked?"

**What Claude does:** Calls `get_user_info` with no parameters. Returns your Polar user
ID if linked, or a message directing you to `/oauth/login` if not yet linked.

**Example response:**

```
Your Polar account is linked. Polar user ID: 12345678.
```

---

### `create_training_target`

Creates a single scheduled workout in Polar Flow. The workout is structured as a flat
phases array: an optional warmup, one or more repeat blocks, and an optional cooldown.

**Ask Claude:**

> "Create a 5x1km threshold session with 2-minute recovery for next Thursday"

**What Claude does:** Resolves the date, applies default warmup (10 min) and cooldown
(5 min), maps "threshold" to Z4, and calls `create_training_target`.

**Tool input:**

```json
{
  "name": "5x1km Threshold",
  "date": "2026-05-21",
  "time": "18:00",
  "sport": "RUNNING",
  "phases": [
    { "type": "warmup", "duration_s": 600 },
    {
      "type": "repeat",
      "reps": 5,
      "goal": { "distance_m": 1000 },
      "intensity": { "label": "threshold" },
      "recovery": { "duration_s": 120 }
    },
    { "type": "cooldown", "duration_s": 300 }
  ]
}
```

**Defaults:** If you do not specify them, Claude uses:

- Warmup: 10 minutes (600 seconds)
- Cooldown: 5 minutes (300 seconds)
- Time: 18:00
- Sport: RUNNING

**Response:** The tool returns the new target's `ID`. Claude will tell you the session
was created and include the ID for future reference (e.g., to delete it).

---

### `list_training_targets`

Lists upcoming training targets in Polar Flow for a date range.

**Ask Claude:**

> "What do I have planned this week?" or "Show me my workouts for the next two weeks"

**What Claude does:** Calls `list_training_targets` with `from_date` set to today and
`to_date` set to end of the requested range. If no range is specified, the default is
today through +30 days.

**Tool input:**

```json
{
  "from_date": "2026-05-12",
  "to_date": "2026-05-19"
}
```

**Response:** A list of upcoming sessions with their `target_id`, name, date, and sport.
Claude formats this as a readable summary.

---

### `delete_training_target`

Deletes a training target by its `target_id`.

**Ask Claude:**

> "Delete the threshold session on Thursday" or "Remove the workout with ID abc123"

**What Claude does:**

1. If you have not provided an ID, calls `list_training_targets` to find matching sessions.
2. Confirms which session you mean (shows name + date).
3. Calls `delete_training_target` with the `target_id`.

**Tool input:**

```json
{
  "target_id": "abc123"
}
```

**Response:** Confirmation that the session was deleted. If the session was not found,
Claude tells you plainly — this is not an error (it may have already been deleted
through the Polar app).

---

## Worked examples

### Example 1: Simple interval session

> "Create a 5x1km threshold session with 2-minute recovery for next Thursday"

Claude creates one workout with warmup, five 1km threshold repeats with 2-minute
recoveries, and a cooldown. The session appears in Polar Flow scheduled for that Thursday.

### Example 2: Mixed-zone workout

> "3x1km at threshold, then 2x500m at vo2max, this Saturday"

```json
{
  "name": "Threshold + VO2max ladder",
  "date": "2026-05-15",
  "phases": [
    { "type": "warmup", "duration_s": 600 },
    {
      "type": "repeat", "reps": 3,
      "goal": { "distance_m": 1000 },
      "intensity": { "label": "threshold" },
      "recovery": { "duration_s": 90 }
    },
    {
      "type": "repeat", "reps": 2,
      "goal": { "distance_m": 500 },
      "intensity": { "label": "vo2max" },
      "recovery": { "duration_s": 180 }
    },
    { "type": "cooldown", "duration_s": 300 }
  ]
}
```

### Example 3: Building a marathon plan iteratively

> "Build me a 12-week marathon plan starting in three weeks"

For a multi-session plan, Claude asks clarifying questions first (peak mileage, long-run
day, rest days), then proposes week 1 for your review, and creates sessions one week at
a time after confirmation. This keeps you in control and prevents accidental calendar
pollution.

---

## What the tools do NOT support

- **Pace targets** — v1 supports HR zones only. Ask for the closest zone equivalent.
- **Power targets** — v1 does not support watt-based intensity.
- **Past session analytics** — this server writes targets only; it does not read
  historical activity data.
- **Polar account creation or device sync** — outside this server's scope.

---

## Installing the skill

The `polar-coach` skill is a plain Markdown file (`SKILL.md`) that tells Claude how to
use the four MCP tools. It is included in the repository at `skill/polar-coach/SKILL.md`.

### Option 1: Claude Desktop or Claude Code

This method works if you are using Claude Desktop or Claude Code on your local machine:

1. Locate your Claude skills directory. On Claude Code this is typically `~/.claude/skills/`.
2. Copy (or symlink) the `polar-coach/` directory into that skills directory:

   ```bash
   cp -r skill/polar-coach ~/.claude/skills/
   ```

3. Restart Claude. The skill is auto-discovered and active for all conversations.

### Option 2: Claude.ai project file upload

This method works for Claude.ai conversations via the Projects feature:

1. Open your Claude.ai project (or create one for training management).
2. Upload `skill/polar-coach/SKILL.md` as a project file.
3. The skill becomes active for all chats in that project — no restart needed.

Both methods give Claude identical guidance and tool-calling behavior.

---

## Connecting Claude to your polar-flow-mcp server

The skill tells Claude *what* tools are available and *how* to use them. Separately, you
must configure your Claude client to connect to your polar-flow-mcp MCP server:

- **Claude Code / Claude Desktop MCP settings:** Add your server URL
  (`https://<your-host>/mcp`) as an MCP server endpoint. The server uses StreamableHTTP
  transport — no WebSocket or SSE needed.
- The MCP connection goes through your reverse proxy, which injects the identity header
  and proxy secret automatically.

See [Reference: HTTP Endpoints](reference/http-endpoints.md) for details on the `/mcp`
endpoint.
