# polar-coach

A Claude skill for managing Polar Flow training targets via the polar-flow-mcp server.
With this skill installed, you can ask Claude in plain language to create, list, or
delete workouts in Polar Flow — no UI navigation required.

## When to use this skill

Activate this skill when the user's message involves any of:

- Polar Flow, Polar watch, or Polar account
- creating, scheduling, planning, or modifying a training session, workout, run, or interval session
- terms like "intervals", "tempo run", "long run", "threshold", "vo2max", "easy run"
- training plans (5k, 10k, half marathon, marathon, base building, taper)
- listing upcoming planned workouts or removing one that was created in error

If the message is about *analyzing past sessions, viewing performance data, or syncing devices*,
this skill does NOT apply — see the "When NOT to call tools" section below.

## Tools

Eight tools are exposed by the polar-flow-mcp server. Use the **exact** names below.

| Tool | Purpose |
|------|---------|
| `get_user_info` | Confirm which Polar account the server is acting as. Returns identity (email, name, country). Call this FIRST if you're unsure whether the server is configured. |
| `create_training_target` | Create a single scheduled workout in Polar Flow. Accepts a flat phases array (warmup, one or more repeat blocks, cooldown). |
| `list_training_targets` | List upcoming training targets in a date range. Defaults to today through +30 days. |
| `delete_training_target` | Delete a training target by its numeric `target_id`. |
| `get_calendar_events` | Raw calendar events (targets, exercises, etc.) in a date range. Use only if `list_training_targets` doesn't give what you need. |
| `list_training_sessions` | Completed training sessions in a date range. Use for "how did my last run go?" / "what have I done this week?" |
| `get_training_session_summary` | Summary of one completed session by id (duration, distance, calories, HR averages). |
| `get_training_session_details` | Lap- and sample-level detail of one completed session. Use for "split times" / "what were my paces". |

### Tool availability check

Before calling any of these tools, confirm the polar-flow-mcp server is connected (the
tool should appear in your available MCP tool list). If it does not:

1. Tell the user the polar-flow-mcp server is not connected.
2. Point them at the project README and deployment docs (`docs/deployment/`) for setup.
3. Do NOT fabricate tool calls or simulate results.

## HR zones and intensity vocabulary

Polar Flow uses heart-rate zones Z1-Z5. The `intensity` field on a repeat phase accepts
either a `label` (preferred — coaching vocabulary) or an `hr_zone` integer (escape hatch).

| Label | Zone | Coaching meaning |
|-------|------|------------------|
| `easy` | Z1 | Recovery pace. Conversational, fully aerobic. Use for shakeout runs, warm-ups, recovery jogs. |
| `aerobic` | Z2 | Easy aerobic / base. Comfortable, sustainable for long runs. Bulk of weekly volume. |
| `tempo` | Z3 | Steady-state, comfortably hard. Marathon to half-marathon effort. |
| `threshold` | Z4 | Lactate threshold. Hard, sustainable for ~30-60 min. Classic 1km repeats, cruise intervals. |
| `vo2max` | Z5 | Very hard. Short repeats (1-5 min) at near-maximal aerobic effort. |

Always prefer labels over zone numbers in conversation with the user, then pass the label
through to `create_training_target`. Use `hr_zone` numerically only if the user explicitly
gives one ("zone 4", "Z3").

## Defaults

When the user does not specify, use these defaults — they match what Polar Flow expects
and what the polar-flow-mcp server applies:

- **Warmup**: 10 min (`duration_s: 600`). Always include unless the user says "no warmup".
- **Cooldown**: 5 min (`duration_s: 300`). Always include unless the user says "no cooldown".
- **Scheduled time**: 18:00. Pass `time: "18:00"` or omit (handler defaults to 18:00).
- **Sport**: RUNNING. Omit unless the user names a different sport.

## Worked examples

### Example 1: 5x1km threshold session

User says: *"create a 5x1km threshold session with 2 minute recovery for next Thursday"*.

You should:

1. Resolve "next Thursday" to an ISO date based on the user's reference date.
2. Use the default warmup (10 min) and cooldown (5 min) since the user did not specify.
3. Map "threshold" -> `label: "threshold"` (handler maps to Z4 internally).
4. Call `create_training_target` with:

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

The response includes the new target's `ID`. Tell the user the session was created and
include the ID (they can use it to delete the session if it was wrong).

### Example 2: Iterative marathon plan construction

User says: *"build me a 12-week marathon plan starting in three weeks"*.

A marathon plan is many sessions — do NOT create all 36+ sessions silently. Ask the user
first, then create per-week (or per-block) so they can review and steer.

1. Ask first: peak weekly mileage? long-run day? quality-session day(s)? rest day(s)?
2. Once you have a structure, propose week 1 in plain prose, ask for confirmation.
3. On confirmation, call `create_training_target` once per session in week 1.
4. Repeat for each subsequent block (e.g., 4-week phases).
5. After each block, summarize what was created and confirm before continuing.

This iterative approach keeps the user in control and avoids polluting their Polar Flow
calendar with sessions they did not explicitly approve.

### Example 3: Mixed-zone workout (two distinct repeat blocks)

The tool supports multiple repeat blocks in a single workout. Example for
"3x1km at threshold, then 2x500m at vo2max":

```json
{
  "name": "Threshold + VO2max ladder",
  "date": "2026-05-22",
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

### Example 4: Listing and deleting

User says: *"what do I have planned this week?"*

Call `list_training_targets` with `from_date` set to today and `to_date` set to the end
of the current week (or omit both to use the default today..+30 days). Format the result
for the user.

User says: *"delete the one on Friday — I'm not going to do it"*.

1. Use `list_training_targets` to find the matching session and its `target_id`.
2. Confirm with the user which session they mean (show name + date).
3. Call `delete_training_target` with that `target_id`.
4. If the response says "not found", the session was already deleted — tell the user
   plainly; this is not a failure.

## When NOT to call tools

Do NOT call polar-flow-mcp tools when the user is asking about:

- **Past sessions** — analytics, splits, heart-rate review, sync status. The polar-flow-mcp
  server writes training targets only; it does not read historical activity data.
- **Pace targets in min/km or min/mile** — v1 supports HR zones only. If the user asks for
  pace-based intensity, explain the limitation and offer the closest HR zone equivalent.
- **Power targets in watts** — v1 does not support power. Same limitation; offer HR zones.
- **Polar account creation, sync, or device pairing** — outside this skill's scope. Refer
  to the official Polar Flow app.
- **General training advice that does not need to create a workout** — answer normally
  without invoking tools.

## Safe degradation

The polar-flow-mcp server is self-hosted and may not always be reachable.

- If the tools above are not in your available MCP tool list, the server is not
  connected. Tell the user this clearly and link them to the deployment docs in the
  project repository (start with `README.md`).
- If a tool call returns an error mentioning "Polar credentials rejected" or
  "no Polar credentials configured", the server's `POLAR_EMAIL` / `POLAR_PASSWORD`
  environment is wrong or missing. Tell the user to check the polar-flow-mcp
  server's `.env` (or its container environment) and restart it.
- If a tool call returns an error containing "status 4xx" or "status 5xx", surface the
  message to the user verbatim and ask them to check their server logs. Do not retry
  silently — the Polar API may be rejecting a malformed request that needs a fix on the
  server side.
- Do NOT fabricate tool call results or simulate API responses when the server is
  unreachable. Always surface failures to the user honestly.

## Installation

There are two ways to install this skill:

### Option 1: Claude Desktop or Claude Code (local skills directory)

1. Locate your Claude skills directory. On Claude Code this is typically
   `~/.claude/skills/`; on Claude Desktop see the official skills documentation for
   your platform's path.
2. Copy the `polar-coach/` directory (containing this `SKILL.md`) into that directory.
3. Restart Claude. The skill is auto-discovered.

### Option 2: Claude.ai (project file upload)

1. Open your Claude.ai project (or create one).
2. Upload this `SKILL.md` file as a project file.
3. The skill becomes active for all chats in that project.

Both paths give Claude the same vocabulary and tool-calling guidance.

---

## Quick reference card

| Want to... | Tool | Key args |
|------------|------|----------|
| Check Polar account is linked | `get_user_info` | none |
| Create a single workout | `create_training_target` | `name`, `date`, `phases[]` |
| List upcoming workouts | `list_training_targets` | `from_date`, `to_date` (optional) |
| Remove a workout | `delete_training_target` | `target_id` |

Intensity labels (always prefer labels; use `hr_zone` integer only when user gives a zone number):
`easy` (Z1) · `aerobic` (Z2) · `tempo` (Z3) · `threshold` (Z4) · `vo2max` (Z5)
