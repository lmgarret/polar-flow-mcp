# Logging completed sessions

`create_training_session` logs an **already-completed** workout into the diary —
the "Manual training result" form in Polar Flow. It is the only write that
creates session history.

## ⚠ When you may call it

**User-initiated only.** Call it only when the user explicitly asks to log a
real session they actually did off-watch — e.g. "log the 5k I ran yesterday",
"add my treadmill run from this morning".

Do **not**:

- call it on your own initiative or to "be helpful",
- fabricate or backfill sessions,
- use it to create test data on a real account.

It counts toward weekly volume, progress summaries, and Polar's training-load
model. A fabricated session permanently contaminates those metrics. If you
think logging would help but the user hasn't asked, **suggest it and wait**.

This logs the **past**. To schedule a **future** workout, use
`create_training_target` (see `reference/training-targets.md`).

## Parameters & units

| Param | Req | Unit / format | Notes |
|-------|-----|---------------|-------|
| `name` | ✓ | text | Diary display name, e.g. `"Easy 5km"`. |
| `date` | ✓ | ISO `YYYY-MM-DD` | The day it happened. |
| `time` | | 24h `HH:MM` | Start time. Default `18:00`. |
| `duration_s` | ✓ | **seconds** | Elapsed time, e.g. `1800` = 30 min. |
| `distance_m` | | **metres** | Default `0`. `5000` = 5 km. |
| `kcal` | | kilocalories | Default `0`. |
| `hr_avg` | | bpm | Omit or `0` to leave unset. |
| `hr_max` | | bpm | Omit or `0` to leave unset. |
| `speed_kmh` | | km/h | Omit or `0` to leave unset. |
| `sport_id` | | Polar sport id | Default `1` = running. |
| `note` | | text | Optional. |

Pass heart-rate values as plain bpm integers; `0` (or omitting) means "not
recorded". The tool handles the API's empty-string convention for unset HR
internally — you never send empty strings yourself.

## Worked example

A 30-minute, 5 km easy run at avg 142 bpm on the morning of 2026-05-25:

```
create_training_session(
  name = "Easy 5km",
  date = "2026-05-25",
  time = "08:48",
  duration_s = 1800,
  distance_m = 5000,
  hr_avg = 142,
  sport_id = 1
)
```

After logging, the session appears in `list_training_sessions` and contributes
to `get_progress_summary` / `get_calendar_week_summary`.
