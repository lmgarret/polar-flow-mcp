# Tool reference

Exact parameters and return shapes for every polar-flow-mcp tool. Names are
case-sensitive. All dates are **ISO YYYY-MM-DD**, times **24h HH:MM**,
distances **metres**, durations **seconds**, HR **bpm**, speed **km/h**,
pace **seconds per km**, power **watts**.

Legend: **req** = required, everything else optional with the noted default.

---

## Identity

### `get_user_info`
No parameters. Returns the linked account's numeric `id`, `email`,
`firstName`, `lastName`, `country`. Call once per session to confirm which
account the server drives.

### `list_sports`
No parameters. Returns the full Polar sport catalogue as a map of numeric sport
id → name constant (e.g. `"1": "RUNNING"`, `"2": "CYCLING"`, `"23": "SWIMMING"`).
These ids are the `sport_id` values for `create_training_target` and
`create_training_session`. Call this to find the id for any non-running sport.
The catalogue is a moving snapshot (Polar adds sports), so re-fetch rather than
hard-coding ids.

### `get_training_zones`
Optional `sport_id` (integer). Returns what zones 1–5 mean for that sport:
`heart_rate.zones[{zone, min_bpm, max_bpm}]`,
`speed.zones[{zone, min_kmh, max_kmh, slowest_pace_s_per_km, fastest_pace_s_per_km}]`,
`power.zones[{zone, min_w, max_w}]`, plus `thresholds` (MAS km/h, MAP / FTP W).
Ranges are `[min, max)`; the top speed/power zone has `max_*: null` (no
ceiling). A type the sport lacks is omitted (swimming: HR only). `source` is
`profile` (the account's stored sport profile — `setting: "free"` means the
user typed the limits) or `default` (Polar's defaults, computed without
saving). Without `sport_id`: every stored profile, possibly none. Use it to
turn "4:30/km" or "220 W" into a `speed_zone` / `power_zone` number.

### `create_sport_profile`
- `sport_id` **req**

Creates the sport's profile with Polar's default zones and settings. Idempotent
— an existing profile is returned unchanged. Needed before
`update_training_zones` can edit that sport. Returns the zones like
`get_training_zones`.

### `update_training_zones`
- `sport_id` **req** — the sport must have a stored profile
- `heart_rate_bpm` — 6 ascending whole bpm (Z1 lower … Z5 upper), 15–240,
  each zone ≥ 2 bpm
- `speed_kmh` — 5 or 6 ascending km/h, 1–399 (5 = open-ended Z5). Always km/h,
  even for running: 5:00/km = 12 km/h
- `power_w` — 5 or 6 ascending whole watts, 0–2000, each zone ≥ 2 W
- `reset` — any of `"heart_rate"`, `"speed"`, `"power"`: back to Polar's defaults

Lists are zone **boundaries**, so zones are contiguous by construction. Unnamed
lists stay as they are. Returns the saved zones (read back). Example:
`{"sport_id": 1, "heart_rate_bpm": [115, 134, 153, 172, 182, 192]}`.

### `delete_sport_profile`
- `sport_id` **req**

Deletes the sport's profile (its zones fall back to Polar's defaults). Says
"nothing to delete" when there is none; Polar refuses the account's last
profile. Asks the user to confirm where the host supports it.

---

## Planned training targets

See `reference/training-targets.md` for the phases model and worked examples.

### `list_training_targets`
- `from_date` — start (default: today)
- `to_date` — end (default: today + 30 days)

Returns each planned target's numeric id, title, and start time. Use the ids
with the get/update/delete target tools.

### `get_training_target`
- `target_id` **req** — numeric id

Returns the full, server-normalized target body (name, datetime, per-sport
exercise targets, rolled-up phases). On read-back the server fills phase
durations (a DISTANCE phase shows `"00:00:00"`) and rolls each exerciseTarget's
duration up from its phases. **Always call this before `update_training_target`.**

### `create_training_target`
- `name` **req** — diary display name, 1–45 characters (an emoji counts 2)
- `date` **req** — scheduled local date
- `time` — start time (default: `18:00`)
- `sport_id` — Polar sport id (default: `1` = running). Common: `2` cycling,
  `23` swimming, `15` strength_training, `11` hiking, `68` triathlon. Call
  `list_sports` for the full catalogue.
- `description` — free-text notes
- `phases` — ordered list of `warmup` / `repeat` / `cooldown` blocks; omit for
  an open VOLUME target. See `reference/training-targets.md`.

Returns the **new numeric target id**.

### `update_training_target`
Same fields as create, plus `target_id` **req**. **Full replace** — the whole
target is overwritten, so read with `get_training_target`, modify, then send
the complete body. `name` and `date` are required. Returns a confirmation.

### `delete_training_target`
- `target_id` **req**

Permanently deletes the target. Irreversible. Reports "no target with id …" if
it doesn't exist (or was already deleted); another account's id is an error.
Confirm with the user first.

---

## Calendar & history (reads)

See `reference/reading-data.md` for ranges and what each returns.

### `get_calendar_events`
- `from_date` — start (default: today)
- `to_date` — end (default: today + 30 days)

Raw diary events: completed sessions, planned targets (`type
"TRAININGTARGET"`), and other items. Low-level — prefer the typed list tools and
reach for this only when you need the unfiltered calendar.

### `get_calendar_week_summary`
- `from_date` — start (default: today − 28 days)
- `to_date` — end (default: today)

One entry per ISO week intersecting the range — the diary's weekly-totals strip.
**Range must be ≤ 45 days** and `from_date ≤ to_date`, or the server rejects it.

### `list_training_sessions`
- `from_date` — start (default: today − 30 days)
- `to_date` — end (default: today)

Completed sessions with id, sport, start time, distance, duration. Pass an id to
the summary/details tools below.

### `get_training_session_summary`
- `session_id` **req**

Totals for one completed session: duration, distance, avg/max HR, calories,
sport.

### `get_training_session_details`
- `session_id` **req**

Lap splits and per-sample time series (HR, speed, …). **Heavy payload** — use
only when the user asks about splits or the within-session trace.

### `get_progress_summary`
- `from_date` — start (default: today − 90 days)
- `to_date` — end (default: today)
- `group` — time-bucket granularity (NOT a sport filter): `MONTH` (default,
  only value verified live), likely also `DAY`/`WEEK`/`YEAR`
- `time_frame` — breakdown bucket size: `6w`, `3m`, or `1y` (default: `3m`)

Aggregated totals + distributions (session count, distance, duration, HR-zone
time, per-sport mix, training-benefit mix). Accounts with no sessions return
zeros rather than an error.

### `get_daily_activity`
- `from_date` — first day (default: today − 6 days)
- `to_date` — last day (default: today; ≤ 31 days after `from_date`)

Per day: `steps`, `step_distance_m`, `active_time_s`, `kcal`,
`activity_goal_pct`, `inactivity_alerts`, `sleep_s`, `intensity` (seconds per
band: `sleep_s`, `sedentary_s`, `light_s`, `moderate_s`, `vigorous_s`),
`heart_rate` (`day_min_bpm`, `day_max_bpm`, `night_min_bpm`). Days without
device data (and future days) have `has_data: false` and null metrics — they
are not zero-step days. `heart_rate`, `intensity` and `benefit` can be null. A single-day call adds intraday `samples` (`{"t": "HH:MM", "v": …}`).

### `get_sleep`
- `from_date` — first wake-up date (default: today − 13 days)
- `to_date` — last wake-up date (default: today; ≤ 365 days after `from_date`)

Per night (keyed by the wake-up `date`): `fell_asleep`, `woke_up`, `sleep_s`,
`score` (0–100 or null), `continuity_index` (1–5), `sleep_cycles`, `rating`,
`stages` (`light_s`, `deep_s`, `rem_s`, `unknown_s`; null on nights without
Sleep Plus Stages), `interruptions_s`, and a `hypnogram` of `{stage, start_s,
end_s}` with stage `wake` / `light` / `deep` / `rem` / `unknown`, or `sleep` on
nights without stages.
`averages` covers the range. Empty without a sleep-tracking device.

---

## Completed sessions (writes)

### `create_training_session`
⚠ Writes real history. **User-initiated only.** Full detail and the unit
conventions are in `reference/logging-sessions.md`.

- `name` **req** — diary display name
- `date` **req** — date it happened
- `time` — start time (default: `18:00`)
- `duration_s` **req** — elapsed seconds
- `distance_m` — metres (default: `0`)
- `kcal` — kilocalories (default: `0`)
- `hr_avg` — average bpm (omit or `0` = unset)
- `hr_max` — max bpm (omit or `0` = unset)
- `speed_kmh` — average km/h (omit or `0` = unset)
- `sport_id` — Polar sport id (default: `1`). Common: `2` cycling, `23` swimming,
  `15` strength_training, `11` hiking, `68` triathlon. Call `list_sports` for
  the full catalogue.
- `note` — free-text note

### `edit_training_session`
Changes a completed session in place. **User-initiated only.** See
`reference/logging-sessions.md`.

- `session_id` **req**
- Pass only the fields to change. Everything else keeps its current value.
- Any session: `note` (≤ 10 000 characters, `""` clears it) and `feeling`
  (1 = bad … 5 = great; can't be removed once set).
- Manual single-exercise sessions only: `name` (1–100 characters),
  `sport_id`, `duration_s` (1–359 999), `distance_m` (0–9 999 000),
  `hr_avg` / `hr_max` (0–240 bpm, 0 clears, `hr_max ≥ hr_avg`), `kcal`
  (0–65 535), `speed_kmh` (0–399).
- The date and time can't be changed.

Returns the session summary as read back.

### `delete_training_session`
- `session_id` **req**

Permanently deletes a completed session. **User-initiated only.** It first
checks that the session exists and reports "no session" if not, because
Polar's delete answers success for any id. Afterwards it checks the session is
gone. It may ask the user to confirm (elicitation).

---

## Favorites (templates) & routes

See `reference/favorites-and-routes.md` for the workflows.

### `list_favorites`
- `kind` — `all` (default), `templates` or `routes`

Each entry: `favorite_id`, `exercise_target_id`, `name`, `type`
(`VOLUME` / `STEADY_RACE_PACE` / `PHASED` / `ROUTE`), `sport_id`,
`duration_s`, `distance_m`, `calories`.

### `get_favorite`
- `favorite_id` **req**

Returns the name, description, type and `exercise_targets[]` (`sport_id`,
`duration_s`, `distance_m`, and the raw `phases` of a PHASED favorite). For a
route this is only the name, sport and distance. Use `get_route` for the
geometry.

### `create_favorite`
- `name` **req** — 1–45 characters
- `description` — ≤ 500 characters
- `sport_id` — default `1`, must exist in `list_sports`
- exactly one goal: `duration_s` (1–359 999), `distance_m` (≤ 9 999 000), or
  `phases` (same shape as `create_training_target`)

Returns the new favorite as read back.

### `update_favorite`
Same fields as `create_favorite`, plus `favorite_id` **req**. **Full
replace**: call `get_favorite` first and send everything that should stay.
Refused for route favorites and multi-sport favorites.

### `rename_favorite`
- `favorite_id` **req**
- `name` **req** — 1–45 characters, not blank

Works for templates and routes.

### `set_favorite_sport`
- `favorite_id` **req**
- `sport_id` **req**
- `exercise_target_id` — only needed for multi-sport favorites

Checks that the sport exists, applies the change, and reads the favorite back
to confirm it.

### `delete_favorite`
- `favorite_id` **req**

Permanently deletes a template or route. Targets already scheduled from it are
kept. It may ask the user to confirm (elicitation).

### `save_target_as_favorite`
- `target_id` **req**
- `name` — 1–45 characters; defaults to the target's name

Copies a scheduled target into a new favorite and drops the date. The copy
isn't linked back to the target.

### `schedule_favorite`
- `favorite_id` **req** — a template, not a route
- `date` **req**
- `time` — default `18:00`; `00:00` is not allowed

Creates a new training target and returns its `target_id`. If another target
already starts at that minute, Polar shifts the new one +1 minute; the result
says so.

### `import_route`
- `content` **req** — full GPX or TCX text
- `format` — `auto` (default), `gpx`, `tcx`
- `name` — 1–45 characters; defaults to the name in the file
- `sport_id`

Rejected before upload if the file has fewer than 2 points, invalid
coordinates, zero length, or is over 25 MB. Returns `favorite_id`,
`exercise_target_id`, the point count and the length in metres.

### `get_route`
- `exercise_target_id` or `favorite_id` — one is required
- `max_points` — 2–100 000, default 500

Returns the name, `distance_m`, `sport_id`, `point_count`, `returned_points`
and `points[{lat, lon, altitude_m}]`. Long routes are thinned out evenly,
always keeping the first and last point.
