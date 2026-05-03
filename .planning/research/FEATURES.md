# Features Research: polar-flow-mcp

**Domain:** Polar AccessLink training targets + MCP tool interface design
**Researched:** 2026-05-03
**Confidence:** MEDIUM-HIGH — Polar API shapes from public docs + OAuth flow confirmed; MCP tool interface design is a recommendation, not spec-verified.

---

## Table Stakes (v1 — must ship)

These are the minimum features that make the project useful at all.

| Feature | Why table stakes |
|---|---|
| `create_training_target` MCP tool | Core value — the whole point |
| Phased structure: warmup → repeats → cooldown | Polar's training model; all structured workouts use this |
| Intensity by HR zone (Z1–Z5) | Most common targeting method for endurance athletes |
| `list_training_targets` MCP tool | Can't use the tool blind — need to see what exists |
| `delete_training_target` MCP tool | Need to fix mistakes Claude makes |
| `get_user_info` MCP tool | Users need to verify which Polar account is linked |
| Per-user OAuth link flow | Multi-user is a stated requirement from day 1 |
| `/oauth/login` browser redirect | Entry point for linking |
| `/oauth/callback` token exchange + registration | Completes the link |

---

## Differentiators (v2+)

Features that add value but are not required for the project to be useful.

| Feature | Notes |
|---|---|
| Intensity by pace (min/km or min/mile) | Running-specific; adds precision for pace-based training |
| Intensity by power (watts) | Cycling-specific; requires power meter |
| Favorite targets (saved templates) | Polar supports `GET /training-target/favorites` |
| Multi-sport (swim, bike, run, cross-training) | Polar has sport IDs; v1 can default to running |
| Recurring target series | Not in Polar API; would require client-side sequencing |
| Token refresh / re-link UX | Tokens don't expire; low priority |
| Prometheus metrics endpoint | Nice for homelab observability |
| Key rotation (ENCRYPTION_KEY v2) | Schema ready; implementation is v2+ |

---

## Anti-Features (deliberately not building)

| Feature | Why excluded |
|---|---|
| Reading past training sessions | Out of scope — project writes targets, not reads sessions |
| Training load / recovery analytics | Different API surface (Training Data API); no write capability |
| Polar account creation | Users must already have a Polar account |
| Workout sync / push from Polar | No webhook support; would require polling |
| Web UI for managing targets | Claude conversation is the UX surface |
| Built-in user authentication | Proxy contract only; no login forms |

---

## Polar API Shapes (v3)

### OAuth Token Response

`POST https://polarremote.com/v2/oauth2/token`

```json
{
  "access_token": "<opaque bearer token>",
  "token_type": "bearer",
  "x_user_id": 12345678
}
```

No expiry. Store `access_token` (encrypted) and `x_user_id` (as `polar_user_id`).

### User Registration

`POST https://www.polaraccesslink.com/v3/users`
Header: `Authorization: Bearer <access_token>`

```json
{ "member-id": "alice@example.com" }
```

- 200: registered successfully
- 409: already registered (treat as success, log at DEBUG)

Response includes `polar-user-id` — store this as `polar_user_id` in the users table.

### Create Training Target

`POST https://www.polaraccesslink.com/v3/users/{polar-user-id}/training-targets`
Header: `Authorization: Bearer <access_token>`

```json
{
  "name": "5×1km Threshold",
  "sport": "RUNNING",
  "scheduledDate": "2026-05-08",
  "scheduledTime": "18:00",
  "comment": "Created by Claude",
  "phases": [
    {
      "name": "Warmup",
      "repeatCount": 1,
      "goal": { "type": "DURATION", "durationInSeconds": 600 },
      "intensity": {
        "intensityType": "HEART_RATE_ZONE",
        "lowerZone": 1,
        "upperZone": 2
      }
    },
    {
      "name": "Repeat",
      "repeatCount": 5,
      "goal": { "type": "DISTANCE", "distanceInMeters": 1000 },
      "intensity": {
        "intensityType": "HEART_RATE_ZONE",
        "lowerZone": 4,
        "upperZone": 4
      },
      "recovery": {
        "goal": { "type": "DURATION", "durationInSeconds": 120 },
        "intensity": {
          "intensityType": "HEART_RATE_ZONE",
          "lowerZone": 1,
          "upperZone": 2
        }
      }
    },
    {
      "name": "Cooldown",
      "repeatCount": 1,
      "goal": { "type": "DURATION", "durationInSeconds": 600 },
      "intensity": {
        "intensityType": "HEART_RATE_ZONE",
        "lowerZone": 1,
        "upperZone": 2
      }
    }
  ]
}
```

Key fields:
- `sport`: `RUNNING`, `CYCLING`, `STRENGTH_TRAINING`, `OTHER` — default to `RUNNING` in v1
- `scheduledDate`: ISO 8601 date (`YYYY-MM-DD`)
- `scheduledTime`: `HH:MM` — default `18:00` per Polar convention
- Phase `goal.type`: `DURATION` (seconds) or `DISTANCE` (meters) or `FREE`
- Phase `intensity.intensityType`: `HEART_RATE_ZONE`, `PACE`, `POWER` (v2+)
- `lowerZone` / `upperZone`: 1–5 for HR zone intensity

### List Training Targets

`GET https://www.polaraccesslink.com/v3/users/{polar-user-id}/training-targets`
Optional query params:
- `from`: ISO 8601 date (filter upcoming targets)
- `to`: ISO 8601 date

Returns an array of training target objects (same structure as create).

### Delete Training Target

`DELETE https://www.polaraccesslink.com/v3/users/{polar-user-id}/training-targets/{target-id}`

Returns 204 No Content on success.

---

## Polar HR Zone Definitions (Z1–Z5)

Polar uses heart rate zones as a percentage of maximum heart rate. The zones are:

| Zone | Name | % HRmax | Training language |
|------|------|---------|-------------------|
| Z1 | Very light | 50–60% | Recovery, easy, warm-up/cool-down |
| Z2 | Light | 60–70% | Aerobic base, long slow distance, fat burning |
| Z3 | Moderate | 70–80% | Aerobic / tempo, "comfortably hard", marathon pace |
| Z4 | Hard | 80–90% | Threshold, lactate threshold, 10K race pace |
| Z5 | Maximum | 90–100% | VO2max, intervals, race effort |

**Common training language → zone mapping:**
- "easy" / "recovery" / "jog" → Z1–Z2
- "aerobic" / "base" / "long run" → Z2
- "tempo" / "marathon pace" → Z3
- "threshold" / "LT" / "comfortably hard" → Z4
- "VO2max" / "hard intervals" / "race pace (5K)" → Z5

The bundled Claude skill should include this mapping so Claude can resolve coaching language to Polar zones without asking the user to specify zone numbers.

---

## MCP Tool Interface Recommendations

### Design principle: coaching language, not raw API fields

Claude users say "5×1km threshold with 2 min recovery" — not `"intensityType": "HEART_RATE_ZONE", "lowerZone": 4`. The tool schema should accept coaching language where possible and do the mapping internally.

### `create_training_target` — recommended input schema

```
name (string, required): Session name, e.g. "5×1km Threshold"
sport (string, optional, default "running"): Sport type
scheduled_date (string, required): Date in YYYY-MM-DD or natural language
scheduled_time (string, optional, default "18:00"): Time in HH:MM

phases (array, required): List of phases
  Each phase:
    type (string): "warmup" | "main" | "cooldown" | "repeat"
    repeat_count (integer, optional, default 1): Number of repeats (for "repeat" type)
    duration_seconds (integer, optional): Duration of this phase
    distance_meters (integer, optional): Distance of this phase
    zone (integer, optional, 1-5): HR zone for intensity
    intensity_label (string, optional): "easy"|"aerobic"|"tempo"|"threshold"|"vo2max"
    recovery (object, optional): Recovery phase between repeats
      duration_seconds (integer)
      zone (integer, 1-5)
```

The tool handler maps `intensity_label` to zone number using the zone table above, then builds the Polar API request.

### `list_training_targets` — recommended input schema

```
from_date (string, optional): Show targets from this date (default: today)
to_date (string, optional): Show targets up to this date (default: +30 days)
```

Returns a human-readable list of upcoming targets.

### `delete_training_target` — recommended input schema

```
target_id (string, required): Polar training target ID to delete
```

Returns confirmation message.

### `get_user_info` — no required input

Returns: Polar username/email, linked status, and `polar_user_id`.

---

## Feature Complexity Notes

| Feature | Complexity | Dependencies |
|---|---|---|
| OAuth link flow | Medium | KeyProvider, store, polar client |
| Token encryption/decryption | Low | crypto package, store schema |
| `create_training_target` | Medium | All phases, zone mapping, Polar API shape |
| `list_training_targets` | Low | Polar list API, date filtering |
| `delete_training_target` | Low | Polar delete API |
| `get_user_info` | Low | Store lookup |
| Pace intensity (v2) | Low | Polar pace field schema |
| Power intensity (v2) | Low | Polar power field schema |
| Multi-sport support (v2) | Low | Sport ID enum expansion |

---

## Dependencies Between Features

- OAuth flow must exist before any MCP tool works (no linked account = all tools fail)
- Token encryption is a schema-level decision that cannot change after data is written
- Zone mapping logic is shared by all phases of `create_training_target`
- `get_user_info` is the simplest tool and makes the best first integration test
