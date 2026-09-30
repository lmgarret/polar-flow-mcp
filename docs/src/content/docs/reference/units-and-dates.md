---
title: Units & dates
description: The one canonical unit and date contract every MCP tool speaks, and how the adapter reconciles the many encodings the Polar Flow web API uses under the hood.
sidebar:
  order: 5
---

The Polar Flow web API is internally inconsistent: the same concept arrives in
different units, encodings, and field names depending on which endpoint you hit.
polar-flow-mcp hides this behind a single **adapter layer** (`internal/convert`)
so every MCP tool — its arguments, its results, and the MCP-app payloads — speaks
one canonical contract.

## Canonical contract

| Concept | Canonical form | Field suffix |
|---------|----------------|--------------|
| Duration | integer **seconds** | `*_s` |
| Distance | **metres** (number) | `*_m` |
| Speed | **km/h** (number) | `*_kmh` |
| Pace | integer **seconds per km** | `*_s_per_km` |
| Power | integer **watts** | `*_w` |
| Heart rate | integer **bpm**; omitted when absent | `hr_*` |
| Date (only) | ISO 8601 `YYYY-MM-DD` | `*_date` |
| Datetime | ISO 8601 (`start_time`, RFC3339 or tz-less) | `*_time` / `*_at` |
| Sport | numeric `sport_id` (+ `sport_name` on output) | — |
| Sport category | one of ~20 UI category keys (`run`, `cycle`, …, `generic`) | `sport_category` |
| Session feeling | integer **1** (bad) – **5** (great); `null` when unset | `feeling` |
| Coordinates | WGS84 degrees `lat` / `lon`, altitude in metres | `altitude_m` |
| Absent value | omitted / JSON `null` — never `""`, `-1`, or `" "` | — |

If you are writing a new tool, keep to these. State the unit explicitly in every
parameter description.

## What the adapter reconciles

The wire API is the source of these divergences — the adapter absorbs them so you
never see them:

**Durations** arrive in five encodings: integer seconds (`create` bodies),
integer milliseconds (`list_training_sessions`), `HH:MM:SS` strings
(training-target phases), ISO-8601 `PTxxM` (`get_training_session_summary`), and
a `StandardDuration` object (`get_progress_summary`). All are normalised to
seconds on output; inputs are encoded to whatever the target endpoint wants.

**Dates** arrive in ~eight encodings: `D.M.YYYY` (calendar endpoints),
`DD-MM-YYYY` (progress endpoints — a genuinely different form, not
interchangeable with the calendar one), `YYYY-MM-DD`, tz-less ISO, `Z`-suffixed
ISO, offset ISO, space-separated ISO, and Unix epoch seconds/millis. Tool
arguments are always ISO `YYYY-MM-DD` (plus `HH:MM` where a time is needed); the
adapter renders the wire form each endpoint requires.

**Feeling** is stored on an inverted 0–1 scale sent as a string on write
(`"0.19"` = great … `"0.99"` = bad) and read back as a number; the adapter maps
it to and from the 1–5 rating.

**Favorites** add three more encodings of the same numbers: `/api/favorites`
reports durations in milliseconds, a favorite's own read-back uses `HH:MM:SS`,
and scheduling a favorite answers with distance and duration as *strings*
(metres and milliseconds, `"0"` meaning unset) and the date as `DD-MM-YYYY`.
Scheduling also needs its own datetime form, `YYYY-MM-DDTHH:MM:SS.sss+00:00`,
whose offset Polar ignores.

**Activity and sleep** add minutes: the 24/7 activity timeline reports active
time, sleep and every intensity band in minutes (converted to seconds), step
distance in metres, the daily goal as a percentage, and intraday samples as
epoch milliseconds that encode the *local* wall clock (rendered as `HH:MM`).
Days without device data arrive as zeros and are reported as `null`. Sleep
times carry an offset whose local wall clock is kept, plus separate
second offsets to the moments of falling asleep and waking.

**Routes** are uploaded as trackpoints with a cumulative distance in metres
(haversine for GPX, the file's own distances for TCX) and read back as
waypoints with a synthesized per-point sequence number, which the adapter drops.

**Units** differ per endpoint: distance is metres almost everywhere (the Flow UI
shows km — 5 km on screen is `5000` on the wire), speed is km/h in one place and
m/s in another, heart rate is an integer-as-string (`""` when unset) on write but
a nullable integer on read. Field names drift too — `hrAverage` vs `hrAvg`,
`kiloCalories` vs `calories`, `sportId` vs `sport` vs `sportName`.

## Where it lives

- `internal/convert/units.go` — pure primitives (duration, date, and datetime
  converters; the intensity-label → HR-zone map; sentinel cleaners), each unit-tested.
- `internal/convert/dto.go` — the canonical response DTOs and their `gen.*` → DTO
  mappers, one per normalised read tool.
- `internal/convert/dto_favorites.go` / `dto_sessions.go` — favorite, route,
  scheduled-target and session-edit mappers (including the favorite copy of a
  training target and the session edit body built from the live summary).
- `internal/convert/dto_zones.go` / `dto_activity.go` / `dto_sleep.go` — zones
  and zone edits, daily activity, and sleep nights (stage totals and the
  hypnogram, derived the way Flow's own sleep report does).
- `internal/convert/route.go` — the GPX/TCX parser and route validation used by
  `import_route`.
- `internal/convert/sport.go` — `SportCategory(name, id)`, the single source of
  truth mapping any of Polar's ~150 sports to one of ~20 UI categories
  (keyword-first on the sport name, resilient to new sports; stable id hints as a
  fallback). It feeds `sport_category` on the session DTOs and target list, and
  the `sport_categories` name→category lookup on the progress summary, so the
  MCP-app UIs read a field instead of re-deriving the classification. The apps
  own only the presentation (SVG glyph, colour, label) per category. Unit-tested.

The two hand-written trimmed structs in `internal/flow` — `UserInfo` and
`CalendarTarget` — predate this package and already follow the same pattern.

## Scope

Read normalisation currently covers the headline fields of
`list_training_sessions`, `get_training_session_summary`,
`get_progress_summary`, the favorites tools (`list_favorites`, `get_favorite`,
`schedule_favorite`) and `get_route`. Deep, partly-unpinned payloads — training-target phase
trees (`get_training_target`), session lap/sample detail
(`get_training_session_details`), the week-summary strip, and the progress
breakdown lists — are passed through from the wire unchanged. `get_calendar_events`
is intentionally the raw, unfiltered escape hatch.
