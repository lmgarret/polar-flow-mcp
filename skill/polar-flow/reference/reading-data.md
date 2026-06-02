# Reading data

How to pull history, summaries, calendar, and progress out of Polar Flow. All
these tools are read-only and safe to call freely. Dates are ISO `YYYY-MM-DD`.

## Picking the right read

| Question | Tool | Notes |
|----------|------|-------|
| What's planned? | `list_training_targets` | Default today..+30d. Returns ids for edit/delete. |
| What's in this planned session? | `get_training_target` | Full normalized body by id. |
| What did I actually do? | `list_training_sessions` | Completed sessions; default last 30d. |
| How did one session go? | `get_training_session_summary` | Totals by id. |
| Show me splits / per-lap HR | `get_training_session_details` | Heavy; only when asked. |
| Weekly volume trend | `get_calendar_week_summary` | Per-ISO-week; ≤45-day range. |
| Monthly/quarterly totals | `get_progress_summary` | Distributions; default last 90d. |
| I need raw diary entries | `get_calendar_events` | Low-level fallback. |

Prefer the typed list tools (`list_training_targets`, `list_training_sessions`)
over `get_calendar_events` — reach for raw events only when the typed tools
don't return what you need.

## Date ranges & defaults

Every range tool defaults to a sensible window if you omit dates:

- `list_training_targets`, `get_calendar_events` → today .. +30 days
- `list_training_sessions` → last 30 days
- `get_calendar_week_summary` → last 28 days (**hard cap: 45-day span**,
  `from_date ≤ to_date`)
- `get_progress_summary` → last 90 days

Pass explicit `from_date` / `to_date` when the user names a window ("last two
weeks", "this month", "since April"). Convert relative phrases to ISO dates
yourself before calling.

## Planned vs completed

Two different worlds — don't confuse them:

- **Targets** = *planned* future workouts. Created/edited/deleted with the
  `*_training_target` tools; listed by `list_training_targets`.
- **Sessions** = *completed* workouts that already happened. Read with
  `list_training_sessions` and the session summary/details tools. The only way
  to create one is `create_training_session` (see
  `reference/logging-sessions.md`) — and that is user-initiated only.

If asked "what should I do today / did I do today", read both: targets for the
plan, sessions for what's done.

## Session summary vs details

- **Summary** (`get_training_session_summary`) — duration, distance, avg/max
  HR, calories, sport. The default for "how did my run go?".
- **Details** (`get_training_session_details`) — lap splits and the full
  per-sample time series. Large. Only call it when the user explicitly asks
  about pace splits, per-lap HR, or the within-session trace.

Typical flow: `list_training_sessions` (today) → grab the id →
`get_training_session_summary(id)` → only then `…_details(id)` if needed.

## Progress summary

`get_progress_summary` is the headline analytics read. Beyond `from_date` /
`to_date` it takes:

- `group` — time-bucket granularity for the breakdown, **not** a sport filter:
  `MONTH` (default, only value verified live), likely also `DAY`/`WEEK`/`YEAR`.
- `time_frame` — bucket size for the per-time-slice breakdowns: `6w`, `3m`, or
  `1y` (default `3m`). It only changes how the breakdown is grouped, not the
  headline totals.

Accounts with no recorded sessions return **zeros**, not an error — so a
zeroed result means "no data in range", not a failure.

## Unit reminders on read

Returned values follow the wire API, which is not always self-consistent across
endpoints (e.g. some durations come back in milliseconds, distances in metres
as floats). Read field-by-field; don't assume a unit carries across tools.
Distances are metres; convert to km for display only.
