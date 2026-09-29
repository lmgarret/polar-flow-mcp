# Favorites and routes

A **favorite** is a reusable, date-less workout template, like "Easy 45" or
"6×800 m", that syncs to the watch and can be put in the diary on any date.
A **route** is a GPS track imported from a GPX/TCX file and used for watch
navigation. Polar stores routes as favorites of type `ROUTE`, so the same
list, rename, sport and delete tools cover both.

## Favorite vs training target

| | Training target | Favorite |
|---|---|---|
| What | A planned workout **on a date** | A **template** with no date |
| Tools | `*_training_target` | `*_favorite`, `schedule_favorite` |
| Id | `target_id` | `favorite_id` (+ `exercise_target_id`) |

You can convert in both directions:

- **Template → diary:** `schedule_favorite(favorite_id, date, time)` copies the
  favorite into a **new target** and returns its `target_id`. Edit or delete
  that target with the target tools. The favorite is unchanged.
- **Diary → template:** `save_target_as_favorite(target_id)` copies a scheduled
  target into a **new favorite** and drops the date. Neither copy stays linked
  to the other, and calling it twice creates two favorites.

Pick a favorite when the user wants something reusable ("save this as my
Tuesday intervals", "put my long-run template on Sunday"). Pick a target for a
one-off planned workout.

## The two ids

`list_favorites` returns both ids for every entry. Use each where it belongs:

| Id | Used by |
|----|---------|
| `favorite_id` | `get_favorite`, `update_favorite`, `rename_favorite`, `set_favorite_sport`, `schedule_favorite`, `delete_favorite` |
| `exercise_target_id` | `get_route` (Polar keys route geometry by it), `set_favorite_sport` on multi-sport favorites |

`get_route` also accepts `favorite_id` and looks up the other id for you.

## Limits (checked before anything is sent)

Polar hardly validates favorites itself. It stores unknown sport ids and
whitespace-only names, and silently truncates route names. The tools therefore
reject bad input up front and the error names the argument. Keep to:

- **name:** 1–45 characters, not blank. That's stricter than training targets.
  Pass a shorter `name` to `save_target_as_favorite` when the target's name is
  longer.
- **description:** ≤ 500 characters.
- **goal:** exactly one of `duration_s` (seconds, ≤ 359 999), `distance_m`
  (metres, ≤ 9 999 000) or `phases` (same shape as `create_training_target`,
  see `reference/training-targets.md`).
- **sport_id:** must exist in `list_sports`.

## Creating and editing templates

```
create_favorite(name="6x800", sport_id=1, phases=[
  {"type": "warmup", "duration_s": 900},
  {"type": "repeat", "reps": 6, "goal": {"distance_m": 800},
   "intensity": {"hr_zone": 4}, "recovery": {"duration_s": 90}},
  {"type": "cooldown", "duration_s": 600}])
```

**`update_favorite` is a full replace**, just like `update_training_target`.
It overwrites the name, description, sport and goal, so call `get_favorite`
first and send back everything that should stay. It refuses route favorites
and multi-sport favorites. To change only one thing, use the narrow tools,
which leave the rest alone:

- `rename_favorite(favorite_id, name)` works for templates and routes.
- `set_favorite_sport(favorite_id, sport_id)` checks the sport and then reads
  the favorite back to confirm. It needs `exercise_target_id` only for
  multi-sport favorites.

## Scheduling

```
schedule_favorite(favorite_id=83468869, date="2026-10-20", time="07:30")
```

- The tool returns the new `target_id`.
- Past dates are allowed.
- **00:00 is not allowed**, because Polar treats midnight as "no time" and
  uses 18:00 instead.
- If another target already starts at that exact minute, Polar moves the new
  one 1 minute later instead of failing. The result says when this happens.
  Tell the user.
- Route favorites cannot be scheduled.

## Routes

- **Import:** `import_route(content=<full GPX/TCX text>, sport_id=…, name=…)`.
  The file is parsed server-side and needs ≥ 2 points, valid coordinates, a
  non-zero length and a size ≤ 25 MB. The result returns `favorite_id`,
  `exercise_target_id`, the point count and the length in metres. Only pass a
  file the user supplied. Don't invent a GPS track.
- **Read geometry:** `get_route(exercise_target_id)` or `get_route(favorite_id)`
  returns the waypoints `{lat, lon, altitude_m}`. Long routes are thinned out
  to `max_points` (default 500). `point_count` is always the full count.
- `get_favorite` on a route returns only its name, sport and distance. Use
  `get_route` for the track itself.

## Deleting

`delete_favorite(favorite_id)` is permanent. It works for templates and
routes. Targets already scheduled from the favorite are kept.

- Confirm with the user first.
- On hosts that support elicitation the tool asks the user itself. If the
  result says the user did not confirm, nothing was deleted. Don't retry
  around it.
