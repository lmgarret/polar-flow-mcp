---
title: Favorites, routes and session edits
description: Keep reusable workouts as favorites, schedule them, import GPX/TCX routes, and fix or delete completed sessions — with worked examples.
sidebar:
  order: 2
---

This guide covers the tools added on top of training targets: favorites
(reusable workout templates), imported routes, and edits to sessions you have
already done. Each section shows what to ask Claude and the tool call it turns
into. Full argument tables live in the
[MCP tools reference](/polar-flow-mcp/reference/mcp-tools/#favorites-and-routes).

## Keep a workout as a favorite

A favorite is a training target without a date. Create one directly, or save a
target you already scheduled.

> "Save a 6×800 m session as a favorite: 15 min warm-up, 800 m reps in zone 4
> with 90 s jog recovery."

```json
create_favorite {"name": "6x800", "sport_id": 1, "phases": [
  {"type": "warmup", "duration_s": 900},
  {"type": "repeat", "reps": 6, "goal": {"distance_m": 800},
   "intensity": {"hr_zone": 4}, "recovery": {"duration_s": 90}}]}
```

> "Save next Tuesday's threshold session as a favorite."

```json
save_target_as_favorite {"target_id": 1461116593}
```

Favorite names are limited to **45 characters** — shorter than target names.
If the target's name is longer, Claude passes a shorter `name`.

To tweak a favorite, `rename_favorite` and `set_favorite_sport` change one
thing; `update_favorite` replaces the whole favorite (read it with
`get_favorite` first).

## Put a favorite in the diary

> "Schedule my 6x800 favorite for Saturday at 7:30."

```json
schedule_favorite {"favorite_id": 83468869, "date": "2026-10-24", "time": "07:30"}
```

The result carries the new `target_id`, which the training-target tools accept.
Two things Polar does on its own:

- If another target already starts at that minute, the new one is moved one
  minute later. The result tells you when that happened.
- Midnight means "no time" to Polar (it would schedule 18:00), so `00:00` is
  refused — use `00:01`.

## Import a route for navigation

Give Claude the text of a GPX or TCX file (paste it, or let it read the file).

> "Import this GPX as a running route called Park loop."

```json
import_route {"content": "<?xml version=\"1.0\"?><gpx …</gpx>", "name": "Park loop", "sport_id": 1}
```

The file is parsed by the server and uploaded the same way the Flow web page
does it. Problems are reported before anything is uploaded: a file with fewer
than two positions, coordinates out of range, a name over 45 characters, or an
unknown sport. (Polar itself would silently truncate the name or drop the bad
points.)

To look at a route again:

```json
get_route {"favorite_id": 83468958, "max_points": 200}
```

The geometry endpoint is keyed by the route's `exercise_target_id`, not its
`favorite_id`; passing `favorite_id` makes the tool look it up for you.

## Fix a completed session

> "Yesterday's run was actually 10.2 km, and it felt average."

```json
edit_training_session {"session_id": 8429796771, "distance_m": 10200, "feeling": 3}
```

Pass only the fields that change. Feeling is a 1–5 rating (1 = bad,
5 = great). The session's date and time cannot be changed, and on sessions
recorded by a watch only the note and feeling can be edited. The tool checks
every value against the limits Polar enforces before sending, because Polar
itself only answers "server error" without saying which field was wrong.

## Delete a completed session

> "Delete the duplicate session from Monday."

```json
delete_training_session {"session_id": 8429796771}
```

Deleting is permanent. Where your MCP host supports it, you are asked to confirm
first; the prompt names the session (title, start time, duration). The same
applies to `delete_favorite`.
