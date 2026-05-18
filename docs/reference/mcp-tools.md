# MCP Tools Reference

This page documents the formal input/output schemas for all four MCP tools exposed by
polar-flow-mcp. For usage examples and coaching vocabulary, see the [Usage guide](../usage.md).

![MCP tool response in Claude](../images/mcp-tool-response.png)

All tools use the MCP StreamableHTTP transport. The user identity is extracted from the
HTTP request context (injected by the auth middleware) — tool call parameters do not
include authentication information.

---

## `get_user_info`

Retrieve the Polar account linked to the current user.

### Input

No parameters required.

```json
{}
```

### Output

**Success:**

```
Polar account linked.
Identity: alice
```

**Not linked:**

```
No Polar account linked to your identity (alice). Visit /oauth/login to link your Polar account.
```

### Error conditions

| Condition | Response |
|-----------|----------|
| Not linked | Text message directing user to `/oauth/login` |
| Database error | MCP error response |

---

## `create_training_target`

> **Not supported.** The Polar v4 Dynamic API is read-only for training targets.
> This tool returns an informational error directing you to create targets in the
> Polar Flow app. The tool definition is retained for forward compatibility.

Create a single scheduled workout in Polar Flow.

### Input schema

```json
{
  "name": "string (required) — workout name, displayed in Polar Flow",
  "date": "string (required) — ISO 8601 date, e.g. 2026-05-21",
  "time": "string (optional) — 24h time, e.g. 18:00. Default: 18:00",
  "sport": "string (optional) — Polar sport name. Default: RUNNING",
  "phases": "array (required) — ordered list of phase objects"
}
```

### Phase types

A workout consists of an ordered array of phases. Three phase types are supported:

#### Warmup phase

```json
{
  "type": "warmup",
  "duration_s": 600
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be `"warmup"` |
| `duration_s` | integer | Yes | Duration in seconds |

#### Repeat phase

```json
{
  "type": "repeat",
  "reps": 5,
  "goal": {
    "distance_m": 1000
  },
  "intensity": {
    "label": "threshold"
  },
  "recovery": {
    "duration_s": 120
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be `"repeat"` |
| `reps` | integer | Yes | Number of repetitions |
| `goal` | object | Yes | Target for each repetition (see below) |
| `intensity` | object | Yes | Intensity specification (see below) |
| `recovery` | object | No | Recovery between reps (see below) |

**Goal object** — specify either distance or duration:

```json
{ "distance_m": 1000 }
```

or

```json
{ "duration_s": 300 }
```

| Field | Type | Description |
|-------|------|-------------|
| `distance_m` | integer | Target distance in meters |
| `duration_s` | integer | Target duration in seconds |

**Intensity object** — specify either a label or a zone number:

```json
{ "label": "threshold" }
```

or

```json
{ "hr_zone": 4 }
```

| Field | Type | Description |
|-------|------|-------------|
| `label` | string | One of: `easy`, `aerobic`, `tempo`, `threshold`, `vo2max` |
| `hr_zone` | integer | HR zone number: 1 (Z1) through 5 (Z5) |

Prefer `label` over `hr_zone` — labels are more readable and map to the same zones.
Use `hr_zone` only when the user explicitly specifies a zone number.

Zone mapping:

| Label | hr_zone |
|-------|---------|
| `easy` | 1 |
| `aerobic` | 2 |
| `tempo` | 3 |
| `threshold` | 4 |
| `vo2max` | 5 |

**Recovery object:**

```json
{ "duration_s": 120 }
```

| Field | Type | Description |
|-------|------|-------------|
| `duration_s` | integer | Recovery duration in seconds |

#### Cooldown phase

```json
{
  "type": "cooldown",
  "duration_s": 300
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Must be `"cooldown"` |
| `duration_s` | integer | Yes | Duration in seconds |

### Complete input example

```json
{
  "name": "5x1km Threshold",
  "date": "2026-05-21",
  "time": "18:00",
  "sport": "RUNNING",
  "phases": [
    {
      "type": "warmup",
      "duration_s": 600
    },
    {
      "type": "repeat",
      "reps": 5,
      "goal": { "distance_m": 1000 },
      "intensity": { "label": "threshold" },
      "recovery": { "duration_s": 120 }
    },
    {
      "type": "cooldown",
      "duration_s": 300
    }
  ]
}
```

Multiple repeat blocks are supported in a single workout:

```json
{
  "name": "Threshold + VO2max ladder",
  "date": "2026-05-22",
  "phases": [
    { "type": "warmup", "duration_s": 600 },
    {
      "type": "repeat", "reps": 3,
      "goal": { "distance_m": 1000 },
      "intensity": { "label": "threshold" },
      "recovery": { "duration_s": 90 }
    },
    {
      "type": "repeat", "reps": 2,
      "goal": { "distance_m": 500 },
      "intensity": { "label": "vo2max" },
      "recovery": { "duration_s": 180 }
    },
    { "type": "cooldown", "duration_s": 300 }
  ]
}
```

### Output

**Success:**

```json
{
  "target_id": "abc123",
  "message": "Training target created successfully."
}
```

The `target_id` is a Polar-assigned identifier. Save it if you may need to delete the
target later.

### Error conditions

| Condition | Response |
|-----------|----------|
| No Polar account linked | Text: "No Polar account linked. Visit /oauth/login." |
| Invalid date format | MCP error: invalid date |
| Unknown intensity label | MCP error: invalid label |
| Polar API error | MCP error with Polar's error message |

---

## `list_training_targets`

List upcoming training targets in Polar Flow for a date range.

### Input schema

```json
{
  "from_date": "string (optional) — ISO 8601 date. Default: today",
  "to_date": "string (optional) — ISO 8601 date. Default: today + 30 days"
}
```

Both parameters are optional. Omitting both returns all targets in the next 30 days.

### Complete input example

```json
{
  "from_date": "2026-05-12",
  "to_date": "2026-05-19"
}
```

### Output

**Success with results:**

```
Training targets from 2026-05-12 to 2026-05-19:

1. 5x1km Threshold — 2026-05-15 (RUNNING)
   Target ID: abc123

2. Easy 45 min — 2026-05-17 (RUNNING)
   Target ID: def456
```

**No results:**

```
No training targets found between 2026-05-12 and 2026-05-19.
```

### Error conditions

| Condition | Response |
|-----------|----------|
| No Polar account linked | Text: "No Polar account linked. Visit /oauth/login." |
| Invalid date format | MCP error: invalid date |
| Polar API error | MCP error with Polar's error message |

---

## `delete_training_target`

> **Not supported.** The Polar v4 Dynamic API is read-only for training targets.
> This tool returns an informational error directing you to delete targets in the
> Polar Flow app. The tool definition is retained for forward compatibility.

Delete a training target from Polar Flow by its ID.

### Input schema

```json
{
  "target_id": "string (required) — the Polar target ID to delete"
}
```

### Complete input example

```json
{
  "target_id": "abc123"
}
```

### Output

**Success:**

```
Training target abc123 deleted successfully.
```

**Not found:**

```
Training target abc123 not found (may have already been deleted).
```

The "not found" response is informational, not an error — the target may have been
deleted through the Polar app or by a previous tool call.

### Error conditions

| Condition | Response |
|-----------|----------|
| No Polar account linked | Text: "No Polar account linked. Visit /oauth/login." |
| Target not found | Informational text (not an MCP error) |
| Polar API error | MCP error with Polar's error message |
