# Building and editing planned workouts

## Contents
- Choosing a shape
- Intensity
- Editing: turning the read-back into update arguments
- What an update can't do

## Choosing a shape

| The user asks for… | Send to `create_training_target` |
|---|---|
| A slot with no structure ("run Sunday") | No goal at all (no `duration_s`, `distance_m`, `kcal` or `phases`): a FREE target |
| One total goal ("35 min easy", "10 km", "burn 500 kcal") | Exactly one of `duration_s`, `distance_m` or `kcal` (VOLUME) |
| A time for a distance ("10 km in 45 min", "sub-1:45 half") | Both `duration_s` and `distance_m`: a race-pace target the watch paces |
| One continuous zoned effort ("40 min at threshold", "8 km at tempo") | A single `block` phase with `duration_s` or `distance_m`, `intensity` and a custom `name` |
| A run or race split into segments ("km 1–5 in Z3, km 5–15 in Z1–Z3, then free") | One `block` per segment, each with its own `distance_m` (or `duration_s`) and `intensity` |
| Structured intervals | `warmup` → `repeat` (`goal` + optional `recovery`) → `cooldown` |
| Intervals with more than work + rest, or sets of intervals ("3 × (400 m, 2 × 30/30)") | `repeat` with its own `phases` list, which may hold one more `repeat` — never as its last phase |
| A rest that ends when the athlete is ready | That phase with `manual: true` |
| Several sports in one session (brick, duathlon) | `exercises`, one entry per sport with its own goal; all sports need the same kind of goal |

`warmup`, `block` and `cooldown` are the same thing to Polar: one continuous
phase with exactly one goal, `duration_s` or `distance_m`, at the phase's own
level. They differ only in their default name, so use them in any order and
as many times as needed. A `repeat` takes either its own `phases` or the
`goal` / `intensity` / `recovery` shorthand.

## Intensity

Each phase takes at most one of `hr_zone`, `speed_zone`, `power_zone` (all zone
numbers 1–5) or `label`. Pass a label when the user speaks in effort terms. Use
`hr_zone` only when they name a zone number. For a range ("Z1 to Z3", "up to
Z4 from Z3"), add `zone_max`: `{"hr_zone": 1, "zone_max": 3}`. Labels map to
HR zones:

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

| Read back | Send |
|---|---|
| `type: "FREE"` | No goal arguments |
| `type: "VOLUME"` | The one set field of the exercise target: `duration_s` from `duration` (`HH:MM:SS` → seconds), `distance_m` from `distance`, or `kcal` from `calories` |
| `type: "STEADY_RACE_PACE"` | `duration_s` and `distance_m` (and `kcal` if `calories` is set) |
| `type: "PHASED"` | `phases`, translated as below (and `kcal` if `calories` is set). **Ignore** the exercise target's `duration` / `distance`: Polar totals them from the phases |
| Top-level `phaseType: "PHASE"` | `block` (or `warmup` / `cooldown`, they are equivalent) with `name`, and `distance_m` or `duration_s` according to `goalType`, at the phase's own level |
| `phaseType: "REPEAT"`, `repeatCount: N` | `repeat`, `reps: N`, and a `phases` list translating each of its phases the same way (a nested `REPEAT` becomes a nested `repeat`) |
| `goalType: "DISTANCE"` | `distance_m` = `distance`. **Ignore** the `"00:00:00"` duration Polar adds to distance phases |
| `goalType: "DURATION"` | `duration_s` = `duration` in seconds |
| `phaseChangeType: "MANUAL"` | `manual: true` |
| `intensityType` `HEART_RATE_ZONES` / `SPEED_ZONES` / `POWER_ZONES`, `lowerZone` = `upperZone` = Z | `hr_zone` / `speed_zone` / `power_zone`: Z |
| Same, `lowerZone` = L < `upperZone` = U | `hr_zone` / `speed_zone` / `power_zone`: L, `zone_max`: U |
| `intensityType: "NONE"` (zones read back as `0`) | Omit `intensity` |
| Several `exerciseTargets` | `exercises`, one entry per exercise target **in the same order**, each with `sport_id` from `sportId` and its goal translated as above |

Always decide whether a phase is a distance or duration goal from `goalType`,
never from which field is filled in. Also carry over the top-level `name`,
`description`, the sport (`exerciseTargets[0].sportId` → `sport_id` for a
single sport) and the date and time (`datetime`).

The update result returns the saved target, so there's no need to read it
again afterwards.

## What an update can't do

Polar's update can neither add nor remove a sport: it keeps the sports it
isn't sent and ignores new ones. The tool therefore refuses an update with a
different number of sports. To change which sports a target has, tell the
user, then delete the target and create it again.

## Name rejections

Polar runs a content filter on target names that sometimes rejects harmless
names with an error on `trainingSessionTarget.name`. Reword the name or add a
short prefix, then retry. A clash with another target at the same minute is
also rejected, so pick another time.
