---
name: polar-flow
description: >-
  Drives the polar-flow-mcp server to read and write a Polar Flow training
  diary: planned workouts (training targets), completed sessions, heart-rate /
  pace / power zones, favorites (workout templates), GPS routes, 24/7 activity
  and sleep. Use when a task touches Polar Flow or calls a polar-flow-mcp
  tool, or when the user asks to schedule, template, log, edit or review
  workouts there. Covers tool mechanics and safe write practices, not
  coaching methodology.
---

# polar-flow

How to use the polar-flow-mcp tools together. Each tool's own description
and schema already give its parameters, units, limits and an example, so this
skill covers what they don't: which tool fits a request, rules that span
several tools, and multi-step workflows. Deciding *what* training to do is
out of scope; leave that to a coaching skill (e.g. `running-coach`) and use
this one to carry the decision out.

Tool names are given bare (`list_sports`). Your client may prefix them with
the connector name (e.g. `mcp__Polar_Flow__list_sports`). If no Polar Flow
tools are available, the server isn't connected: say so and stop.

## Which tool

| The user wants… | Use |
|---|---|
| What's planned | `list_training_targets` → `get_training_target` |
| What was done | `list_training_sessions` → `get_training_session_summary`; `get_training_session_details` for zones, interval reps (pace / HR per rep) and laps; `get_training_session_samples` for the HR / pace curve inside one rep or window |
| Weekly totals / longer trends | `get_calendar_week_summary` (≤ 45 days) / `get_progress_summary` |
| Steps, daily HR / sleep | `get_daily_activity` / `get_sleep` |
| What zone N means, or which zone a pace / wattage / bpm falls in | `get_training_zones(sport_id)` |
| A sport's id | `list_sports` (don't guess beyond the common ids in the tool docs) |
| Plan a one-off workout on a date | `create_training_target` |
| A reusable template, or put one in the diary | `create_favorite`, `save_target_as_favorite`, `schedule_favorite` |
| A GPS route | `import_route`, `get_route` |
| Log / correct / remove a completed session | `create_training_session` / `edit_training_session` / `delete_training_session` |
| Change their zones | `create_sport_profile` (if the sport has none) → `update_training_zones` |

Planned workouts (**targets**) and completed workouts (**sessions**) are
separate. "What's on today?" needs both. `get_calendar_events` returns every
raw diary entry. Use it only when the typed list tools miss something.

## Rules

1. **Full-replace updates.** `update_training_target` and `update_favorite`
   overwrite every field and clear whatever you leave out. Always read the
   current version first (`get_training_target` / `get_favorite`), change it,
   then send the whole thing back. To change only a favorite's name or sport,
   use `rename_favorite` / `set_favorite_sport` instead.
2. **Session history is the user's record.** Only create, edit or delete a
   completed session when the user explicitly asks about one they actually
   did. Never backfill, invent or "tidy up" sessions. If logging would help,
   suggest it and wait.
3. **Confirm writes** unless the user directly asked for that exact change.
   Some hosts also ask the user themselves. If a result says the user did not
   confirm, nothing changed: report that and don't retry another way.
4. **Zones are numbers 1–5, not bpm, watts or km/h.** Each athlete's zones
   are different, so turn a pace, wattage or heart rate into
   `hr_zone` / `speed_zone` / `power_zone` with `get_training_zones` for the
   workout's sport. Never estimate a zone from generic percentages.
5. **Errors are information.** If a tool names a bad argument, nothing
   reached Polar: fix that argument and retry. For other failures, report the
   message as returned and don't retry blindly. Never make up a result.

## Workflows

**"Intervals at 4:30/km"**: `get_training_zones(sport_id=1)` → pick the speed
zone whose pace range contains 270 s/km → `create_training_target` with
`intensity: {"speed_zone": N}`.

**"Move Thursday's run to Friday"**: `list_training_targets` →
`get_training_target(id)` → `update_training_target` with the full body read
back and only the date changed.

**"I ran 10.2 km, not 10"**: `list_training_sessions` →
`edit_training_session(session_id, distance_m=10200)`. Only manually entered
sessions can change their numbers; on a watch-recorded session only `note`
and `feeling` can change. The date can't change at all: moving a session
means deleting and re-logging it, and only if the user agrees.

**Several writes (e.g. a week of workouts).** Copy this checklist and work
through it:

```
- [ ] Confirm the whole plan with the user once
- [ ] Look up sport ids and zones once, up front
- [ ] Create one item at a time and note each returned id
- [ ] On a failure, stop and report what landed and what didn't
- [ ] Finish with list_training_targets over the range to show the result
```

## Reference

- **Building workouts** (phase patterns, VOLUME vs phased, intensity labels,
  read-back quirks): [reference/training-targets.md](reference/training-targets.md)
- **Favorites, scheduling and routes** (the two ids, narrow vs full edits):
  [reference/favorites-and-routes.md](reference/favorites-and-routes.md)
