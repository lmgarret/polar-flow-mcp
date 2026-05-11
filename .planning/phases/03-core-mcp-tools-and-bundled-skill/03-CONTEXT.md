# Phase 3: Core MCP Tools + Bundled Skill - Context

**Gathered:** 2026-05-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver three MCP tools (`create_training_target`, `list_training_targets`, `delete_training_target`) that let a proxy-authenticated user manage Polar Flow training targets directly from Claude, plus a bundled skill file (`skill/polar-coach/SKILL.md`) that teaches Claude the tool vocabulary, HR zone mapping, and usage patterns without any user configuration.

Delivers: MCP-02, MCP-03, MCP-04, MCP-05, MCP-06, MCP-07, SKILL-01, SKILL-02, SKILL-03, SKILL-04.

</domain>

<decisions>
## Implementation Decisions

### Polar API Shapes

- **D-01:** Training target shapes have NOT been confirmed against a live account. The researcher must fetch the public Polar AccessLink v3 API reference (training targets section) as the authoritative source. Accept that minor shape corrections may be needed during integration testing.
- **D-02:** Wrap each Polar API call in a single `*Client` method (`CreateTrainingTarget`, `ListTrainingTargets`, `DeleteTrainingTarget`). Keep API-facing struct types thin and internal to the `polar` package. If real shapes differ from spec, only the client method and its internal struct need updating — tool handlers are untouched.

### create_training_target Parameter Design

- **D-03:** The MCP tool exposes a **full phases array** — Claude constructs the phases, not the handler. The tool parameter schema includes: `name`, `sport` (default `RUNNING`), `date` (ISO 8601), `time` (default `18:00`), and a `phases` array where each element specifies: `type` (`warmup`/`repeat`/`cooldown`), `reps` (for repeat type), `goal` (`distance_m` or `duration_s`), `intensity` (`label` or `hr_zone`), optional `recovery` (`duration_s`).
- **D-04:** Warmup and cooldown phases are **optional** in the tool schema. SKILL.md teaches Claude the sensible defaults (10 min warmup, 5 min cooldown) and instructs it to include them unless the user says otherwise.
- **D-05:** Multiple distinct repeat blocks in one phases array are supported (e.g., 3×1km Z4 then 2×500m Z5 in the same workout). The handler maps each repeat-type phase to a distinct block in the Polar payload.

### Intensity Label → HR Zone Mapping

- **D-06:** Clean 1:1 mapping, no overlaps:
  - `easy` → Z1
  - `aerobic` → Z2
  - `tempo` → Z3
  - `threshold` → Z4
  - `vo2max` → Z5
- **D-07:** Each phase's `intensity` field accepts **both** a label (`label: "threshold"`) and a zone number (`hr_zone: 4`). Handler maps labels to zones internally. Labels are the preferred vocabulary (taught in SKILL.md); zone numbers are an accepted escape hatch.

### list_training_targets and delete_training_target

- **D-08:** These follow the same identity extraction and token lookup pattern as `get_user_info` (MCP-06): `auth.UserIDFromContext(ctx)` → `st.GetPolarUserID` → decrypt token → call Polar. Clear error returned (not panic) if no linked account.
- **D-09:** `list_training_targets` accepts optional `from_date` / `to_date` (defaults: today to +30 days). Claude's discretion on output formatting (human-readable text that Claude can relay or reformat as needed).
- **D-10:** `delete_training_target` accepts `target_id`. Returns a confirmation message on success; a clear "not found" message (not a server panic) on 404 from Polar.

### Bundled Skill File

- **D-11:** `skill/polar-coach/SKILL.md` gives **equal weight** to the HR zone/label table and worked examples. Structure: trigger description → tool list with purposes → HR zone table (Z1–Z5 with coaching vocabulary) → worked examples (5×1km threshold session with full phases array, marathon-plan iterative approach) → when NOT to call tools → safe degradation guidance → installation paths.
- **D-12:** Safe degradation: SKILL.md instructs Claude to check whether the polar-flow-mcp server is connected before calling any tool. If unavailable, tell the user and link to the deployment docs. Do not attempt tool calls against a missing server.
- **D-13:** Installation paths documented in SKILL.md: (1) Claude Desktop/Code skills directory, (2) Claude.ai project file upload.

### Claude's Discretion

- Output formatting for `list_training_targets` (human-readable text structure — table, bullets, or summary prose)
- Error message wording for unlinked accounts, not-found targets, and Polar API failures
- Exact default values for warmup/cooldown duration in handler (if user omits phases entirely — whether to inject defaults or require at least one phase)
- Sport enumeration validation (whether to validate against a fixed list or pass through to Polar)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project constraints and prior decisions
- `.planning/PROJECT.md` — tech stack, key decisions table, locked decisions from Phases 1–2, Polar AccessLink API endpoints
- `.planning/REQUIREMENTS.md` — MCP-02..07, SKILL-01..04 (full requirement text)
- `CLAUDE.md` — critical decisions table; CGO disabled, pure Go, driver import paths, context key pattern, SQLite dual-pool

### Polar AccessLink v3 API
- Public docs: `https://www.polar.com/accesslink-api/#training-targets` — researcher MUST fetch this; shapes not confirmed live, so the reference is authoritative
- Endpoints:
  - Create: `POST /v3/users/{user-id}/training-targets`
  - List: `GET /v3/users/{user-id}/training-targets`
  - Delete: `DELETE /v3/users/{user-id}/training-targets/{target-id}`

### Existing code to read before planning
- `internal/mcp/mcp.go` — `RegisterTools` signature and `GetUserInfoHandler` pattern to replicate for new tools
- `internal/polar/client.go` — `Client` struct and `NewClient(bearerToken)` pattern; add `CreateTrainingTarget`, `ListTrainingTargets`, `DeleteTrainingTarget` as methods
- `internal/store/store.go`, `internal/store/store_users.go`, `internal/store/store_tokens.go` — `GetPolarUserID` and `GetToken` methods; no new store methods needed for Phase 3 (training targets not persisted locally)
- `internal/auth/auth.go` — `UserIDFromContext` for identity extraction
- `internal/crypto/crypto.go` — `Decrypt` for token decryption before Polar API calls

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/auth.UserIDFromContext(ctx)` — identity extraction; use identically in all three new tool handlers
- `internal/store.Store.GetPolarUserID(ctx, identity)` — Polar user ID lookup for building API URLs
- `internal/crypto.Decrypt(key, blob)` — token decryption on demand; same pattern as `get_user_info`
- `internal/polar.NewClient(bearerToken)` — create per tool invocation; add new methods to `*Client`
- `mcpgo.NewToolResultError(...)` / `mcpgo.NewToolResultText(...)` — established return conventions

### Established Patterns
- Unexported struct type as context key for user identity (MCP-07 requirement; already in `internal/auth`)
- `RegisterTools(s *server.MCPServer, st *store.Store)` — extend with three new `s.AddTool(...)` calls
- Tool handler exported as `XxxHandler(st *store.Store) func(...)` — matches `GetUserInfoHandler` pattern; enables direct test invocation without live transport
- Polar client methods use `http.NewRequestWithContext(ctx, ...)` to satisfy `noctx` linter

### Integration Points
- `internal/mcp/mcp.go` `RegisterTools()` — add all three tools here; no changes to `main.go` needed (already wires `RegisterTools`)
- `internal/polar/client.go` — add `CreateTrainingTarget`, `ListTrainingTargets`, `DeleteTrainingTarget` as methods on `*Client`
- `skill/polar-coach/SKILL.md` — new top-level directory; no compilation or test dependency

</code_context>

<specifics>
## Specific Ideas

- Phases array shape Claude uses (from discussion):
  ```
  phases: [
    {type: "warmup", duration_s: 600},
    {type: "repeat", reps: 5, goal: {distance_m: 1000}, intensity: {label: "threshold"}, recovery: {duration_s: 120}},
    {type: "cooldown", duration_s: 300}
  ]
  ```
- Intensity mapping table for SKILL.md: easy=Z1, aerobic=Z2, tempo=Z3, threshold=Z4, vo2max=Z5 — no overlaps
- Multiple repeat blocks supported in a single phases array (complex sessions)
- Skill safe degradation: check server availability first, then call tools; if unavailable, tell user + link to deployment docs

</specifics>

<deferred>
## Deferred Ideas

- **Pace-based intensity** (`min/km` or `min/mile`) — explicitly listed as v2 (INT-01 in REQUIREMENTS.md); do not implement in Phase 3
- **Power-based intensity** (watts for cycling) — v2 (INT-02); do not implement in Phase 3
- **Sport enumeration validation** — whether to validate sport against a fixed Polar enum; left to Claude's discretion (pass-through to Polar for v1)

</deferred>

---

*Phase: 3-Core MCP Tools + Bundled Skill*
*Context gathered: 2026-05-11*
