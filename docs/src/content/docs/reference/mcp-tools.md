---
title: MCP tools
description: Every MCP tool polar-flow-mcp exposes, with arguments, defaults, and response shapes.
sidebar:
  order: 1
---

polar-flow-mcp exposes thirty-three tools backed by the
[ogen](https://github.com/ogen-go/ogen)-generated client in `internal/flow/`.
All tools act as the single Polar Flow account configured via `POLAR_EMAIL` /
`POLAR_PASSWORD`.

Authentication failures (`401 NotAuthenticated`) trigger a transparent
silent-refresh-then-retry — callers will not see a 401 unless the credentials
themselves are bad.

## User confirmation on writes

Five tools touch real diary data and ask the user to confirm before they run:
[`create_training_session`](#create_training_session), which writes a session
that counts toward Polar's training-load model, and the four irreversible
deletes — [`delete_training_target`](#delete_training_target),
[`delete_training_session`](#delete_training_session),
[`delete_favorite`](#delete_favorite) and
[`delete_sport_profile`](#delete_sport_profile).

The mechanism is an MCP [elicitation](https://modelcontextprotocol.io/): the
tool answers the first call with "I need confirmation", the host puts the
question to the user, and then retries the same call with the answer attached.
On protocol `2026-07-28` and later this is a
[multi round-trip request](https://modelcontextprotocol.io/); on earlier
versions the server issues the `elicitation/create` those clients understand.
Either way the handler is the same code.

Each delete prompt names what it is about to remove (`Permanently delete
the training target "5x1km Threshold" scheduled for 2026-06-02T09:00 (id
7286431)?`, `… the training session "Easy 5km" on 2026-09-20T08:00:00 (30 min)
(id 8429796771)? …`), so the ask costs one extra read on the first leg only.

If the user declines or cancels, the tool returns a plain text result saying so
and nothing is written — that is a normal outcome, not a tool error to retry
around.

:::caution[Not every host can be asked]
Confirmation requires the connected client to declare the `elicitation`
capability. **Against a client that does not, the gate is skipped and the call
proceeds unconfirmed** — the tool descriptions remain the only thing keeping a
coach model from firing these unprompted. The gate is skipped deliberately:
asking a client that cannot answer fails the entire tool call instead of
protecting anything.
:::

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

## `get_training_zones`

Returns the athlete's **training zones** — what `hr_zone`, `speed_zone` and
`power_zone` 1–5 mean in bpm, km/h (with min/km pace) and watts. Zones are per
sport. Call it before planning a zoned workout to pick zone numbers from a
target pace, wattage or heart rate.

| Argument | Type | Required | Notes |
|---|---|---|---|
| `sport_id` | integer | no | Polar sport ID. With it: that sport's zones — from the stored sport profile if the account has one (`source: "profile"`), otherwise Polar's defaults, computed without saving anything (`source: "default"`). Without it: every stored sport profile (possibly none). |

**Response:** a text summary plus a `training_zones` payload:

```json
{"type": "training_zones", "sports": [{
  "sport_id": 1, "sport_name": "RUNNING", "sport_category": "run", "source": "profile",
  "heart_rate": {"setting": "default", "zones": [{"zone": 1, "min_bpm": 95, "max_bpm": 114}, …]},
  "speed": {"setting": "default", "method": "mas_based", "speed_view": "pace", "zones": [
    {"zone": 1, "min_kmh": 9.45, "max_kmh": 12.03, "slowest_pace_s_per_km": 381, "fastest_pace_s_per_km": 299}, …,
    {"zone": 5, "min_kmh": 19.76, "max_kmh": null, "slowest_pace_s_per_km": 182, "fastest_pace_s_per_km": null}]},
  "power": {"setting": "default", "method": "map_based", "zones": [… {"zone": 5, "min_w": 464, "max_w": null}]},
  "thresholds": {"mas_kmh": 17.18, "mas_source": "estimated", "map_w": 403, "map_source": "estimated"}
}]}
```

- Each zone is the half-open range `[min, max)`; zones are contiguous. The top
  speed and power zones are open-ended (`max_kmh` / `max_w` null — Flow's
  399 km/h and 2000 W ceilings are sentinels).
- `setting` is `default` (computed) or `free` (limits typed in by the user).
  `method` says how default speed/power zones were computed: `mas_based` /
  `map_based` (55/70/85/100/115 % of maximum aerobic speed / power, running),
  `ftp_based` (55/75/90/105/120 % of FTP, cycling),
  `sport_specific_predefined` (a fixed table — cycling 10/20/30/40/50 km/h).
  Default heart-rate zones are 50/60/70/80/90/100 % of max HR.
- A zone type the sport lacks is omitted (swimming and strength have
  heart-rate zones only).
- `thresholds` carries MAS (km/h), MAP and FTP (W) where they apply; `estimated`
  means Polar derived them from physical info rather than a test.

## Sport profiles and zone edits

A **sport profile** holds a sport's training zones and watch settings. Most
accounts store only a few; [`get_training_zones`](#get_training_zones) reports
Polar's computed defaults for the rest. A profile is keyed by its sport
(Polar's profile uuid is derived from the `sport_id`), so these tools take
`sport_id` — validated against [`list_sports`](#list_sports). Changes reach a
paired watch on its next sync.

### `create_sport_profile`

| Argument | Type | Required |
|----------|------|----------|
| `sport_id` | integer | yes |

Creates the profile with Polar's default settings and zones. Idempotent: when
the sport already has a profile nothing changes and the stored one is returned
(the text says which). **Response:** the same `training_zones` payload as
`get_training_zones`, plus a `notice` sentence.

### `update_training_zones`

| Argument | Type | Notes |
|----------|------|-------|
| `sport_id` | integer | required; the sport must have a stored profile |
| `heart_rate_bpm` | number[6] | whole bpm, 15–240, each zone ≥ 2 bpm |
| `speed_kmh` | number[5 or 6] | km/h, 1–399 — **always km/h**, even for sports shown as pace |
| `power_w` | number[5 or 6] | whole watts, 0–2000, each zone ≥ 2 W |
| `reset` | string[] | `heart_rate`, `speed`, `power` — back to Polar's computed defaults |

Each list is the ascending zone **boundaries**, Z1 lower first: zone *n* is
`[value n, value n+1)`, so zones are contiguous by construction. Give 5 speed or
power values to leave Z5 open-ended. Lists not mentioned keep their stored
limits; a list cannot be both set and reset. Every rule above is checked before
a request goes out (Flow's own 400 lists every violation in one long line).
Swimming and strength-type sports only have heart-rate zones.

```json
{"sport_id": 1, "heart_rate_bpm": [115, 134, 153, 172, 182, 192], "reset": ["speed"]}
```

**Response:** the zones read back after the save (`setting: "free"` on the
hand-entered lists), as a `training_zones` payload with a `notice`.

### `delete_sport_profile`

| Argument | Type | Required |
|----------|------|----------|
| `sport_id` | integer | yes |

Deletes the profile — its zones fall back to Polar's defaults. Reports
"nothing to delete" when the sport has none (Flow itself answers `200` either
way, so the tool checks first). Polar refuses to delete the account's last
profile. Asks the user to confirm first where the host supports it — see
[User confirmation on writes](#user-confirmation-on-writes).

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

- `warmup` / `cooldown` require `duration_s` (seconds, > 0) and take an
  optional `intensity` (no `distance_m`/`goal` — duration-goaled only). Omit
  `intensity` for open/easy intensity. A single continuous *zoned* effort with
  a duration goal and no interval structure is one `warmup`/`cooldown` phase
  with `intensity` set and a custom `name` — it does not need `repeat`.
- `repeat` is an interval block repeated `reps` times (**`reps` must be ≥ 2**).
  It needs a `goal` (exactly one of `distance_m` metres **or** `duration_s`
  seconds), an optional `intensity`, and an optional `recovery` (`duration_s`,
  inserted between reps). A single continuous *distance*-goaled effort still
  needs `repeat` (only `repeat` accepts `distance_m`); an unzoned effort with
  no structure at all is a VOLUME target instead.
- `intensity` accepts at most one of: an `hr_zone` integer (1–5), a `label`
  (`easy`→Z1–2, `aerobic`→Z2, `tempo`→Z3, `threshold`→Z4, `vo2max`→Z5), a
  `power_zone` integer (1–5), or a `speed_zone` integer (1–5). Precedence when
  several are given: `hr_zone` > `power_zone` > `speed_zone` > `label`.
  `power_zone` and `speed_zone` are Polar zone **indices**, not raw watts or
  km/h/min-per-km — Polar Flow computes each zone's physical range from the
  athlete's sport profile; [`get_training_zones`](#get_training_zones) returns
  those ranges. `power_zone` needs a power-capable sport
  (e.g. cycling, `sport_id` `2`).
- Each phase (and a `recovery`) accepts an optional `name`, persisted verbatim
  by Polar; it defaults to a type-derived label (`Warm-up`, `Work`, `Recovery`,
  `Cool-down`).
- Length limits, checked before anything is sent and counted as Polar counts
  them (UTF-16 code units — an emoji counts 2): `name` 1–45, `description`
  ≤ 500, each phase name ≤ 45. An over-long phase name matters most: Polar
  rejects it but still stores a hidden, half-created target on that time slot.
- Polar refuses two targets on the same minute; the tool then says which
  datetime is taken so you can pick another.

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

**Response:** `Deleted target <id>.` or `No target with id <id>.` (also for an
already-deleted id). Another account's id is an error. The target is read
first — Polar's own delete answers missing and foreign ids with the same
opaque 400.

Asks the user to confirm first where the host supports it — see
[User confirmation on writes](#user-confirmation-on-writes).

## `get_training_target`

Return the full server-normalized view of a single target. Use this before
`update_training_target` to read the current shape, modify it, then write back.

| Argument | Type | Required |
|----------|------|----------|
| `target_id` | integer | yes |

**Response:** JSON `TrainingTargetCreate` object (same shape as the create
payload, plus server-assigned ids and rolled-up totals). `description` reads back
as `null` when the target was created without one.

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

On success it reads the target back and returns the same server-normalized
view as `get_training_target` (name, datetime, and the rolled-up
exercise-target phases), so a follow-up `get_training_target` to confirm the
change landed is unnecessary.

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

**Response:** JSON array of week-summary objects — one per ISO week in range.
The element shape is currently TBD upstream: the objects come back empty
(`[{}, {}, …]`) even on accounts with recorded sessions, so the tool surfaces the
raw JSON as-is for callers to adapt as the spec firms up.

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

**Response:** canonical progress object — `number_of_sessions`,
`total_duration_s` (seconds, decoded from the wire's `StandardDuration`
object), `total_distance_m`, `total_kcal`, `total_ascent_m` / `total_descent_m`,
plus `from_date` / `to_date` echoing the range. The `sport_breakdown`,
`heart_rate_zones`, and `training_benefit_breakdown` lists are passed through
from the wire (their element shapes are only partly pinned upstream). See
[Units & dates](/polar-flow-mcp/reference/units-and-dates/).

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

Asks the user to confirm first where the host supports it — see
[User confirmation on writes](#user-confirmation-on-writes).
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

**Response:** JSON array of canonical session objects (`id`, `sport_id`,
`sport_name`, `start_time`, `session_duration_s`, `distance_m`, `hr_avg`,
`calories`, …). Durations are seconds, distances metres, dates ISO 8601 — see
[Units & dates](/polar-flow-mcp/reference/units-and-dates/). Absent values are omitted rather
than sent as `-1` / `""`.

## `get_training_session_summary`

Summary view of a completed session — duration, distance, calories, HR
averages, sport, etc.

| Argument | Type | Required |
|----------|------|----------|
| `session_id` | integer | yes |

**Response:** canonical session object — `session_duration_s` (seconds, decoded
from the wire's ISO-8601 `PTxxM`), `distance_m`, `hr_avg` / `hr_max` (bpm),
`calories`, `start_time` (ISO 8601), `note`, and `feeling` (1 = bad … 5 = great,
`null` when unset). See [Units & dates](/polar-flow-mcp/reference/units-and-dates/).

## `get_training_session_details`

Lap- and sample-level details for a completed session. Use this when the
user asks about pace splits, HR zone time, or per-lap stats.

| Argument | Type | Required |
|----------|------|----------|
| `session_id` | integer | yes |

## Activity and sleep

24/7 data a Polar device syncs to Flow. On an account without such a device
both tools return empty results rather than errors.

### `get_daily_activity`

| Argument | Type | Default |
|----------|------|---------|
| `from_date` | `YYYY-MM-DD` | today − 6 days |
| `to_date` | `YYYY-MM-DD` | today; at most 31 days after `from_date` |

One entry per day: `steps`, `step_distance_m`, `active_time_s`, `kcal`,
`activity_goal_pct` (% of the daily activity goal), `inactivity_alerts`,
`sleep_s`, `intensity` (seconds in `sleep_s` / `sedentary_s` / `light_s` /
`moderate_s` / `vigorous_s`), `heart_rate` (`day_min_bpm`, `day_max_bpm`,
`night_min_bpm`), `benefit` (Polar's activity-benefit feedback) and `last_sync`.
A single-day call adds `samples`: the intraday activity level and heart rate at
10-minute resolution, as `{"t": "HH:MM", "v": …}` in local time. A `summary`
totals the days with data.

Flow answers days without device data with zeros; those days have
`has_data: false` and null metrics instead. Flow's minutes are converted to
seconds and its step distance is already metres. Fetched four days per request.

### `get_sleep`

| Argument | Type | Default |
|----------|------|---------|
| `from_date` | `YYYY-MM-DD` | today − 13 days |
| `to_date` | `YYYY-MM-DD` | today; at most 365 days after `from_date` |

One entry per recorded night, keyed by the wake-up `date`: `fell_asleep` /
`woke_up` (local, ISO 8601), `sleep_s`, `score` (Sleep Score 0–100, null when
Polar has none), `continuity_index` (1–5), `continuity_class`, `sleep_cycles`,
`rating` (the user's own), `stages` (`light_s` / `deep_s` / `rem_s` /
`unknown_s`, Sleep Plus Stages devices only), `interruptions_s` /
`long_interruptions_s`, and a `hypnogram` of `{stage, start_s, end_s}` segments
in seconds after falling asleep. `averages` summarizes the range.

Flow's sleep report only answers windows of 30–365 days, so shorter requests
are widened and filtered. Its field meanings are taken from the Flow web UI's
own code — the test account has no sleep-tracking device, so they are not yet
confirmed against a populated capture.

## Favorites and routes

A **favorite** is a reusable, date-less training-target template (type
`VOLUME`, `STEADY_RACE_PACE` or `PHASED`); an imported GPX/TCX **route** is a
favorite of type `ROUTE`. Every favorite has two ids: the `favorite_id` (used by
almost every tool) and an inner `exercise_target_id` (used by
[`get_route`](#get_route) and, for multi-sport favorites,
[`set_favorite_sport`](#set_favorite_sport)). `list_favorites` returns both.

Polar's favorite endpoints validate little — they store unknown sport ids,
accept whitespace-only names, silently truncate route names and drop
out-of-range route points — so these tools check every argument before any
request goes out and verify writes by reading them back.

| Limit | Value |
|-------|-------|
| Favorite / route name | 1–45 characters, not blank |
| Favorite description | ≤ 500 characters |
| Duration goal | 1 s – 99:59:59 (`359999` s) |
| Distance goal | > 0 and ≤ 9 999 000 m |
| `sport_id` | must exist in [`list_sports`](#list_sports) |

### `list_favorites`

| Argument | Type | Default |
|----------|------|---------|
| `kind` | `all` \| `templates` \| `routes` | `all` |

**Response:** a text list plus canonical items: `favorite_id`,
`exercise_target_id`, `name`, `description`, `type`, `sport_id`, `sport_name`,
`sport_category`, `duration_s`, `distance_m`, `calories`, `route_source`
(`FLOW_FILE_IMPORT` for imported files). Durations are converted from the
wire's milliseconds.

### `get_favorite`

| Argument | Type | Required |
|----------|------|----------|
| `favorite_id` | integer | yes |

**Response:** `favorite_id`, `name`, `description`, `type`, and
`exercise_targets[]` (`exercise_target_id`, `sport_id`, `duration_s`,
`distance_m`, `calories`, `phases`). For PHASED favorites Polar rolls the phase
goals up into `duration_s` / `distance_m`; the phase tree is passed through
raw. A ROUTE favorite has no geometry here — use `get_route`. `No favorite with
favorite_id <id>.` when it does not exist.

### `create_favorite`

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | yes | 1–45 characters. |
| `description` | string | no | ≤ 500 characters. |
| `sport_id` | integer | no | Default `1` (running). |
| `duration_s` | integer | one goal | VOLUME goal in seconds. |
| `distance_m` | number | one goal | VOLUME goal in metres. |
| `phases` | array | one goal | PHASED goal — same shape as [`create_training_target`](#phase-shape). |

Exactly one of `duration_s`, `distance_m` or `phases`.

```json
{"name": "6x800", "sport_id": 1, "phases": [
  {"type": "warmup", "duration_s": 900},
  {"type": "repeat", "reps": 6, "goal": {"distance_m": 800},
   "intensity": {"hr_zone": 4}, "recovery": {"duration_s": 90}},
  {"type": "cooldown", "duration_s": 600}]}
```

**Response:** the new favorite, read back (same shape as `get_favorite`).

### `update_favorite`

Full replace of a single-sport, non-route favorite: same arguments as
`create_favorite` plus the required `favorite_id`. Anything omitted is cleared,
so read it with `get_favorite` first. The tool carries Polar's
exercise-target id over for you — Polar silently ignores an update without it.
ROUTE and multi-sport favorites are refused.

### `rename_favorite`

| Argument | Type | Required |
|----------|------|----------|
| `favorite_id` | integer | yes |
| `name` | string (1–45) | yes |

Works on routes too. Checks the favorite exists first (Polar's rename answers an
unknown id with a server error).

### `set_favorite_sport`

| Argument | Type | Required |
|----------|------|----------|
| `favorite_id` | integer | yes |
| `sport_id` | integer | yes |
| `exercise_target_id` | integer | only for multi-sport favorites |

Polar's endpoint accepts unknown sport ids and exercise-target ids with a
success reply (and clears the sport when none is sent), so the tool validates
both and confirms the change by reading the favorite back.

### `delete_favorite`

| Argument | Type | Required |
|----------|------|----------|
| `favorite_id` | integer | yes |

Deletes a template or a route. Targets already scheduled from it are not
affected. Asks the user to confirm first where the host supports it — see
[User confirmation on writes](#user-confirmation-on-writes).

### `save_target_as_favorite`

| Argument | Type | Required |
|----------|------|----------|
| `target_id` | integer | yes |
| `name` | string (1–45) | no — defaults to the target's name |

The Flow target editor's "Add to favorites" button: copies the target's name,
description, type, sport and phases into a new favorite (the date is dropped).
Polar has no endpoint for this — the web UI copies the target client-side and
creates a favorite, and so does this tool. The target is not linked to the new
favorite; calling twice creates two favorites. Pass `name` when the target's
name exceeds the 45-character favorite limit.

### `schedule_favorite`

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `favorite_id` | integer | yes | A training-target favorite (not a route). |
| `date` | `YYYY-MM-DD` | yes | Local date; past dates are allowed. |
| `time` | `HH:MM` | no | Default `18:00`. `00:00` is refused. |

Creates a training target from the favorite (the diary's "Add → Favorites"
picker) and returns its `target_id`, `date`, `time`, `start`, `sport_id`,
`duration_s`, `distance_m`, `calories`. If another target already starts at
that minute Polar shifts the new one by one minute instead of failing; the
result says so. Polar reads midnight as "no time" and moves it to 18:00, hence
the `00:00` refusal.

### `import_route`

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `content` | string | yes | Full text of the GPX or TCX file. |
| `format` | `auto` \| `gpx` \| `tcx` | no | Default `auto` (detected from the XML root). |
| `name` | string (1–45) | no | Default: the name in the file, else `Imported route`. |
| `sport_id` | integer | no | Binds the route to a sport. |

The file is parsed in Go and uploaded as JSON trackpoints, in exactly the shape
the Flow web UI sends (verified byte-for-byte against captured uploads):

- **GPX** — the first `<trk>` (all of its segments; the web UI keeps only the
  first) or else the first `<rte>`; cumulative distance by haversine with the
  UI's Earth radius; missing `<ele>` → altitude `0`.
- **TCX** — the first `<Course>` or `<Activity>`; only trackpoints with a
  position; the file's own `DistanceMeters`; missing altitude → `null`.

Rejected before upload: empty or non-XML content, unknown root element, fewer
than two points, a latitude outside [-90, 90] or longitude outside
[-180, 180], zero length, decreasing TCX distances, files over 25 MB.

**Response:** `favorite_id`, `exercise_target_id`, `name`, `format`,
`point_count`, `distance_m`. Polar's import returns an empty body, so the tool
finds the new favorite by listing favorites before and after.

### `get_route`

| Argument | Type | Required |
|----------|------|----------|
| `exercise_target_id` | integer | one of the two |
| `favorite_id` | integer | one of the two |
| `max_points` | integer (2–100000) | no — default `500` |

Polar's geometry endpoint is keyed by the **exercise-target id** and answers
"forbidden" for anything else, including a favorite id passed by mistake; pass
`favorite_id` and the tool resolves it. **Response:** `exercise_target_id`,
`name`, `distance_m`, `sport_id`, `point_count` (full), `returned_points`,
`start`, and `points[]` (`lat`, `lon`, `altitude_m`), evenly down-sampled to
`max_points` with the first and last point kept. Polar's synthesized per-point
`time` (a sequence index) is dropped.

## Editing and deleting completed sessions

### `edit_training_session`

What the diary's **Edit session** form allows. Pass only what changes;
everything else keeps its current value. The date/time cannot be edited.

| Argument | Type | Limits |
|----------|------|--------|
| `session_id` | integer | required |
| `name` | string | 1–100 characters |
| `note` | string | ≤ 10 000 characters; `""` clears it |
| `feeling` | integer | 1 (bad) – 5 (great) |
| `sport_id` | integer | must exist in `list_sports` |
| `duration_s` | integer | 1 – 359999 seconds |
| `distance_m` | number | 0 – 9 999 000 metres |
| `hr_avg` | integer | 0 – 240 bpm (`0` clears) |
| `hr_max` | integer | 0 – 240 bpm, ≥ `hr_avg` |
| `kcal` | integer | 0 – 65535 |
| `speed_kmh` | number | 0 – 399 km/h |

```json
{"session_id": 8429796771, "distance_m": 10200, "note": "Windy, legs heavy", "feeling": 3}
```

- `note` and `feeling` alone go through Polar's partial note/feeling endpoint,
  which is safe on any session.
- Any other field goes through the full edit form, which the tool only uses on
  single-exercise **manually entered** sessions (recorded sessions are refused:
  the web form re-synthesizes the heart-rate trace, which is unverified on
  device data). The tool reads the live values and sends every field, because
  the endpoint resets a missing name to the sport name and stores a `null` note
  as the text `"null"`.
- Polar answers any out-of-range value with an unexplained server error, which
  is why the limits above are enforced before sending.
- A feeling cannot be removed once set.

**Response:** the session summary as read back (same shape as
[`get_training_session_summary`](#get_training_session_summary), including
`feeling` as 1–5).

### `delete_training_session`

| Argument | Type | Required |
|----------|------|----------|
| `session_id` | integer | yes |

Deletes a completed session (`DELETE /api/training/deleteTrainingSession/{id}/`
— the trailing slash is required). Polar reports success even for an id that
does not exist, so the tool first checks the session exists (`No session with
id <id>.` otherwise, or an error for another account's session) and afterwards
verifies it is gone. Asks the user to confirm first where the host supports it
— see [User confirmation on writes](#user-confirmation-on-writes).

## Behaviour notes

- **Units & dates.** Every tool speaks one canonical contract — durations in
  seconds (`*_s`), distances in metres (`*_m`), speed in km/h (`*_kmh`), heart
  rate in bpm, ISO 8601 dates — regardless of the many encodings the underlying
  Flow endpoints use. The adapter (`internal/convert`) does the conversion. See
  [Units & dates](/polar-flow-mcp/reference/units-and-dates/) for the full table.
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
