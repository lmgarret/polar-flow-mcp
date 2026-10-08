# Building and editing planned workouts

## Contents
- Choosing a shape
- Intensity
- Editing: turning the read-back into update arguments
- What an update can't keep

## Choosing a shape

| The user asks for… | Send to `create_training_target` |
|---|---|
| A slot with no structure ("run Sunday") | No `phases` and no goal. This is an open VOLUME target |
| One total goal ("35 min easy", "10 km") | No `phases`, top-level `duration_s` or `distance_m` (VOLUME) |
| One continuous zoned effort by **time** ("40 min at threshold") | A single `warmup` phase with `intensity` and a custom `name` |
| One continuous zoned effort by **distance** ("8 km at tempo") | `repeat` with `reps: 2` and half the distance, or ask the user. A distance-goal block has to be a `repeat`, and `reps` must be at least 2 |
| Structured intervals | `warmup` → `repeat` (`goal` + optional `recovery`) → `cooldown` |

There is no "main" phase type. `warmup` / `cooldown` take only a duration, and
the `name` field relabels them.

## Intensity

Each phase takes at most one of `hr_zone`, `speed_zone`, `power_zone` (all zone
numbers 1–5) or `label`. Pass a label when the user speaks in effort terms. Use
`hr_zone` only when they name a zone number. Labels map to HR zones:

| `label` | HR zone |
|---|---|
| `easy` | 1–2 |
| `aerobic` | 2 |
| `tempo` | 3 |
| `threshold` | 4 |
| `vo2max` | 5 |

For a pace, wattage or bpm, read `get_training_zones` for the sport and pick
the zone whose `[min, max)` contains it. For pace, compare against
`slowest_pace_s_per_km` / `fastest_pace_s_per_km`. `power_zone` only means
something on power-capable sports such as cycling.

## Editing: turning the read-back into update arguments

`get_training_target` returns Polar's raw format, not the `phases` format that
`update_training_target` takes. Translate it, keeping every field you are not
asked to change:

| Read back (`exerciseTargets[0].phases[]`) | Send |
|---|---|
| Top-level `phaseType: "PHASE"` | `warmup` (before any repeat) or `cooldown` (after it), with `name` and `duration_s` from `duration` (`HH:MM:SS` → seconds) |
| `phaseType: "REPEAT"`, `repeatCount: N`, `phases: [work, recovery?]` | `repeat`, `reps: N`. Take `goal` from the work leaf, `recovery.duration_s` from the second leaf, and keep both names |
| `goalType: "DISTANCE"` | `goal.distance_m` = `distance`. **Ignore** the `"00:00:00"` duration Polar adds to distance phases |
| `goalType: "DURATION"` | `goal.duration_s` = `duration` in seconds |
| `intensityType` `HEART_RATE_ZONES` / `SPEED_ZONES` / `POWER_ZONES`, `lowerZone` = `upperZone` = Z | `hr_zone` / `speed_zone` / `power_zone`: Z |
| `HEART_RATE_ZONES` 1–2 | `label: "easy"` |
| `intensityType: "NONE"` (zones read back as `0`) | Omit `intensity` |

Always decide whether a phase is a distance or duration goal from `goalType`,
never from which field is filled in. Also carry over the top-level `name`,
`description`, `sport_id` (`exerciseTargets[0].sportId`), and the date and time
(`datetime`). For a VOLUME target (`type: "VOLUME"`, no phases), send
`duration_s` from `exerciseTargets[0].duration` (`HH:MM:SS` → seconds) or
`distance_m` from `.distance`.

The update result returns the saved target, so there's no need to read it
again afterwards.

## What an update can't keep

The `phases` format can't express everything Polar stores. If the target has
any of the following, tell the user what would be lost before updating:

- More than one `exerciseTargets` entry (multi-sport). Only the first sport survives.
- `phaseChangeType: "MANUAL"`. Every phase becomes automatic.
- A recovery phase with a zone. It becomes zone-free.
- A top-level distance phase, or a repeat with more than two phases.

## Name rejections

Polar runs a content filter on target names that sometimes rejects harmless
names with an error on `trainingSessionTarget.name`. Reword the name or add a
short prefix, then retry. A clash with another target at the same minute is
also rejected, so pick another time.
