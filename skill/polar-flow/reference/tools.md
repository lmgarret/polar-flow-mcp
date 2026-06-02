# Tool reference

Exact parameters and return shapes for every polar-flow-mcp tool. Names are
case-sensitive. All dates are **ISO YYYY-MM-DD**, times **24h HH:MM**,
distances **metres**, durations **seconds**, HR **bpm**, speed **km/h**.

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
- `name` **req** — diary display name
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
it doesn't exist or belongs to another account. Confirm with the user first.

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

---

## Logging a completed session (write)

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
