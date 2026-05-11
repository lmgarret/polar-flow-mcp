---
phase: 03-core-mcp-tools-and-bundled-skill
plan: "01"
subsystem: mcp-create-training-target
tags: [mcp, polar-api, store, crypto, tdd]
dependency_graph:
  requires:
    - 02-03 (get_user_info MCP tool — established GetUserInfoHandler pattern)
    - 02-04 (gap closure — UpsertToken, GetPolarUserID, BytesKeyProvider all validated)
  provides:
    - store.GetEncryptedToken (read method for encrypted token blobs)
    - mcp.RegisterTools(s, st, cipher) (updated signature accepting *crypto.Cipher)
    - mcp.CreateTrainingTargetHandler (exported handler closure for create_training_target tool)
    - polar.Client.CreateTrainingTarget (POST method + struct types)
    - polar.SetTrainingTargetsBaseURL (test override, polartest-gated)
  affects:
    - cmd/polar-flow-mcp/main.go (call site updated)
    - internal/mcp/mcp.go (RegisterTools signature + create_training_target registration)
tech_stack:
  added: []
  patterns:
    - flat-to-tree transform (warmup/repeat/cooldown -> Polar PhaseOrRepeat nested tree)
    - JSON round-trip for typed phases parsing (map[string]any -> json.Marshal -> json.Unmarshal into typed struct)
    - buildRepeatPhase/flatToTree helpers to keep gocyclo < 15
    - resolveZone: hr_zone (escape hatch) takes priority over label
key_files:
  created:
    - internal/mcp/mcp_create_training_target.go
    - internal/mcp/mcp_create_training_target_test.go
    - internal/polar/client_training_targets_test.go
  modified:
    - internal/store/store_tokens.go (added GetEncryptedToken)
    - internal/store/store_tokens_test.go (added 3 GetEncryptedToken tests)
    - internal/mcp/mcp.go (updated RegisterTools signature + create_training_target registration)
    - internal/polar/client.go (added trainingTargetsBaseURL var + struct types + CreateTrainingTarget method)
    - internal/polar/testexports.go (added SetTrainingTargetsBaseURL)
    - cmd/polar-flow-mcp/main.go (updated RegisterTools call site)
decisions:
  - JSON round-trip (GetArguments() -> json.Marshal -> json.Unmarshal) chosen for phases parsing over BindArguments; robust regardless of how mcp-go stores Arguments internally
  - buildRepeatPhase and flatToTree extracted as separate functions to satisfy gocyclo<=15 (CreateTrainingTargetHandler was cyclomatic 29 before refactor)
  - phaseInput.HRZone stored as float64 to match JSON number representation from map[string]any
  - store tests added to existing store_tokens_test.go (package store, no build tag) matching existing internal test convention
metrics:
  duration: "6 min"
  completed: "2026-05-11T11:52:11Z"
  tasks: 3/3
  files_created: 3
  files_modified: 7
  new_tests: 20
---

# Phase 3 Plan 01: create_training_target Tool Summary

Added `create_training_target` MCP tool: flat phases array input -> Polar HR zone mapping + nested PhaseOrRepeat tree -> POST to Polar AccessLink v3 training-targets endpoint.

## What Was Built

### Task 1: Foundation (store + RegisterTools signature)
- `store.GetEncryptedToken(ctx, identity)` — reads encrypted token blob via JOIN polar_tokens/users; returns (nil, false, nil) on ErrNoRows
- `RegisterTools` signature updated to `(s, st *store.Store, cipher *crypto.Cipher)` in mcp.go
- `main.go` call site updated: `mcp.RegisterTools(mcpServer, st, cipher)`
- 3 new store tests: `TestGetEncryptedToken_{Found,NotFound,UserWithoutToken}`

### Task 2: Polar client method + struct types
- `trainingTargetsBaseURL` package-level var (overridable in tests via SetTrainingTargetsBaseURL)
- Struct types: `CreateTrainingTargetRequest`, `TrainingSessionTarget`, `DateTime`, `ExerciseTarget`, `PhaseOrRepeat`, `PhaseGoal`, `PhaseIntensity`
- `*Client.CreateTrainingTarget` method: POST with Bearer auth + JSON body; parses `{"id":...}` from response (handles string/uint64/missing); 200 and 201 both accepted
- `SetTrainingTargetsBaseURL` in testexports.go (gated behind `//go:build polartest`)
- 4 new polar tests: success, error status, 200/201 acceptance, no-id response

### Task 3: Handler + flat-to-tree transform + tool registration
- `labelToZone` map: easy=1, aerobic=2, tempo=3, threshold=4, vo2max=5
- `resolveZone`: hr_zone (1-5) takes priority over label (escape hatch per D-07)
- `flatToTree` + `buildRepeatPhase`: warmup/cooldown -> single PhaseOrRepeat; repeat -> intervals node with repeatCount + nested work/recovery children
- `parsePhasesFromRequest`: JSON round-trip extraction with 50-phase cap
- `CreateTrainingTargetHandler` exported closure: identity from ctx, GetPolarUserID, GetEncryptedToken, cipher.Decrypt, parse args, flatToTree, POST to Polar
- `create_training_target` registered in RegisterTools with full JSON schema (phases array with type/reps/goal/intensity/recovery)
- `_ = cipher` stub removed from Task 1
- 10 new mcp tests including: no-identity, no-account, no-token, 5x1km success (body assertions), hr_zone escape hatch, multi-repeat, Polar error, invalid date, empty phases, unknown phase type

## Public API Surface

```go
// store package
func (s *Store) GetEncryptedToken(ctx context.Context, identity string) ([]byte, bool, error)

// mcp package
func RegisterTools(s *server.MCPServer, st *store.Store, cipher *crypto.Cipher)
func CreateTrainingTargetHandler(st *store.Store, cipher *crypto.Cipher) func(ctx, CallToolRequest) (*CallToolResult, error)

// polar package
func (c *Client) CreateTrainingTarget(ctx context.Context, polarUserID string, body CreateTrainingTargetRequest) (string, error)
type CreateTrainingTargetRequest struct { Session TrainingSessionTarget; Exercise []ExerciseTarget }
type TrainingSessionTarget struct { Name string; StartTime DateTime }
type DateTime struct { Year, Month, Day, Hour, Min, Sec int }
type ExerciseTarget struct { Idx int64; Type string; PhaseOrRepeat []PhaseOrRepeat }
type PhaseOrRepeat struct { Name, ChangeType string; Goal PhaseGoal; Intensity *PhaseIntensity; RepeatCount *int; PhaseOrRepeat []PhaseOrRepeat }
type PhaseGoal struct { Type string; Duration *int64; Distance *float64 }
type PhaseIntensity struct { Type string; LowerZone, UpperZone *int }

// polar testexports.go (polartest build tag only)
func SetTrainingTargetsBaseURL(s string) func()
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Refactor] Extracted flatToTree helpers to satisfy gocyclo**
- **Found during:** Task 3 lint
- **Issue:** `CreateTrainingTargetHandler` had cyclomatic complexity 29 (limit 15). The inline switch+nested logic for all phase types exceeded the threshold.
- **Fix:** Extracted `buildRepeatPhase(i, p)` and `flatToTree(phases)` as separate functions. Each is well within the gocyclo limit.
- **Files modified:** `internal/mcp/mcp_create_training_target.go`
- **Commit:** 730f315

**2. [Plan deviation] store_tokens_test.go uses package `store` (internal), not `store_test` with build tag**
- **Found during:** Task 1 test implementation
- **Reason:** The existing `store_tokens_test.go` is `package store` (internal test) without a build tag — matching all other store test files. Adding `//go:build polartest` to an existing no-tag file would change the semantics of all existing tests in that file. The build command `CGO_ENABLED=0 go test -tags=polartest -count=1 ./...` always compiles both tagged and untagged files, so the new tests run regardless. Followed codebase convention.

**3. [Plan deviation] mcp_create_training_target.go omits `strconv` import and `var _ = strconv.Atoi`**
- **Reason:** The plan included a stub `var _ = strconv.Atoi` to keep strconv referenced. After implementing the handler, strconv is not needed (all numeric parsing is via JSON unmarshal into typed struct or float64->int cast). The import was omitted per Go convention (no unused imports).

## Known Stubs

None — all data paths are wired. The Polar API endpoint shape is ASSUMED from v4 swagger (not confirmed against live account). Errors surface verbatim to the user via ToolResultError, enabling fast iteration without re-deploy when the live API returns unexpected shapes.

## Threat Flags

No new threat surface beyond what is documented in the plan's `<threat_model>`. All STRIDE mitigations from T-03-01-01 through T-03-01-08 are implemented as specified.

## Self-Check: PASSED

Files exist:
- internal/store/store_tokens.go — FOUND
- internal/mcp/mcp_create_training_target.go — FOUND
- internal/polar/client.go (with CreateTrainingTarget) — FOUND
- internal/polar/testexports.go (with SetTrainingTargetsBaseURL) — FOUND

Commits exist:
- a45f418 (Task 1) — FOUND
- 0e3f7cb (Task 2) — FOUND
- 730f315 (Task 3) — FOUND
