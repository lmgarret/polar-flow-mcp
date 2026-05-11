# Phase 3: Core MCP Tools + Bundled Skill — Research

**Researched:** 2026-05-11
**Domain:** Go MCP tool handlers, Polar AccessLink API (training targets), bundled Claude skill authoring
**Confidence:** MEDIUM (Polar training target API shapes are ASSUMED from v4 spec; write endpoints not confirmed in any public spec)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Training target shapes have NOT been confirmed against a live account. The researcher must fetch the public Polar AccessLink v3 API reference (training targets section) as the authoritative source. Accept that minor shape corrections may be needed during integration testing.
- **D-02:** Wrap each Polar API call in a single `*Client` method (`CreateTrainingTarget`, `ListTrainingTargets`, `DeleteTrainingTarget`). Keep API-facing struct types thin and internal to the `polar` package.
- **D-03:** The MCP tool exposes a **full phases array** — Claude constructs the phases, not the handler. Tool schema: `name`, `sport` (default `RUNNING`), `date` (ISO 8601), `time` (default `18:00`), `phases` array with `type`/`reps`/`goal`/`intensity`/`recovery` per element.
- **D-04:** Warmup and cooldown phases are **optional** in the tool schema. SKILL.md teaches defaults (10 min warmup, 5 min cooldown).
- **D-05:** Multiple distinct repeat blocks in one phases array are supported.
- **D-06:** Intensity label mapping — `easy`→Z1, `aerobic`→Z2, `tempo`→Z3, `threshold`→Z4, `vo2max`→Z5.
- **D-07:** Each phase's `intensity` accepts both `label` (preferred) and `hr_zone` (escape hatch). Handler maps labels to zones internally.
- **D-08:** list/delete follow the same identity extraction pattern as `get_user_info`.
- **D-09:** `list_training_targets` accepts optional `from_date`/`to_date` (defaults: today to +30 days).
- **D-10:** `delete_training_target` accepts `target_id`. Returns confirmation on success; "not found" (not panic) on 404.
- **D-11:** `skill/polar-coach/SKILL.md` structure: trigger description → tool list → HR zone table → worked examples → when NOT to call → safe degradation → installation paths.
- **D-12:** Safe degradation: check server availability before calling any tool.
- **D-13:** Installation paths: (1) Claude Desktop/Code skills directory, (2) Claude.ai project file upload.

### Claude's Discretion

- Output formatting for `list_training_targets` (table, bullets, or summary prose)
- Error message wording for unlinked accounts, not-found targets, Polar API failures
- Exact default values for warmup/cooldown duration in handler (inject defaults or require at least one phase when user omits phases entirely)
- Sport enumeration validation (validate against fixed list or pass through to Polar)

### Deferred Ideas (OUT OF SCOPE)

- Pace-based intensity (`min/km` or `min/mile`) — v2 (INT-01)
- Power-based intensity (watts for cycling) — v2 (INT-02)
- Sport enumeration validation — Claude's discretion (pass-through for v1)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MCP-02 | `create_training_target` tool accepts name, sport, date, time, phases array with type/reps/goal/intensity/recovery | mcp-go `WithArray`/`WithObject` schema; `BindArguments` for nested struct deserialization |
| MCP-03 | Intensity label→HR zone mapping (easy→Z1..vo2max→Z5); constructs Polar API JSON; calls POST /v3/users/{id}/training-targets | Polar v4 schema reveals `PhaseIntensity.lowerZone`/`upperZone` (1–5); endpoint is ASSUMED from CONTEXT.md |
| MCP-04 | `list_training_targets` with optional from/to date defaults | Polar v4 GET /training-target/calendar-targets confirmed; `from_date`/`to_date` params confirmed |
| MCP-05 | `delete_training_target` by target_id; clear not-found error | Endpoint ASSUMED from CONTEXT.md; 404 handling pattern established |
| MCP-06 | All handlers: identity from context → store lookup → decrypt → call Polar; clear error if no linked account | Requires new `store.GetEncryptedToken` method + `cipher` passed to `RegisterTools` |
| MCP-07 | Unexported struct type as context key for user identity | Already in `internal/auth` as `userIDKey{}` — reuse identical pattern |
| SKILL-01 | SKILL.md includes trigger description, 4 tool names, HR zone table (Z1–Z5), warmup/cooldown defaults | Format derived from CONTEXT.md D-06/D-07 table; no framework constraint |
| SKILL-02 | Worked examples: 5×1km threshold session with full phases array; 12-week marathon plan iterative approach | Phase shape from CONTEXT.md `<specifics>`; full example derivable from locked decisions |
| SKILL-03 | Guidance on when NOT to call tools; safe degradation if server unavailable | CONTEXT.md D-12; MCP tool availability check pattern |
| SKILL-04 | Both installation paths documented in SKILL.md | Claude Desktop skills dir + Claude.ai project upload |
</phase_requirements>

---

## Summary

Phase 3 adds three MCP tool handlers (`create_training_target`, `list_training_targets`, `delete_training_target`) and a bundled skill file. The Go implementation follows the exact pattern already established by `GetUserInfoHandler` — an exported closure accepting `*store.Store` (and now also `*crypto.Cipher`) and returning `func(ctx, CallToolRequest) (*CallToolResult, error)`.

The single largest uncertainty is the Polar API itself. **The public Polar AccessLink v3 swagger (`polar.com/accesslink-api/swagger.yaml`) does not contain any training-target write endpoints.** The v4 API (`polar.com/polar-api-v4/swagger.yaml`) confirms read endpoints (`GET /training-target/calendar-targets`, `GET /training-target/favorites`) and the full schema for `TrainingTarget`/`ExerciseTarget`/`PhaseOrRepeat`/`PhaseIntensity`, but also has no POST or DELETE. The CONTEXT.md canonical reference (`POST /v3/users/{user-id}/training-targets`, `DELETE /v3/users/{user-id}/training-targets/{target-id}`) lists endpoints that are not in any public Polar spec found during research. They are assumed to exist at a partner or restricted tier. The v4 spec's schema definitions (particularly `PhaseIntensity` with `lowerZone`/`upperZone` 1–5, and `PhaseGoal` with `DURATION`/`DISTANCE` types) are the best available source for constructing the request body — treat as a close approximation subject to live testing.

A secondary finding: `RegisterTools(s *server.MCPServer, st *store.Store)` currently does not accept a `*crypto.Cipher`. All three new tool handlers must decrypt the stored token before calling Polar, so `RegisterTools` must be updated to accept a cipher parameter. This is a non-breaking change to an internal function — `main.go` already constructs a cipher and can pass it through.

**Primary recommendation:** Build the polar client methods to the v4 schema shapes (best available evidence), keep all struct types in `internal/polar`, and gate them behind package-level vars overridable in tests (same `testexports.go` pattern). Plan for live API validation in the success criteria.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| User identity extraction | MCP tool handler (Go) | auth middleware (injected upstream) | Identity lives in context.Context; handler reads it via `auth.UserIDFromContext` |
| Token retrieval and decryption | MCP tool handler (Go) | store + crypto packages | Store returns encrypted blob; crypto.Cipher.Decrypt produces bearer token |
| Polar API HTTP calls | polar package (Go) | — | `*Client` methods own all HTTP; handlers never call `net/http` directly |
| Training target JSON construction | polar package (Go) | — | Struct types in polar package; handler passes typed request struct |
| HR zone label mapping | mcp package (Go) | — | Pure mapping function; belongs with the tool handler, not the API client |
| MCP tool schema definition | mcp package (Go) | — | `mcp.WithArray`, `mcp.WithObject`, `mcp.WithInputSchema` |
| Claude coaching vocabulary | skill/polar-coach/SKILL.md | — | Markdown file; no compilation dependency |

---

## Standard Stack

### Core (already in go.mod)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/mark3labs/mcp-go` | v0.50.0 | MCP tool registration, schema definition, `CallToolRequest` parameter access | Project-locked; already wired in Phase 1 |
| `modernc.org/sqlite` | v1.50.0 | Pure-Go SQLite driver | Project-locked (CGO disabled) |
| `log/slog` | stdlib | Structured logging in tool handlers | Project convention |
| `net/http` | stdlib | Polar API HTTP client (already in polar.Client) | Project convention; noctx linter requires `NewRequestWithContext` |
| `encoding/json` | stdlib | JSON marshal/unmarshal for Polar request/response bodies | Already used in polar/client.go |

No new dependencies are required for Phase 3. All needed libraries are already present.

**Version verification:** `go.mod` pinned versions confirmed by reading the file directly. [VERIFIED: /var/home/lm/git/polar-flow-mcp/go.mod]

---

## Architecture Patterns

### System Architecture Diagram

```
Claude (LLM)
     |
     | MCP CallToolRequest (phases array + params)
     v
auth.Middleware (already applied — identity in context)
     |
     v
mcp.CreateTrainingTargetHandler(st, cipher)
     |
     +-- auth.UserIDFromContext(ctx)         --> "alice"
     |
     +-- st.GetPolarUserID(ctx, "alice")     --> "12345", found=true
     |
     +-- st.GetEncryptedToken(ctx, "alice")  --> []byte blob
     |
     +-- cipher.Decrypt(blob)               --> "tok-xyz" (bearer token)
     |
     +-- labelToZone(intensity.Label)       --> zone 1-5
     |
     +-- polar.NewClient("tok-xyz")
          |
          v
     .CreateTrainingTarget(ctx, polarUserID, req)
          |
          v
     POST https://www.polaraccesslink.com/v3/users/12345/training-targets
     (JSON body built from phase structs)
          |
          v
     HTTP 201 / error
          |
     mcpgo.NewToolResultText("Training target created: ...")
          |
          v
Claude (displays result to user)
```

### Recommended Project Structure

No new top-level packages. Changes are additive within existing packages:

```
internal/
├── mcp/
│   ├── mcp.go                        # RegisterTools: add 3 new s.AddTool calls; accept cipher param
│   ├── mcp_create_training_target.go # CreateTrainingTargetHandler + labelToZone mapping
│   ├── mcp_list_training_targets.go  # ListTrainingTargetsHandler
│   ├── mcp_delete_training_target.go # DeleteTrainingTargetHandler
│   └── mcp_test.go                   # Existing + new handler tests
├── polar/
│   ├── client.go                     # Existing + CreateTrainingTarget/List/Delete methods
│   ├── client_test.go                # Existing + new method tests
│   └── testexports.go                # Add overridable endpoint vars for new endpoints
├── store/
│   └── store_tokens.go               # Add GetEncryptedToken method
skill/
└── polar-coach/
    └── SKILL.md                      # New top-level directory; no compilation dependency
```

### Pattern 1: Handler Closure with Store + Cipher

All three new handlers follow the same pattern as `GetUserInfoHandler` but add cipher for token decryption and call Polar.

```go
// Source: internal/mcp/mcp.go (existing GetUserInfoHandler pattern)
// Phase 3 extension: add cipher *crypto.Cipher parameter

func CreateTrainingTargetHandler(
    st *store.Store,
    cipher *crypto.Cipher,
) func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        identity, ok := auth.UserIDFromContext(ctx)
        if !ok {
            return mcpgo.NewToolResultError("no identity in context — auth middleware not applied"), nil
        }

        polarUserID, found, err := st.GetPolarUserID(ctx, identity)
        if err != nil {
            return mcpgo.NewToolResultError("database error: " + err.Error()), nil
        }
        if !found {
            return mcpgo.NewToolResultText(
                "No Polar account linked. Visit /oauth/login to link your account.",
            ), nil
        }

        blob, found, err := st.GetEncryptedToken(ctx, identity)
        if err != nil {
            return mcpgo.NewToolResultError("token lookup error: " + err.Error()), nil
        }
        if !found {
            return mcpgo.NewToolResultText("No token stored. Visit /oauth/login to re-link."), nil
        }

        tokenBytes, err := cipher.Decrypt(blob)
        if err != nil {
            return mcpgo.NewToolResultError("token decryption error: " + err.Error()), nil
        }

        // Parse tool params, call polar.NewClient(string(tokenBytes)).CreateTrainingTarget(...)
        // ...
    }
}
```

[CITED: internal/mcp/mcp.go — existing GetUserInfoHandler]

### Pattern 2: store.GetEncryptedToken (new method needed)

The store currently has `UpsertToken` but no read method for tokens. Phase 3 requires a `GetEncryptedToken` method on `*store.Store`:

```go
// Source: pattern derived from store_users.go GetPolarUserID
// Add to internal/store/store_tokens.go

func (s *Store) GetEncryptedToken(ctx context.Context, identity string) ([]byte, bool, error) {
    var blob []byte
    err := s.readDB.QueryRowContext(ctx,
        `SELECT pt.encrypted_token
         FROM polar_tokens pt
         JOIN users u ON u.id = pt.user_id
         WHERE u.identity = ?`,
        identity,
    ).Scan(&blob)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, false, nil
    }
    if err != nil {
        return nil, false, fmt.Errorf("store: get encrypted token: %w", err)
    }
    return blob, true, nil
}
```

[CITED: internal/store/store_users.go — GetPolarUserID query pattern]

### Pattern 3: mcp-go Tool Schema with Array of Objects

For `create_training_target`'s `phases` parameter — an array of heterogeneous objects — use `WithArray` + `Items` with an inline object schema. The alternative `WithInputSchema[T]()` (struct-based) is cleaner for complex nested types.

```go
// Source: Context7 /mark3labs/mcp-go — tool schema definition docs

// Option A: builder API for array of objects
mcp.WithArray("phases",
    mcp.Required(),
    mcp.Description("Ordered list of phases: warmup, repeat blocks, cooldown"),
    mcp.Items(map[string]any{
        "type": "object",
        "properties": map[string]any{
            "type":     map[string]any{"type": "string", "enum": []string{"warmup", "repeat", "cooldown"}},
            "duration_s": map[string]any{"type": "integer", "description": "Duration in seconds (warmup/cooldown)"},
            "reps":     map[string]any{"type": "integer", "description": "Repeat count (repeat type only)"},
            "goal":     map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "distance_m": map[string]any{"type": "number"},
                    "duration_s": map[string]any{"type": "integer"},
                },
            },
            "intensity": map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "label":   map[string]any{"type": "string", "enum": []string{"easy", "aerobic", "tempo", "threshold", "vo2max"}},
                    "hr_zone": map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
                },
            },
            "recovery": map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "duration_s": map[string]any{"type": "integer"},
                },
            },
        },
        "required": []string{"type"},
    }),
)

// Option B: struct-based schema — cleaner for complex types
type PhaseInput struct {
    Type       string          `json:"type" jsonschema:"enum=warmup,repeat,cooldown"`
    DurationS  int             `json:"duration_s,omitempty"`
    Reps       int             `json:"reps,omitempty"`
    Goal       *PhaseGoalInput `json:"goal,omitempty"`
    Intensity  *IntensityInput `json:"intensity,omitempty"`
    Recovery   *RecoveryInput  `json:"recovery,omitempty"`
}
// then: mcp.WithInputSchema[CreateTargetInput]()
// parse with: req.BindArguments(&args)
```

**Recommendation:** Use `req.BindArguments(&args)` with a struct hierarchy (Option B). This produces clean Go code with named fields, avoids map[string]any casts in handler body, and is less error-prone than manual `req.GetString`/`req.GetInt` chaining for nested structures.

[CITED: Context7 /mark3labs/mcp-go — BindArguments and WithInputSchema patterns]

### Pattern 4: Polar API Client Method (training target creation)

```go
// Source: polar/client.go — existing NewClient + method pattern
// ASSUMED shapes based on Polar v4 swagger schema definitions

type CreateTrainingTargetRequest struct {
    Session  TrainingSessionTarget  `json:"session"`
    Exercise []ExerciseTarget       `json:"exercise"`
}

type TrainingSessionTarget struct {
    Name      string    `json:"name"`
    StartTime DateTime  `json:"startTime"`
    SportID   *uint64   `json:"sportId,omitempty"`
}

type DateTime struct {
    Year  int `json:"year"`
    Month int `json:"month"`
    Day   int `json:"day"`
    Hour  int `json:"hour"`
    Min   int `json:"min"`
    Sec   int `json:"sec"`
}

type ExerciseTarget struct {
    Idx          int64         `json:"idx"`
    Type         string        `json:"type"`          // "PHASED"
    PhaseOrRepeat []PhaseOrRepeat `json:"phaseOrRepeat,omitempty"`
}

type PhaseOrRepeat struct {
    Name         string         `json:"name"`
    ChangeType   string         `json:"changeType"` // "MANUAL" or "AUTOMATIC"
    Goal         PhaseGoal      `json:"goal"`
    Intensity    *PhaseIntensity `json:"intensity,omitempty"`
    RepeatCount  *int           `json:"repeatCount,omitempty"`
    PhaseOrRepeat []PhaseOrRepeat `json:"phaseOrRepeat,omitempty"` // child phases for repeats
}

type PhaseGoal struct {
    Type     string   `json:"type"`              // "DURATION" | "DISTANCE" | "MANUAL"
    Duration *int64   `json:"duration,omitempty"` // milliseconds
    Distance *float64 `json:"distance,omitempty"` // meters
}

type PhaseIntensity struct {
    Type       string `json:"type"`       // "HEART_RATE_ZONES" | "NONE"
    LowerZone  *int   `json:"lowerZone,omitempty"` // 1–5
    UpperZone  *int   `json:"upperZone,omitempty"` // 1–5
}

func (c *Client) CreateTrainingTarget(
    ctx context.Context,
    polarUserID string,
    req CreateTrainingTargetRequest,
) (string, error) {
    // POST https://www.polaraccesslink.com/v3/users/{polarUserID}/training-targets
    // Returns created target ID from response or Location header
    // ...
}
```

[ASSUMED: request body field names based on Polar v4 swagger schema — endpoint existence assumed from CONTEXT.md D-01]

### Pattern 5: testexports.go for new Polar endpoints

```go
//go:build polartest

package polar

func SetTrainingTargetsBaseURL(s string) func() {
    orig := trainingTargetsBaseURL
    trainingTargetsBaseURL = s
    return func() { trainingTargetsBaseURL = orig }
}
```

[CITED: internal/polar/testexports.go — existing SetTokenEndpoint pattern]

### Pattern 6: RegisterTools signature update

```go
// Before (Phase 2):
func RegisterTools(s *server.MCPServer, st *store.Store)

// After (Phase 3):
func RegisterTools(s *server.MCPServer, st *store.Store, cipher *crypto.Cipher)
```

Update the single call site in `main.go`:
```go
mcp.RegisterTools(mcpServer, st, cipher)
```

The `cipher` is already constructed in `main.go` at line 53: `cipher := crypto.NewCipher(crypto.NewBytesKeyProvider(cfg.EncryptionKey))`.

[CITED: cmd/polar-flow-mcp/main.go line 53–64]

### Anti-Patterns to Avoid

- **Calling `net/http` directly in mcp package handlers:** Always delegate HTTP to `polar.Client` methods. The `noctx` linter fires if you call `http.Get` or construct requests without a context.
- **Returning `(nil, err)` from tool handlers:** mcp-go tool handlers should return `(mcpgo.NewToolResultError(msg), nil)` for user-visible errors. The `error` return is for framework/transport failures only.
- **Caching the decrypted token:** Per CLAUDE.md and Phase 1/2 decisions, tokens are decrypted on demand and never cached at server or session level.
- **Using `req.Params.Arguments` directly as `map[string]any`:** Use `req.BindArguments(&struct)` or `req.RequireString(...)` / `req.GetString(...)`. Direct map access requires unsafe type assertions that `errcheck` flags.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Nested struct JSON schema for MCP tools | Manual `map[string]any` nesting | `mcp.WithInputSchema[T]()` + `req.BindArguments` | Struct tags produce correct JSON Schema; no cast errors |
| HTTP client with timeout | Bare `http.Client{}` | `*http.Client{Timeout: 30 * time.Second}` already in `polar.NewClient` | Timeout already set; just add new methods to `*Client` |
| Date parsing for `from_date`/`to_date` | Custom parser | `time.Parse("2006-01-02", s)` | Standard library; already the project pattern |
| Label→zone mapping | Switch with string comparisons | Simple `map[string]int` looked up at handler init | Less code, easier to test |

---

## Polar API — Critical Finding

### The Public Spec Gap

**Finding:** Neither the Polar AccessLink v3 swagger (`polar.com/accesslink-api/swagger.yaml`) nor the v4 swagger (`polar.com/polar-api-v4/swagger.yaml`) contains POST or DELETE endpoints for training targets. [VERIFIED: both swagger files fetched]

The v3 spec only exposes `accesslink.read_all` scope and transaction-based exercise/activity endpoints. The v4 spec exposes `training_targets:read` scope and only `GET /training-target/calendar-targets` and `GET /training-target/favorites`.

The CONTEXT.md references:
- `POST /v3/users/{user-id}/training-targets` [ASSUMED: not found in any public spec]
- `GET /v3/users/{user-id}/training-targets` [ASSUMED: v4 uses `/training-target/calendar-targets`]
- `DELETE /v3/users/{user-id}/training-targets/{target-id}` [ASSUMED: not found in any public spec]

These are either partner-tier endpoints, endpoints available only after specific partner approval, or the v3-style user-scoped path variants of the v4 endpoints. Build to these URLs as specified in CONTEXT.md, with the expectation that live testing in Phase 3 success criteria will confirm or correct the shapes.

### v4 Schema Definitions (MEDIUM confidence — confirmed in spec)

The v4 swagger defines these schemas, which are the best available source for the JSON body structure:

**TrainingTarget container:**
```json
{
  "session": { "...TrainingSessionTarget..." },
  "exercise": [ { "...ExerciseTarget..." } ]
}
```

**TrainingSessionTarget required fields:**
- `name` (string, 1–45 chars)
- `startTime` (DateTime object: year/month/day/hour/min/sec)

**ExerciseTarget required fields:**
- `idx` (int64, unique index starting at 0)
- `type` should be `"PHASED"` for interval workouts

**PhaseOrRepeat:**
- `name` (string, required, 1–45 chars)
- `changeType` (required): `"MANUAL"` or `"AUTOMATIC"`
- `goal` (required): `{type: "DURATION"|"DISTANCE"|"MANUAL", duration?: ms, distance?: meters}`
- `intensity` (optional): `{type: "HEART_RATE_ZONES"|"NONE", lowerZone?: 1-5, upperZone?: 1-5}`
- `repeatCount` (2–99, present for repeat nodes)
- `phaseOrRepeat` (child array, required when `repeatCount` is set)

**Key nuance:** A repeat node in the v4 schema works differently from the tool's flat phases array:
- The tool accepts a flat array: `[{type:"warmup",...}, {type:"repeat", reps:5,...}, {type:"cooldown",...}]`
- The Polar API expects a tree: a repeat node has `repeatCount=5` and its `phaseOrRepeat` array contains the child phase(s) to repeat
- The handler must transform the flat tool input into the nested Polar API tree

**DateTime format:** Nested object `{year, month, day, hour, min, sec}` — NOT an ISO 8601 string. [VERIFIED: v4 swagger schema]

**Duration units:** Milliseconds (1000–359,999,000). Tool input is `duration_s` (seconds) — handler must multiply by 1000. [VERIFIED: v4 swagger schema]

**Distance units:** Meters as float (1–9,999,000). Tool input `distance_m` maps directly. [VERIFIED: v4 swagger schema]

**List endpoint:** `GET /training-target/calendar-targets` with `fromDate`/`toDate` query params (ISO 8601 date strings). The v3-style `/v3/users/{id}/training-targets` is ASSUMED based on CONTEXT.md. [VERIFIED: v4 swagger for params; ASSUMED for v3 path]

**Sport IDs:** The v4 spec references `sportId` as `uint64` and points to `GET /sports/list` for valid values. For Phase 3, default to passing through the string sport label (e.g., `"RUNNING"`) or look up the running sport ID from a hardcoded default. [ASSUMED: RUNNING sport ID]

---

## Common Pitfalls

### Pitfall 1: Flat Tool Input vs Nested Polar API Tree

**What goes wrong:** The tool exposes a flat `phases` array for simplicity (warmup, then repeats, then cooldown). The Polar API expects a tree where a repeat node contains its child phases. A handler that naively copies the flat array to `phaseOrRepeat` will send malformed JSON.

**Why it happens:** The friendly tool interface hides the API complexity.

**How to avoid:** The `CreateTrainingTargetHandler` must walk the flat phases array and build the nested tree. For each `repeat` element in the tool input, create a Polar `PhaseOrRepeat` with `repeatCount = reps` and its `phaseOrRepeat` containing the work phase (from `goal` + `intensity`) and, if present, the recovery phase (from `recovery`).

**Warning signs:** Polar API returns 400 with a validation error about `phaseOrRepeat`.

### Pitfall 2: Duration in Milliseconds

**What goes wrong:** Tool input uses `duration_s` (seconds) for human-readable values. The Polar API expects milliseconds. Sending seconds directly creates 1000x too-short phases.

**Why it happens:** Polar uses milliseconds throughout.

**How to avoid:** `polarDurationMs := durationS * 1000` — document this conversion in a const or comment. Test: 10-minute warmup = 600s input → 600,000ms in API body.

### Pitfall 3: RegisterTools Signature Change Breaks main.go

**What goes wrong:** Adding `cipher *crypto.Cipher` to `RegisterTools` causes a compile error in `main.go` if not updated simultaneously.

**Why it happens:** `main.go` calls `mcp.RegisterTools(mcpServer, st)` with two args.

**How to avoid:** Update both `mcp.go` (signature) and `main.go` (call site) in the same task. The cipher is already available at line 53 of `main.go`.

### Pitfall 4: store.GetEncryptedToken Missing

**What goes wrong:** Handlers attempt to call a non-existent `st.GetEncryptedToken(...)` method — compile error.

**Why it happens:** The store was never asked to expose token read access (only upsert exists).

**How to avoid:** Add `GetEncryptedToken(ctx, identity)` to `store_tokens.go` in the same wave as the handler that uses it.

### Pitfall 5: Polar API Endpoint Shapes Not Confirmed

**What goes wrong:** The v3 POST/DELETE endpoints are not in the public spec. The v4 schema shapes may differ from what the v3 API actually accepts. Fields like `changeType` (required in v4 spec) may have different names in the v3 endpoint.

**Why it happens:** Polar has separate v3 and v4 APIs; write access may be partner-tier only.

**How to avoid:** Build to the v4 schema (best available evidence), use testable `httptest.Server` stubs in unit tests, and include a live integration smoke-test as part of Phase 3 success criteria. Accept that field names may need adjustment after first live API call.

### Pitfall 6: linterr — `noctx` on Polar HTTP calls

**What goes wrong:** Adding Polar API calls without threading `ctx` through `http.NewRequestWithContext` fails the `noctx` linter check in CI.

**Why it happens:** CI runs `golangci-lint` with `noctx` enabled.

**How to avoid:** All new `*Client` methods must use `http.NewRequestWithContext(ctx, method, url, body)` — already the pattern in existing `ExchangeCode` and `RegisterUser`.

### Pitfall 7: SKILL.md Tool Name Drift

**What goes wrong:** SKILL.md documents a tool name that differs from the registered name in `mcp.go`. Claude cannot find the tool.

**Why it happens:** Names like `createTrainingTarget` vs `create_training_target` — easy typo.

**How to avoid:** Copy tool names verbatim from the `mcp.NewTool("name", ...)` call into SKILL.md. Write SKILL.md last or cross-check.

---

## Code Examples

### GetEncryptedToken store method

```go
// Source: pattern from internal/store/store_users.go GetPolarUserID
func (s *Store) GetEncryptedToken(ctx context.Context, identity string) ([]byte, bool, error) {
    var blob []byte
    err := s.readDB.QueryRowContext(ctx,
        `SELECT pt.encrypted_token
         FROM polar_tokens pt
         JOIN users u ON u.id = pt.user_id
         WHERE u.identity = ?`,
        identity,
    ).Scan(&blob)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, false, nil
    }
    if err != nil {
        return nil, false, fmt.Errorf("store: get encrypted token: %w", err)
    }
    return blob, true, nil
}
```

### Label-to-zone mapping function

```go
// Source: CONTEXT.md D-06; project-specific
var labelToZone = map[string]int{
    "easy":      1,
    "aerobic":   2,
    "tempo":     3,
    "threshold": 4,
    "vo2max":    5,
}

func resolveZone(intensity *IntensityInput) (int, bool) {
    if intensity == nil {
        return 0, false
    }
    if intensity.HRZone >= 1 && intensity.HRZone <= 5 {
        return intensity.HRZone, true
    }
    if z, ok := labelToZone[intensity.Label]; ok {
        return z, true
    }
    return 0, false
}
```

### Flat-to-tree phase transformation

```go
// Source: CONTEXT.md specifics section (phases array shape) + v4 swagger PhaseOrRepeat schema
// For a repeat-type phase with reps=5, goal={distance_m:1000}, intensity={label:"threshold"},
// recovery={duration_s:120}:

workPhase := polar.PhaseOrRepeat{
    Name:       "work",
    ChangeType: "AUTOMATIC",
    Goal: polar.PhaseGoal{
        Type:     "DISTANCE",
        Distance: ptr(1000.0), // meters
    },
    Intensity: &polar.PhaseIntensity{
        Type:      "HEART_RATE_ZONES",
        LowerZone: ptr(4),
        UpperZone: ptr(4),
    },
}
recoveryPhase := polar.PhaseOrRepeat{
    Name:       "recovery",
    ChangeType: "AUTOMATIC",
    Goal: polar.PhaseGoal{
        Type:     "DURATION",
        Duration: ptr(int64(120 * 1000)), // 120s → 120,000ms
    },
    Intensity: &polar.PhaseIntensity{Type: "NONE"},
}
repeatNode := polar.PhaseOrRepeat{
    Name:         "intervals",
    ChangeType:   "AUTOMATIC",
    Goal:         polar.PhaseGoal{Type: "MANUAL"},
    RepeatCount:  ptr(5),
    PhaseOrRepeat: []polar.PhaseOrRepeat{workPhase, recoveryPhase},
}
```

### mcp-go tool registration with array parameter

```go
// Source: Context7 /mark3labs/mcp-go — WithArray + Items
createTool := mcpgo.NewTool("create_training_target",
    mcpgo.WithDescription("Create a Polar training target with phases..."),
    mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("Session name (e.g., \"5×1km Threshold\")")),
    mcpgo.WithString("date", mcpgo.Required(), mcpgo.Description("Scheduled date (ISO 8601: 2026-05-15)")),
    mcpgo.WithString("sport", mcpgo.Description("Sport type (default: RUNNING)")),
    mcpgo.WithString("time", mcpgo.Description("Scheduled time HH:MM (default: 18:00)")),
    mcpgo.WithArray("phases", mcpgo.Required(), mcpgo.Description("Ordered phases: warmup/repeat/cooldown"),
        mcpgo.Items(map[string]any{
            "type": "object",
            "properties": map[string]any{
                "type":       map[string]any{"type": "string"},
                "duration_s": map[string]any{"type": "integer"},
                "reps":       map[string]any{"type": "integer"},
                "goal":       map[string]any{"type": "object"},
                "intensity":  map[string]any{"type": "object"},
                "recovery":   map[string]any{"type": "object"},
            },
            "required": []string{"type"},
        }),
    ),
)
s.AddTool(createTool, CreateTrainingTargetHandler(st, cipher))
```

---

## State of the Art

| Old Approach | Current Approach | Notes |
|--------------|------------------|-------|
| `req.Params.Arguments["field"].(string)` | `req.GetString("field", "default")` or `req.BindArguments(&s)` | mcp-go v0.50.0 provides typed helpers; direct map access triggers `errcheck` |
| Polar v3 API only | Polar v4 API exists with richer schema definitions | v4 has `training_targets:read` scope; write is not in public spec for either version |
| Global `http.DefaultClient` | Per-request `*http.Client{Timeout:30s}` in `polar.NewClient` | `noctx` linter; already established in this project |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `POST /v3/users/{user-id}/training-targets` exists and accepts a `{session, exercise}` JSON body | Polar API section | Phase 3's `create_training_target` cannot function; endpoint URL or body shape may differ |
| A2 | `DELETE /v3/users/{user-id}/training-targets/{target-id}` exists | Polar API section | `delete_training_target` cannot function |
| A3 | `GET /v3/users/{user-id}/training-targets` with `from`/`to` date params exists (vs v4 path `/training-target/calendar-targets`) | Polar API section | `list_training_targets` may need to use v4 path instead |
| A4 | v3 request body uses the same `{session, exercise, phaseOrRepeat}` nested structure as the v4 schema definitions | Code Examples section | Field names or nesting depth may differ; discoverable in live testing |
| A5 | `changeType: "AUTOMATIC"` is acceptable for all phase nodes | Code Examples section | Device may require `"MANUAL"` for all or specific phase types |
| A6 | Setting `lowerZone == upperZone` for a single HR zone (e.g., Z4→Z4) is valid | Code Examples section | API may require `lowerZone < upperZone` (e.g., Z3→Z5 for "around threshold") |
| A7 | The RUNNING sport is sent as a string `"RUNNING"` in the `sportId` field, or that `sportId` can be omitted for running | Code Examples section | sportId may be a numeric uint64 requiring a `/sports/list` lookup |
| A8 | A successful POST returns the created target's ID in a response body or `Location` header | Architecture diagram | May return 201 with no body, requiring a subsequent GET to fetch the ID |

All items tagged `[ASSUMED]` in the document correspond to rows in this table.

---

## Open Questions

1. **Are the v3 training target write endpoints available to all Polar partners or restricted?**
   - What we know: Neither public swagger spec lists POST or DELETE on training targets
   - What's unclear: Whether these endpoints exist at all, require specific partner tier, or need a different OAuth scope beyond `accesslink.read_all`
   - Recommendation: Attempt the POST in live integration testing as Phase 3 success criterion; if 404, file a blocker and adjust scope

2. **Does `sportId` accept a string (like `"RUNNING"`) or a numeric ID?**
   - What we know: v4 schema defines `sportId` as `uint64`; `/sports/list` exists
   - What's unclear: What the running sport numeric ID is
   - Recommendation: Default to omitting `sportId` (making the exercise sport-agnostic) for v1; or call `/sports/list` once at startup and cache the RUNNING ID

3. **What does the list response body look like for the v3 endpoint?**
   - What we know: v4 defines `trainingtargetLoadTargetsResponse` containing `trainingtargetTrainingTarget` array; the response includes `session.name`, `session.startTime`, exercise structure
   - What's unclear: v3-specific list response wrapper
   - Recommendation: Write list handler to accept both v3 and v4 response shapes; extract name + date for human-readable output

4. **What is the `target_id` format returned/accepted by the API?**
   - What we know: v4 schema defines `session.id` as `uint64`
   - What's unclear: Whether the tool's `target_id` string parameter maps to this field; what the create response returns
   - Recommendation: Store the returned `id` from the create response and pass it as a string to the delete endpoint

---

## Environment Availability

This phase is code-only changes — no new external tools or services beyond the existing Go toolchain.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go 1.26 | Build | Confirmed in go.mod | 1.26.2 | — |
| golangci-lint v2.11 | Lint | Confirmed in CI | v2.11 | — |
| Polar AccessLink API (live) | Integration testing (success criterion) | Unknown — requires linked account | — | Use httptest.Server stubs for unit tests |

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | `go test` (stdlib) |
| Config file | None — build tag `polartest` required |
| Quick run command | `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./internal/mcp/... ./internal/polar/... ./internal/store/...` |
| Full suite command | `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| MCP-02 | `create_training_target` parses phases array and returns result | unit | `go test -tags=polartest ./internal/mcp/ -run TestCreateTrainingTarget` | ❌ Wave 0 |
| MCP-03 | Label→zone mapping (all 5 labels) | unit | `go test -tags=polartest ./internal/mcp/ -run TestLabelToZone` | ❌ Wave 0 |
| MCP-03 | Polar client CreateTrainingTarget sends correct JSON body | unit (httptest stub) | `go test -tags=polartest ./internal/polar/ -run TestCreateTrainingTarget` | ❌ Wave 0 |
| MCP-04 | `list_training_targets` with date defaults | unit | `go test -tags=polartest ./internal/mcp/ -run TestListTrainingTargets` | ❌ Wave 0 |
| MCP-05 | `delete_training_target` success + 404 path | unit | `go test -tags=polartest ./internal/mcp/ -run TestDeleteTrainingTarget` | ❌ Wave 0 |
| MCP-06 | Unlinked account → clear error (not panic), all 3 handlers | unit | `go test -tags=polartest ./internal/mcp/ -run TestNoLinkedAccount` | ❌ Wave 0 |
| MCP-07 | Context key collision cannot occur | unit (implicit in existing auth tests) | `go test -tags=polartest ./internal/auth/` | ✅ exists |
| MCP-06 | GetEncryptedToken returns (nil, false, nil) for missing row | unit | `go test -tags=polartest ./internal/store/ -run TestGetEncryptedToken` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./internal/mcp/... ./internal/polar/... ./internal/store/...`
- **Per wave merge:** `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/mcp/mcp_create_training_target_test.go` — covers MCP-02, MCP-03, MCP-06
- [ ] `internal/mcp/mcp_list_training_targets_test.go` — covers MCP-04, MCP-06
- [ ] `internal/mcp/mcp_delete_training_target_test.go` — covers MCP-05, MCP-06
- [ ] `internal/polar/client_training_targets_test.go` — covers Polar API client methods
- [ ] `internal/store/store_tokens_test.go` (extend existing) — covers GetEncryptedToken

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Handled upstream by auth middleware (Phase 1) |
| V3 Session Management | no | No session state in Phase 3 tools |
| V4 Access Control | yes | `auth.UserIDFromContext` → scoped store/API calls per user; no cross-user data access |
| V5 Input Validation | yes | `req.BindArguments` / `req.RequireString` — reject malformed params early |
| V6 Cryptography | no | Token decryption uses existing `crypto.Cipher` (AES-256-GCM, Phase 1) |

### Known Threat Patterns for MCP Tool Handlers

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| User A calls delete_training_target with User B's target_id | Tampering | Polar API enforces ownership via bearer token; delete with User A's token rejects User B's target (403/404 from Polar) |
| Phases array with 200+ elements causing excessive Polar API payload | DoS | Polar v4 spec says max 200 items in phaseOrRepeat; validate len(phases) ≤ 50 in handler before constructing API body |
| Injecting sport string with unusual characters | Tampering | Pass through to Polar (as per Claude's discretion); Polar API will reject invalid sport IDs with 400 |
| Tool called with no identity in context | Spoofing | Handler checks `auth.UserIDFromContext(ctx)` → returns `mcpgo.NewToolResultError(...)` (existing pattern) |

---

## Project Constraints (from CLAUDE.md)

All directives from `CLAUDE.md` that affect Phase 3 implementation:

| Directive | Impact on Phase 3 |
|-----------|-------------------|
| `CGO_ENABLED=0` throughout | No new CGO dependencies permitted; all new code must be pure Go |
| `modernc.org/sqlite` (NOT `mattn/go-sqlite3`) | No change needed — no new DB work beyond one new read method |
| `log/slog` for logging | Tool handlers and polar client methods must use `slog.Error`/`slog.Warn` (not `fmt.Println`) |
| `golangci-lint v2.11` (gocyclo, godot, misspell, noctx, errcheck type assertions) | `noctx`: all HTTP calls via `NewRequestWithContext`; `errcheck`: assert `.(type)` must be two-return form |
| `http.NewRequestWithContext(ctx, ...)` for all HTTP | All new `*Client` methods on polar.Client must thread ctx |
| Unexported struct type as context key | MCP-07 is already satisfied by `internal/auth.userIDKey{}`; Phase 3 must not introduce a second key type |
| Pre-commit: `golangci-lint run ./...` and `go test -tags=polartest -race -count=1 ./...` | Both must pass before any commit |
| Layout: `internal/{config,auth,store,crypto,polar,oauth,mcp}/` | No new top-level internal packages; skill/ is a new top-level non-Go directory — acceptable |
| `golangci-lint-action@v9`, `version: v2.11` | CI config unchanged for Phase 3 |

---

## Sources

### Primary (HIGH confidence)
- `/var/home/lm/git/polar-flow-mcp/internal/mcp/mcp.go` — GetUserInfoHandler pattern, RegisterTools signature
- `/var/home/lm/git/polar-flow-mcp/internal/polar/client.go` — NewClient, method pattern, noctx compliance
- `/var/home/lm/git/polar-flow-mcp/internal/store/store_tokens.go` — UpsertToken; no GetEncryptedToken yet
- `/var/home/lm/git/polar-flow-mcp/internal/store/store_users.go` — GetPolarUserID pattern to replicate
- `/var/home/lm/git/polar-flow-mcp/internal/auth/auth.go` — UserIDFromContext, userIDKey{}
- `/var/home/lm/git/polar-flow-mcp/internal/crypto/crypto.go` — Cipher.Decrypt
- `/var/home/lm/git/polar-flow-mcp/cmd/polar-flow-mcp/main.go` — cipher construction, RegisterTools call site
- Context7 `/mark3labs/mcp-go` — WithArray, WithObject, WithInputSchema, BindArguments, GetString, RequireString patterns

### Secondary (MEDIUM confidence)
- Polar v4 swagger (`polar.com/polar-api-v4/swagger.yaml`) — TrainingTarget/ExerciseTarget/PhaseOrRepeat/PhaseIntensity/PhaseGoal schema definitions, DateTime format, unit scales (ms/meters), `training_targets:read` scope, GET endpoints

### Tertiary (LOW confidence — ASSUMED, flagged for validation)
- CONTEXT.md D-01, canonical_refs section — POST/DELETE v3 endpoint paths
- `.planning/PROJECT.md` — v3 endpoint paths listed in Context section
- A1–A8 in Assumptions Log above

---

## Metadata

**Confidence breakdown:**
- Standard stack (mcp-go patterns, existing code): HIGH — verified from actual source files
- Architecture (handler/store/cipher wiring): HIGH — derived from existing code
- Polar API shapes: MEDIUM — v4 swagger confirmed; v3 write endpoints ASSUMED
- Polar API endpoint existence: LOW — not found in any public spec

**Research date:** 2026-05-11
**Valid until:** 2026-06-11 (Polar API spec unlikely to change; mcp-go v0.50.0 pinned in go.mod)
