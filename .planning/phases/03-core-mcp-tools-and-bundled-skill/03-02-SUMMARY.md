---
phase: 03-core-mcp-tools-and-bundled-skill
plan: "02"
subsystem: mcp-list-delete-training-targets
tags: [mcp, polar-api, tdd, sentinel-error]
dependency_graph:
  requires:
    - 03-01 (store.GetEncryptedToken, RegisterTools(s, st, cipher) signature, polar.Client.CreateTrainingTarget precedent)
  provides:
    - polar.Client.ListTrainingTargets (GET method + dual-shape JSON parser)
    - polar.Client.DeleteTrainingTarget (DELETE method + ErrTargetNotFound sentinel)
    - polar.ErrTargetNotFound (exported sentinel for 404 discrimination)
    - polar.TrainingTargetSummary (minimal projection struct)
    - mcp.ListTrainingTargetsHandler (closure for list_training_targets tool)
    - mcp.DeleteTrainingTargetHandler (closure for delete_training_target tool)
  affects:
    - internal/mcp/mcp.go (list_training_targets + delete_training_target registrations)
tech_stack:
  added: []
  patterns:
    - dual-shape JSON parser (try array unmarshal first, fall back to wrapped-object shape)
    - exported sentinel error (ErrTargetNotFound) for informational 404 discrimination
    - url.PathEscape for target_id to prevent path injection (T-03-02-07)
    - same identity/decrypt/unlinked-error closure pattern as CreateTrainingTargetHandler
key_files:
  created:
    - internal/mcp/mcp_list_training_targets.go
    - internal/mcp/mcp_list_training_targets_test.go
    - internal/mcp/mcp_delete_training_target.go
    - internal/mcp/mcp_delete_training_target_test.go
  modified:
    - internal/polar/client.go (ErrTargetNotFound, TrainingTargetSummary, ListTrainingTargets, DeleteTrainingTarget)
    - internal/polar/client_training_targets_test.go (10 new tests for list + delete)
    - internal/mcp/mcp.go (list_training_targets + delete_training_target registrations)
decisions:
  - Variable renamed from err to deleteErr in DeleteTrainingTargetHandler to avoid shadowing outer err; behavior is identical to plan spec
  - Unused mcpgo import removed from mcp_list_training_targets_test.go (makeRequest helper is already in mcp_create_training_target_test.go)
metrics:
  duration: "5 min"
  completed: "2026-05-11T12:00:35Z"
  tasks: 2/2
  files_created: 4
  files_modified: 3
  new_tests: 25
---

# Phase 3 Plan 02: list_training_targets + delete_training_target Summary

Added `list_training_targets` and `delete_training_target` MCP tools: date-defaulting GET with dual-shape JSON parser, and sentinel-error-driven DELETE with informational 404 path.

## What Was Built

### Task 1: Polar client methods + struct types + tests

- `polar.ErrTargetNotFound` — exported sentinel; handlers use `errors.Is` to distinguish 404 from genuine failures
- `polar.TrainingTargetSummary{ID, Name, Date, Time}` — minimal projection for human-readable listing
- `*Client.ListTrainingTargets(ctx, polarUserID, fromDate, toDate)` — GET with optional from_date/to_date query params; tolerates bare-array and wrapped-object (`{"training-targets":[...]}`) response shapes via two-pass unmarshal; body capped at 1 MiB via `io.LimitReader`
- `*Client.DeleteTrainingTarget(ctx, polarUserID, targetID)` — DELETE with `url.PathEscape(targetID)`; nil on 200/204, `ErrTargetNotFound` on 404, wrapped error otherwise
- 10 new polar tests: array shape, wrapped shape, empty list, error status, no-date-params, 200/204 success, 404 sentinel, 500 error, URL path escaping

### Task 2: MCP handlers + tool registration + handler tests

- `ListTrainingTargetsHandler` — defaults `from_date` to today UTC, `to_date` to today+30 days; validates both dates as ISO 8601; formats results as `- {id}: {name} ({date} {time})` per line; empty list returns "No training targets between …"
- `DeleteTrainingTargetHandler` — resolves identity/polarUserID/token via same pattern as CreateTrainingTargetHandler; `ErrTargetNotFound` → ToolResultText (informational, not error); other Polar errors → ToolResultError
- Both handlers return `/oauth/login` hint as ToolResultText when account unlinked or token missing (MCP-06)
- Both use `auth.UserIDFromContext(ctx)` exclusively — no new context key types (MCP-07)
- `list_training_targets` and `delete_training_target` registered in `RegisterTools` via `s.AddTool`
- 7 list handler tests + 8 delete handler tests + 1 concurrent race test (`TestConcurrentListAndDeleteHandlers`)
- Full suite (all packages) green under `-race`

## Public API Surface

```go
// polar package
var ErrTargetNotFound = errors.New("polar: training target not found")
type TrainingTargetSummary struct { ID, Name, Date, Time string }
func (c *Client) ListTrainingTargets(ctx context.Context, polarUserID, fromDate, toDate string) ([]TrainingTargetSummary, error)
func (c *Client) DeleteTrainingTarget(ctx context.Context, polarUserID, targetID string) error

// mcp package
func ListTrainingTargetsHandler(st *store.Store, cipher *crypto.Cipher) func(ctx, CallToolRequest) (*CallToolResult, error)
func DeleteTrainingTargetHandler(st *store.Store, cipher *crypto.Cipher) func(ctx, CallToolRequest) (*CallToolResult, error)
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Renamed err to deleteErr in DeleteTrainingTargetHandler**
- **Found during:** Task 2 implementation
- **Issue:** Using `err` as the variable name for `client.DeleteTrainingTarget(...)` would shadow the outer `err` from `cipher.Decrypt`, triggering a potential confusion and potential lint warning.
- **Fix:** Used `deleteErr` for the delete call result; behavior is identical to the plan specification.
- **Files modified:** `internal/mcp/mcp_delete_training_target.go`
- **Commit:** 26e5796

**2. [Plan deviation] Removed unused mcpgo import from mcp_list_training_targets_test.go**
- **Reason:** The `makeRequest` helper and `mcpgo.CallToolRequest` type are already available via `mcp_create_training_target_test.go` in the same `mcp_test` package. Go forbids unused imports.

## Known Stubs

None — all data paths are wired. The Polar API list-response shape is ASSUMED (dual-shape parser handles both bare-array and `{"training-targets":[...]}` forms). If a third shape is encountered in live testing, a parser extension is needed (noted as residual risk in the plan's threat model).

## Threat Flags

No new threat surface beyond what is documented in the plan's `<threat_model>`. All STRIDE mitigations T-03-02-01 through T-03-02-07 are implemented as specified.

## Self-Check: PASSED

Files exist:
- internal/polar/client.go (with ListTrainingTargets, DeleteTrainingTarget, ErrTargetNotFound) — FOUND
- internal/mcp/mcp_list_training_targets.go — FOUND
- internal/mcp/mcp_delete_training_target.go — FOUND
- internal/mcp/mcp_list_training_targets_test.go — FOUND
- internal/mcp/mcp_delete_training_target_test.go — FOUND

Commits exist:
- dada216 (Task 1) — FOUND
- 26e5796 (Task 2) — FOUND
