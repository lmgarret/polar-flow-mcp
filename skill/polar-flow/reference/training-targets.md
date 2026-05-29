# Planned training targets

How to build, edit, and delete planned workouts with `create_training_target`,
`update_training_target`, `get_training_target`, and `delete_training_target`.
This is the most error-prone area of the API — get the units and the `phases`
shape right and everything round-trips.

## Units & formats (non-negotiable)

| Field | Unit / format | Example |
|-------|---------------|---------|
| `date` | ISO `YYYY-MM-DD` | `2026-06-02` |
| `time` | 24h `HH:MM` | `09:00` |
| `distance_m` | **metres** (the Flow UI shows km) | `5000` = 5 km |
| `duration_s` | **seconds** | `600` = 10 min |
| `intensity.hr_zone` | integer 1–5 | `4` |
| `sport_id` | Polar sport id | `1` = running |

The tools never accept km, miles, or `HH:MM:SS`. Convert before calling.

## Two target shapes

`create_training_target` produces one of two shapes depending on `phases`:

- **No `phases`** → an **open VOLUME target**: a named placeholder on the
  calendar with no structured goal. (The tool does not expose a top-level
  distance/duration goal for VOLUME targets — if the user wants a specific
  "run 10 km easy" goal with structure, express it as a single-`repeat` phase,
  otherwise the VOLUME target is just a scheduled slot.)
- **With `phases`** → a **PHASED workout** built from the ordered phase list.

## The `phases` array

`phases` is an ordered list. Each element has a `type` and type-specific fields.

### `warmup` / `cooldown`
A single easy block.

```json
{ "type": "warmup", "duration_s": 600 }
```

- `duration_s` **required**, > 0. Run at easy intensity automatically.

### `repeat`
An interval block repeated `reps` times, with an optional recovery between reps.

```json
{
  "type": "repeat",
  "reps": 5,
  "goal": { "distance_m": 1000 },
  "intensity": { "label": "threshold" },
  "recovery": { "duration_s": 120 }
}
```

- `reps` **required**, **≥ 2**.
- `goal` **required** — set **exactly one** of `distance_m` or `duration_s`.
- `intensity` optional — `{ "label": … }` or `{ "hr_zone": 1-5 }`. `hr_zone`
  wins if both are given. Omit for open intensity. Label→zone mapping is in
  `SKILL.md`.
- `recovery` optional — `{ "duration_s": … }`, an easy block inserted between
  reps.

**There is no standalone "main" phase.** Structured work is only expressible via
`repeat` (`reps ≥ 2`). For a single continuous effort with no intervals, omit
`phases` and use a VOLUME target.

## Worked example — full session

10 min warmup → 5×1 km @ threshold w/ 2 min jog recovery → 10 min cooldown,
running, Tuesday 2026-06-02 at 09:00:

```
create_training_target(
  name = "5x1km Threshold",
  date = "2026-06-02",
  time = "09:00",
  sport_id = 1,
  phases = [
    { "type": "warmup", "duration_s": 600 },
    { "type": "repeat", "reps": 5,
      "goal": { "distance_m": 1000 },
      "intensity": { "label": "threshold" },
      "recovery": { "duration_s": 120 } },
    { "type": "cooldown", "duration_s": 600 }
  ]
)
```

Returns the new numeric target id.

## Editing: the full-replace rule

`update_training_target` **replaces the entire target**. It is not a patch —
fields you omit are cleared, not preserved. Always:

1. `get_training_target(target_id)` to read the current body.
2. Modify the part you want to change in that body.
3. `update_training_target(target_id, …complete body…)`.
4. Re-read with `get_training_target` to confirm the change landed.

**Example — move a session to a different day** (keep everything else):

1. `list_training_targets` → find the `target_id`.
2. `get_training_target(target_id)` → read name, sport, phases, etc.
3. `update_training_target(target_id, name=<same>, date="2026-06-04",
   time=<same>, sport_id=<same>, phases=<same as read>)`.

Do **not** send only `target_id` + `date` — that would wipe the name and
phases.

## Deleting

`delete_training_target(target_id)` is permanent. Confirm with the user, then
call it. A non-existent or foreign id is reported as a no-op, not an error.
