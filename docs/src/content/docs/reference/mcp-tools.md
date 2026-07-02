---
title: MCP tools
description: Every MCP tool polar-flow-mcp exposes, with arguments, defaults, and response shapes.
sidebar:
  order: 1
---

polar-flow-mcp exposes fourteen tools backed by the
[ogen](https://github.com/ogen-go/ogen)-generated client in `internal/flow/`.
All tools act as the single Polar Flow account configured via `POLAR_EMAIL` /
`POLAR_PASSWORD`.

Authentication failures (`401 NotAuthenticated`) trigger a transparent
silent-refresh-then-retry — callers will not see a 401 unless the credentials
themselves are bad.

## `get_user_info`

Returns the identity of the linked Polar Flow account.

**Arguments:** none.

**Response:** JSON object with `id`, `email`, `first_name`, `last_name`,
`country`.

## `list_sports`

Returns the full Polar sport catalogue — the source of valid `sport_id` values.

**Arguments:** none.

**Response:** JSON object mapping numeric sport id (string key) to its name
constant, e.g. `{"1": "RUNNING", "2": "CYCLING", "23": "SWIMMING", ...}`. The
catalogue is a moving snapshot (Polar adds sports over time), so re-fetch rather
than hard-coding ids.

## `create_training_target`

Schedule a training target (a planned workout) in Polar Flow. There are two
shapes:

- **VOLUME target** (no `phases`): a single open-goal block — set `duration_s`
  **or** `distance_m` at the top level. Use for easy/general runs.
- **PHASED target** (with `phases`): omit the top-level `duration_s`/`distance_m`
  and provide an ordered list of `warmup` / `repeat` / `cooldown` blocks.

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | yes | Display name (shown in the diary). |
| `date` | string | yes | ISO 8601 `YYYY-MM-DD`. The server applies the account's timezone. |
| `time` | string | no | `HH:MM` (24h). Default `18:00`. |
| `sport_id` | integer | no | Polar sport ID. Default `1` (running); e.g. `2` cycling, `23` swimming, `15` strength, `11` hiking, `68` triathlon. Use `list_sports` for the full mapping. |
| `description` | string | no | Free-text notes. |
| `duration_s` | integer | conditional | VOLUME total duration in seconds (e.g. `2100` = 35 min). Required when `phases` is omitted and `distance_m` is not set. Ignored when `phases` is provided. |
| `distance_m` | integer | conditional | VOLUME total distance in metres (e.g. `10000` = 10 km). Required when `phases` is omitted and `duration_s` is not set. Ignored when `phases` is provided. |
| `phases` | array | no | Ordered phase list for a PHASED target. See below. |

A VOLUME target with neither `duration_s` nor `distance_m` is rejected
(`VOLUME targets (no phases) require duration_s or distance_m`).

### Phase shape

Each entry in `phases` is an object with `type` ∈ {`warmup`, `repeat`,
`cooldown`}:

```json
{ "type": "warmup",   "duration_s": 600 }
{ "type": "repeat",
  "reps": 5,
  "goal":      { "distance_m": 1000 },
  "intensity": { "label": "threshold" },
  "recovery":  { "duration_s": 90 }
}
{ "type": "cooldown", "duration_s": 600 }
```

- `warmup` / `cooldown` require `duration_s` (seconds, > 0) and run at easy
  intensity.
- `repeat` is an interval block repeated `reps` times (**`reps` must be ≥ 2**).
  It needs a `goal` (exactly one of `distance_m` metres **or** `duration_s`
  seconds), an optional `intensity`, and an optional `recovery` (`duration_s`,
  inserted between reps). A single continuous effort with no intervals is not a
  `repeat` — use a VOLUME target instead.
- `intensity` accepts either an `hr_zone` integer (1–5) or a `label`
  (`easy`→Z1–2, `aerobic`→Z2, `tempo`→Z3, `threshold`→Z4, `vo2max`→Z5). If both
  are given, `hr_zone` wins.
- Each phase (and a `recovery`) accepts an optional `name`, persisted verbatim
  by Polar; it defaults to a type-derived label (`Warm-up`, `Work`, `Recovery`,
  `Cool-down`).

**Response:** human-readable confirmation including the new target's numeric
ID.

## `list_training_targets`

List training targets in a date range.

| Argument | Type | Default |
|----------|------|---------|
| `from_date` | `YYYY-MM-DD` | today (UTC) |
| `to_date` | `YYYY-MM-DD` | today + 30 days |

Implemented as a filter over `getCalendarEvents` for entries with
`type == TRAININGTARGET`.

**Response:** text list of `<id>: <title> (<start>)` lines.

## `delete_training_target`

Delete a target by numeric ID.

| Argument | Type | Required |
|----------|------|----------|
| `target_id` | integer | yes |

**Response:** `Deleted target <id>.` or `No target with id <id>.`

## `get_training_target`

Return the full server-normalized view of a single target. Use this before
`update_training_target` to read the current shape, modify it, then write back.

| Argument | Type | Required |
|----------|------|----------|
| `target_id` | integer | yes |

**Response:** JSON `TrainingTargetCreate` object (same shape as the create
payload, plus server-assigned ids and rolled-up totals).

## `update_training_target`

Full-replace edit of a target by ID. The argument shape matches
`create_training_target` (same VOLUME/PHASED shapes, units, and `phases`
schema) plus a required `target_id`. The server overwrites the target with the
supplied body — there are no patch semantics, so always read the target with
`get_training_target` first if you only want to change one field. Anything you
omit is cleared.

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `target_id` | integer | yes | Numeric ID of the target to edit. |
| `name` | string | yes | Display name shown in the diary. |
| `date` | string | yes | ISO 8601 `YYYY-MM-DD`. |

All other arguments (`time`, `sport_id`, `description`, `duration_s`,
`distance_m`, `phases`) match `create_training_target`. You do not need to
manage server-side ids — the tool reads the live target and carries its
exercise-target id over so the edit lands on the existing target.

## `get_calendar_events`

Raw calendar events (training targets, completed exercises, etc.) in a date
range.

| Argument | Type | Default |
|----------|------|---------|
| `from_date` | `YYYY-MM-DD` | today |
| `to_date` | `YYYY-MM-DD` | today + 30 days |

**Response:** JSON array of `CalendarEvent` objects (see the spec at
[polar-openapi-maker](https://github.com/lmgarret/polar-openapi-maker) for
the full shape).

## `get_calendar_week_summary`

Returns one summary entry per ISO week intersecting `[from, to]` — the
right-hand "week totals" strip in the Polar Flow diary.

| Argument | Type | Default |
|----------|------|---------|
| `from_date` | `YYYY-MM-DD` | today − 28 days |
| `to_date` | `YYYY-MM-DD` | today |

Range is capped at **45 days** by the server. Larger ranges return a 400
that's surfaced as a tool error.

**Response:** JSON array of week-summary objects. Item shape is currently
TBD upstream (test accounts return `[]`); the tool surfaces the raw JSON so
callers can adapt as the spec firms up.

## `get_progress_summary`

Aggregated training totals (sessions, distance, duration, calories, ascent /
descent, zone time, sport distribution, training-benefit distribution) for
the supplied range.

| Argument | Type | Default |
|----------|------|---------|
| `from_date` | `YYYY-MM-DD` | today − 90 days |
| `to_date` | `YYYY-MM-DD` | today |
| `group` | string | `MONTH` — time-bucket granularity, **not** a sport filter (likely also `DAY`/`WEEK`/`YEAR`) |
| `time_frame` | string | `3m` — also accepts `6w`, `1y` |

**Response:** JSON `ProgressViewSummary` object — sport distributions,
training-benefit distributions, HR-zone totals, fit/fat-zone totals.

## `create_training_session`

Log a manually-entered completed training session via `POST /api/training/create`
— the endpoint behind the "Manual training result" form (`/exercises/add`) in
the Polar Flow web UI.

:::danger[Writes real data]
The created session counts toward weekly volume, progress summaries, and Polar's
training-load model. Intended for user-initiated logging of sessions that
weren't recorded on a watch — not for synthesizing test data on a production
account. Coach skills should only call this when the user explicitly asks to log
a session.
:::

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | yes | Display name shown in the diary. |
| `date` | `YYYY-MM-DD` | yes | Date the session happened. |
| `time` | `HH:MM` | no | Local start time. Default `18:00`. |
| `duration_s` | integer | yes | Duration in seconds. |
| `distance_m` | integer | no | Distance in metres. Default `0`. |
| `kcal` | integer | no | Kilocalories burned. Default `0`. |
| `hr_avg` | integer | no | Average HR (bpm). Omit / `0` for unset. |
| `hr_max` | integer | no | Max HR (bpm). Omit / `0` for unset. |
| `speed_kmh` | number | no | Average speed (km/h). Omit / `0` for unset. |
| `sport_id` | integer | no | Polar sport ID. Default `1` (running); e.g. `2` cycling, `23` swimming. Use `list_sports` for the full mapping. |
| `note` | string | no | Free-text note. |

**Response:** confirmation string. The endpoint returns an empty body — Polar
does not surface the new session id; call `list_training_sessions` afterwards
if you need it.

## `list_training_sessions`

List **completed** training sessions in a date range.

| Argument | Type | Default |
|----------|------|---------|
| `from_date` | `YYYY-MM-DD` | today − 30 days |
| `to_date` | `YYYY-MM-DD` | today |

The tool resolves the numeric user ID by calling `get_user_info` internally,
so no `user_id` parameter is needed.

**Response:** JSON array of `TrainingSessionSummary` objects.

## `get_training_session_summary`

Summary view of a completed session — duration, distance, calories, HR
averages, sport, etc.

| Argument | Type | Required |
|----------|------|----------|
| `session_id` | integer | yes |

## `get_training_session_details`

Lap- and sample-level details for a completed session. Use this when the
user asks about pace splits, HR zone time, or per-lap stats.

| Argument | Type | Required |
|----------|------|----------|
| `session_id` | integer | yes |

## Behaviour notes

- **Time zones.** `create_training_target` sends a local ISO datetime
  (`YYYY-MM-DDTHH:MM` with no offset). The Polar server applies the user's
  configured timezone. Make sure your test account's timezone matches your
  intent.
- **Sport IDs.** `sport_id` defaults to `1` (running). For cycling use `2`,
  swimming `23`, strength `15`; call the `list_sports` tool for the full
  id → name mapping.
- **Failure modes.** On transport failure (network, 5xx) the tool returns
  the underlying error as a tool-result error. The MCP client (Claude) sees
  the error and can decide whether to retry.
