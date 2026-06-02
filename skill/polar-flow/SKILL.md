---
name: polar-flow
description: >-
  Drive the polar-flow-mcp tools to read and write a Polar Flow training diary:
  list/create/update/delete planned training targets, read completed sessions
  and lap detail, pull weekly and progress summaries, and log manual sessions.
  Use whenever a task calls a polar-flow-mcp tool (get_user_info,
  list/create/update/delete/get_training_target, get_calendar_events,
  get_calendar_week_summary, list_training_sessions,
  get_training_session_summary/details, get_progress_summary,
  create_training_session) or asks how to schedule, edit, or read workouts in
  Polar Flow. Covers tool mechanics, parameter shapes, units, and error
  handling — not coaching methodology.
---

# polar-flow

The interface layer for the **polar-flow-mcp** server. This skill explains how
to call the Polar Flow tools correctly: which tool does what, the exact
parameter shapes, the units and conventions the API expects, and how to handle
failures. Get the mechanics right and the data round-trips cleanly.

## Scope — read this first

This skill is **only** about driving the polar-flow-mcp tools. It does **not**
decide *what* training to prescribe.

- **In scope:** picking the right tool, building valid parameters (phases,
  units, dates), full-replace update semantics, reading history/summaries,
  interpreting tool errors, safe degradation.
- **Out of scope:** periodization, weekly load progression, 80/20, taper,
  multi-week race plans, warmup/cooldown prescription, readiness/weather
  judgement. That is **coaching methodology** — handled by the separate,
  brand-agnostic sport-coaching skill. When a request needs a training
  *decision*, defer to that skill; this skill executes the decision against
  Polar Flow.

If the polar-flow-mcp tools are not in your available tool list, the server
isn't connected — see `reference/troubleshooting.md` before doing anything else.

## The 14 tools at a glance

Use the **exact** names below. Reads are safe to call freely; writes change the
user's diary.

| Tool | R/W | One-liner |
|------|-----|-----------|
| `get_user_info` | R | Identity + country for the linked account. Call once to confirm setup. |
| `list_sports` | R | Full Polar sport-id → name catalogue (the `sport_id` values for targets/sessions). |
| `list_training_targets` | R | Planned workouts in a date range (id, title, time). |
| `get_training_target` | R | Full normalized body of one target — read before editing. |
| `create_training_target` | W | Create a planned workout. Returns the new numeric id. |
| `update_training_target` | W | **Full-replace** edit of one target. |
| `delete_training_target` | W | Permanently delete one target. Irreversible. |
| `get_calendar_events` | R | Raw diary events (targets + sessions). Low-level fallback. |
| `get_calendar_week_summary` | R | Per-ISO-week totals strip (≤45-day range). |
| `list_training_sessions` | R | Completed sessions in a date range. |
| `get_training_session_summary` | R | Totals for one completed session. |
| `get_training_session_details` | R | Laps + samples for one session. Heavy payload. |
| `get_progress_summary` | R | Aggregated totals/distributions over a range. |
| `create_training_session` | W | ⚠ Log a *completed* off-watch session. **User-initiated only.** |

Full per-tool parameter reference: **`reference/tools.md`**.

## Critical rules

1. **`update_training_target` is a full replace, not a patch.** Always
   `get_training_target` first, modify the returned body, then send the
   complete result. Anything you omit is cleared. See
   `reference/training-targets.md`.
2. **`create_training_session` writes real history.** It counts toward weekly
   volume, progress summaries, and Polar's training-load model. Call it **only**
   when the user explicitly asks to log a session they actually did off-watch
   ("log the 5k I ran yesterday"). Never to fabricate or backfill data. See
   `reference/logging-sessions.md`.
3. **Confirm before any destructive or write action** (`delete_*`, `create_*`,
   `update_*`) unless the user already gave a direct instruction.
4. **Units are fixed:** distances in **metres**, durations in **seconds**,
   dates in **ISO YYYY-MM-DD**, times in **24h HH:MM**, heart rate in **bpm**,
   speed in **km/h**. The tools never take km or HH:MM:SS. See
   `reference/training-targets.md`.
5. **Never fabricate a tool result.** If a tool fails or is missing, surface it
   honestly — see `reference/troubleshooting.md`.

## Reference files (read on demand)

| Read when you need to… | File |
|------------------------|------|
| Look up a tool's exact parameters and return shape | `reference/tools.md` |
| Build or edit a planned workout (phases, units, examples) | `reference/training-targets.md` |
| Read history, summaries, calendar, or progress | `reference/reading-data.md` |
| Log a completed off-watch session | `reference/logging-sessions.md` |
| Diagnose a tool error or set up the skill | `reference/troubleshooting.md` |

## Intensity shorthand

A `repeat` phase's `intensity` accepts a `label` (mapped to a Polar HR zone) or
an explicit `hr_zone` (1–5). The mapping is a fact of the API, not a coaching
opinion:

| Label | HR zone |
|-------|---------|
| `easy` | 1–2 |
| `aerobic` | 2 |
| `tempo` | 3 |
| `threshold` | 4 |
| `vo2max` | 5 |

`hr_zone` wins if both are given. Pass `label` through verbatim; use `hr_zone`
only when the user names a number ("zone 4"). What each zone *means for
training* is the coaching skill's job.
