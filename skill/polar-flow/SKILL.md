---
name: polar-flow
description: >-
  Drive the polar-flow-mcp tools to read and write a Polar Flow training diary:
  plan, edit and delete training targets; read completed sessions, lap detail,
  weekly and progress summaries; log, edit or delete sessions; read the
  athlete's heart-rate / pace / power zones and edit them per sport profile;
  read 24/7 daily activity and sleep; manage favorites (workout
  templates), schedule them on dates, and import or read GPS routes. Use
  whenever a task calls a polar-flow-mcp tool (e.g. create_training_target,
  list_training_sessions, get_training_zones, update_training_zones,
  get_daily_activity, get_sleep, create_favorite,
  schedule_favorite, import_route, edit_training_session) or asks how to
  schedule, template, edit or read workouts, zones or routes in Polar Flow.
  Covers tool mechanics, parameter shapes, units and error handling — not
  coaching methodology.
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

## The 33 tools at a glance

Use the **exact** names below. Reads are safe to call freely; writes change the
user's diary.

| Tool | R/W | One-liner |
|------|-----|-----------|
| **Account & reference** | | |
| `get_user_info` | R | Identity + country for the linked account. Call once to confirm setup. |
| `list_sports` | R | Full Polar sport-id → name catalogue (the `sport_id` values everywhere). |
| `get_training_zones` | R | What HR / speed (pace) / power zones 1–5 mean in bpm, km/h (min/km) and W for a sport. |
| **Sport profiles & zones (writes)** | | |
| `create_sport_profile` | W | Create a sport's profile with Polar's default zones (idempotent). |
| `update_training_zones` | W | Set a sport's HR / speed / power zones by hand, or reset them to defaults. |
| `delete_sport_profile` | W | Delete a sport's profile. Polar keeps the last one. |
| **Planned workouts (targets)** | | |
| `list_training_targets` | R | Planned workouts in a date range (id, title, time). |
| `get_training_target` | R | Full normalized body of one target — read before editing. |
| `create_training_target` | W | Create a planned workout. Returns the new numeric id. |
| `update_training_target` | W | **Full-replace** edit of one target. |
| `delete_training_target` | W | Permanently delete one target. Irreversible. |
| **Diary & history** | | |
| `get_calendar_events` | R | Raw diary events (targets + sessions). Low-level fallback. |
| `get_calendar_week_summary` | R | Per-ISO-week totals strip (≤45-day range). |
| `list_training_sessions` | R | Completed sessions in a date range. |
| `get_training_session_summary` | R | Totals for one completed session. |
| `get_training_session_details` | R | Laps + samples for one session. Heavy payload. |
| `get_progress_summary` | R | Aggregated totals/distributions over a range. |
| **24/7 activity & sleep** | | |
| `get_daily_activity` | R | Steps, active time, kcal, intensity bands, day/night HR per day (≤ 31 days). |
| `get_sleep` | R | Recorded nights: sleep times, duration, Sleep Score, stages (≤ 365 days). |
| **Completed sessions (writes)** | | |
| `create_training_session` | W | ⚠ Log a *completed* off-watch session. **User-initiated only.** |
| `edit_training_session` | W | Change note/feeling (any session); other fields on manual sessions only. |
| `delete_training_session` | W | ⚠ Permanently delete a completed session. **User-initiated only.** |
| **Favorites (templates) & routes** | | |
| `list_favorites` | R | Templates and routes, each with `favorite_id` + `exercise_target_id`. |
| `get_favorite` | R | One favorite's full body — read before `update_favorite`. |
| `create_favorite` | W | New date-less template (duration, distance or phases goal). |
| `update_favorite` | W | **Full-replace** edit of a single-sport template. |
| `rename_favorite` | W | Rename a template or route; nothing else changes. |
| `set_favorite_sport` | W | Change a template's or route's sport (validated, verified). |
| `delete_favorite` | W | Permanently delete a template or route. Irreversible. |
| `save_target_as_favorite` | W | Copy a scheduled target into a new template. |
| `schedule_favorite` | W | Put a template in the diary on a date → new target. |
| `import_route` | W | Import a GPX/TCX file as a route favorite. |
| `get_route` | R | A route's waypoints (down-sampled). |

Full per-tool parameter reference: **`reference/tools.md`**.

## Critical rules

1. **`update_training_target` and `update_favorite` replace everything, not
   just the fields you pass.** Always read first (`get_training_target` /
   `get_favorite`), change the returned body, then send the complete result.
   Anything you leave out is cleared. To change only a favorite's name or
   sport, use `rename_favorite` / `set_favorite_sport` instead. See
   `reference/training-targets.md` and `reference/favorites-and-routes.md`.
2. **Session history is real data.** `create_training_session`,
   `edit_training_session` and `delete_training_session` change the history
   that weekly volume, progress summaries and Polar's training-load model are
   built from. Call them **only** when the user explicitly asks about a session
   they actually did ("log the 5k I ran yesterday", "that run was 10.2 km, fix
   it", "delete yesterday's duplicate"). Never fabricate, backfill or "tidy up"
   on your own. See `reference/logging-sessions.md`.
3. **Confirm before any destructive or write action** (`delete_*`, `create_*`,
   `update_*`, `edit_*`, `import_*`, `schedule_*`), unless the user already
   gave a direct instruction. On hosts that support elicitation,
   `create_training_session`, `delete_training_target`,
   `delete_training_session` and `delete_favorite` also ask the user
   themselves. A result saying the user did not confirm means nothing was
   written or deleted. Report that back and don't retry around it. Not every
   host can ask the user, so this is a backstop for your own confirmation,
   not a replacement for it.
4. **Units are fixed:** distances in **metres**, durations in **seconds**,
   dates in **ISO YYYY-MM-DD**, times in **24h HH:MM**, heart rate in **bpm**,
   speed in **km/h**, pace in **seconds per km**, power in **watts**. The tools
   never take km or HH:MM:SS. See `reference/training-targets.md`.
5. **Zones are indices, and the numbers behind them are per athlete.** Before
   turning a pace, wattage or heart rate into `hr_zone` / `speed_zone` /
   `power_zone`, call `get_training_zones` with the workout's `sport_id`.
   Don't guess from generic percentages.
6. **Never fabricate a tool result.** If a tool fails or is missing, surface it
   honestly — see `reference/troubleshooting.md`.

## Common workflows

- **"Plan intervals at 4:30/km"** → `get_training_zones(sport_id=1)` → pick
  the speed zone whose pace range contains 4:30 → `create_training_target`
  with `intensity: {"speed_zone": N}`.
- **"Move Thursday's session to Friday"** → `list_training_targets` →
  `get_training_target` → `update_training_target` with the full body and the
  new date.
- **"Save this as a template" / "put my usual long run on Sunday"** →
  `save_target_as_favorite`, or `list_favorites` → `schedule_favorite`.
- **"I ran 10.2 km, not 10"** → `list_training_sessions` →
  `edit_training_session(session_id, distance_m=10200)`. This works on manual
  sessions only. On a watch-recorded session only the note and feeling can
  change.
- **"Here's a GPX of my route"** → `import_route(content=…)`; to show it later,
  `get_route`.

## Reference files (read on demand)

| Read when you need to… | File |
|------------------------|------|
| Look up a tool's exact parameters and return shape | `reference/tools.md` |
| Build or edit a planned workout (phases, units, examples) | `reference/training-targets.md` |
| Read history, summaries, calendar, or progress | `reference/reading-data.md` |
| Log, correct or delete a completed session | `reference/logging-sessions.md` |
| Work with favorites (templates), scheduling, or GPS routes | `reference/favorites-and-routes.md` |
| Diagnose a tool error or set up the skill | `reference/troubleshooting.md` |

## Intensity shorthand

Any phase's (`warmup` / `cooldown` / `repeat`) `intensity` accepts a `label`
(mapped to a Polar HR zone), an explicit `hr_zone` (1–5), a `power_zone`
(1–5), or a `speed_zone` (1–5) — it's not repeat-only, so a single continuous
zoned block (duration goal) is a lone `warmup`/`cooldown` phase with a custom
`name`, not an artificial `repeat`. The mapping is a fact of the API, not a
coaching opinion:

| Label | HR zone |
|-------|---------|
| `easy` | 1–2 |
| `aerobic` | 2 |
| `tempo` | 3 |
| `threshold` | 4 |
| `vo2max` | 5 |

Precedence when more than one is given: `hr_zone` > `power_zone` >
`speed_zone` > `label`. Pass `label` through verbatim; use `hr_zone` only when
the user names a number ("zone 4"). What each zone *means for training* is
the coaching skill's job.

`power_zone` and `speed_zone` are the same kind of value as `hr_zone` — a
Polar zone **index** 1–5, not a raw watt or km/h (or min/km) number. The
API has no field for a literal physical threshold on a phase; each zone's
actual range is computed server-side from the athlete's sport profile.
`get_training_zones` (with the workout's `sport_id`) returns those ranges: if
someone gives you a wattage, pace or heart rate, read the zones and pick the
zone whose `[min, max)` range contains it. `power_zone` also needs a power-capable
sport (e.g. cycling) to be meaningful. See `reference/training-targets.md`.
