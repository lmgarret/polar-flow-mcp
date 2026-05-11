---
phase: 03-core-mcp-tools-and-bundled-skill
reviewed: 2026-05-11T00:00:00Z
depth: standard
files_reviewed: 14
files_reviewed_list:
  - cmd/polar-flow-mcp/main.go
  - internal/mcp/mcp_create_training_target.go
  - internal/mcp/mcp_create_training_target_test.go
  - internal/mcp/mcp_delete_training_target.go
  - internal/mcp/mcp_delete_training_target_test.go
  - internal/mcp/mcp.go
  - internal/mcp/mcp_list_training_targets.go
  - internal/mcp/mcp_list_training_targets_test.go
  - internal/polar/client.go
  - internal/polar/client_training_targets_test.go
  - internal/polar/testexports.go
  - internal/store/store_tokens.go
  - internal/store/store_tokens_test.go
  - skill/polar-coach/SKILL.md
findings:
  critical: 2
  warning: 4
  info: 3
  total: 9
status: issues_found
---

# Phase 03: Code Review Report

**Reviewed:** 2026-05-11T00:00:00Z
**Depth:** standard
**Files Reviewed:** 14
**Status:** issues_found

## Summary

Phase 3 delivers three MCP tool handlers (create/list/delete training targets), the Polar client
methods backing them, a `GetEncryptedToken` store method, and the polar-coach skill document.
The overall structure is sound: handlers are orthogonally testable, the cipher-per-call pattern is
correct, and the 404-as-informational design is well-executed.

Two blockers are present. First, `polarUserID` is interpolated directly into URL paths in
`CreateTrainingTarget` and `ListTrainingTargets` without `url.PathEscape`, creating a path-injection
vector (the same user-supplied value is correctly escaped in `DeleteTrainingTarget`, making the
inconsistency clear). Second, warmup and cooldown phases accept `duration_s: 0` silently, producing
`"duration": 0` in the Polar payload — a semantically broken workout that the server is expected to
reject at runtime.

Four warnings cover: missing `name` length enforcement (the tool description promises max 45 chars
but the handler does not enforce it), missing validation that `from_date <= to_date` in the list
handler, a dead custom `containsSubstring` helper in tests where `strings.Contains` is already
imported, and the concurrent test in `mcp_list_training_targets_test.go` that discards
`result.IsError` so handler-level errors go unreported.

---

## Critical Issues

### CR-01: `polarUserID` not path-escaped in `CreateTrainingTarget` and `ListTrainingTargets`

**File:** `internal/polar/client.go:216` and `internal/polar/client.go:254`

**Issue:** `DeleteTrainingTarget` correctly calls `url.PathEscape(targetID)` when building its URL
(line 313), but the two sibling methods build their URLs by raw string concatenation of
`polarUserID` without escaping:

```go
// CreateTrainingTarget — line 216
reqURL := trainingTargetsBaseURL + "/" + polarUserID + "/training-targets"

// ListTrainingTargets — line 254
u, err := url.Parse(trainingTargetsBaseURL + "/" + polarUserID + "/training-targets")
```

`polarUserID` originates from the Polar token exchange (`x_user_id` integer), is stored in SQLite
as a string, and is returned via `GetPolarUserID`. In practice the values are numeric and safe, but
the type is `string` with no invariant enforcing that. If the stored value ever contains a slash or
other reserved character (e.g., due to data migration or a future code path), the request will hit
a different endpoint than intended. The inconsistency with `DeleteTrainingTarget` is itself a
maintenance hazard: a reviewer reading `DeleteTrainingTarget` would correctly conclude escaping is
necessary, then find the other two methods skip it.

**Fix:**

```go
// CreateTrainingTarget
import "net/url"
reqURL := trainingTargetsBaseURL + "/" + url.PathEscape(polarUserID) + "/training-targets"

// ListTrainingTargets
u, err := url.Parse(trainingTargetsBaseURL + "/" + url.PathEscape(polarUserID) + "/training-targets")
```

---

### CR-02: Warmup and cooldown phases accept `duration_s: 0`, producing `"duration": 0` in Polar payload

**File:** `internal/mcp/mcp_create_training_target.go:121-134`

**Issue:** `flatToTree` converts `p.DurationS * 1000` for warmup and cooldown phases with no
guard against zero:

```go
case "warmup":
    ms := p.DurationS * 1000   // ms = 0 when duration_s omitted or 0
    children = append(children, polar.PhaseOrRepeat{
        ...
        Goal: polar.PhaseGoal{Type: "DURATION", Duration: &ms},
        ...
    })
```

A caller that passes `{"type": "warmup"}` (omitting `duration_s`) or `{"type": "warmup",
"duration_s": 0}` will produce a `PhaseGoal` with `Type: "DURATION"` and `Duration: ptr(0)`,
serialised as `"duration":0`. This is a semantically nonsensical workout phase — a zero-duration
warmup — that the Polar API is likely to reject with a 400, but the rejection happens at runtime
against the live API, not at validation time. The handler returns the Polar error to the user with
no contextual hint about which phase caused the problem.

In contrast, the `repeat` goal path does validate (`p.Goal.DurationS > 0` and
`p.Goal.DistanceM > 0`), so the gap is a latent inconsistency.

**Fix:** Add an explicit check in `flatToTree` before building warmup/cooldown phases:

```go
case "warmup":
    if p.DurationS <= 0 {
        return nil, fmt.Errorf("phase %d: warmup requires duration_s > 0", i)
    }
    ms := p.DurationS * 1000
    ...
case "cooldown":
    if p.DurationS <= 0 {
        return nil, fmt.Errorf("phase %d: cooldown requires duration_s > 0", i)
    }
    ms := p.DurationS * 1000
    ...
```

---

## Warnings

### WR-01: `name` field length not enforced despite tool schema advertising max 45 chars

**File:** `internal/mcp/mcp.go:33` and `internal/mcp/mcp_create_training_target.go:208-211`

**Issue:** The tool schema description says `"max 45 chars"`:

```go
mcpgo.WithString("name", mcpgo.Required(),
    mcpgo.Description("Session name, max 45 chars (e.g. \"5x1km Threshold\")")),
```

The handler checks only that `name` is non-empty:

```go
name := req.GetString("name", "")
if name == "" {
    return mcpgo.NewToolResultError("name is required"), nil
}
```

A name of 200 characters will be sent to the Polar API, which may return a 400 at runtime. The
advertised constraint should be enforced at the handler boundary so the error message is
actionable.

**Fix:**

```go
const maxNameLen = 45
name := req.GetString("name", "")
if name == "" {
    return mcpgo.NewToolResultError("name is required"), nil
}
if len(name) > maxNameLen {
    return mcpgo.NewToolResultError(
        fmt.Sprintf("name exceeds %d characters (%d)", maxNameLen, len(name)),
    ), nil
}
```

---

### WR-02: No validation that `from_date <= to_date` in `ListTrainingTargetsHandler`

**File:** `internal/mcp/mcp_list_training_targets.go:62-67`

**Issue:** The handler validates that each date string is parseable but does not verify that
`from_date` is not after `to_date`. A caller passing `from_date: "2026-06-15", to_date:
"2026-05-01"` will forward the inverted range to the Polar API. The API may return an empty list,
a 400, or an unspecified result — none of which produce a useful error message for the user.

**Fix:**

```go
parsedFrom, parseErr := time.Parse("2006-01-02", fromDate)
if parseErr != nil {
    return mcpgo.NewToolResultError("from_date must be ISO 8601 YYYY-MM-DD: " + parseErr.Error()), nil
}
parsedTo, parseErr := time.Parse("2006-01-02", toDate)
if parseErr != nil {
    return mcpgo.NewToolResultError("to_date must be ISO 8601 YYYY-MM-DD: " + parseErr.Error()), nil
}
if parsedTo.Before(parsedFrom) {
    return mcpgo.NewToolResultError(
        fmt.Sprintf("to_date (%s) must not be before from_date (%s)", toDate, fromDate),
    ), nil
}
```

---

### WR-03: `TestConcurrentListAndDeleteHandlers` discards `result.IsError`, silencing handler-level failures

**File:** `internal/mcp/mcp_list_training_targets_test.go:359-372`

**Issue:** The goroutines launched by this test send `nil` to the `errs` channel regardless of
whether the handler returned an error result:

```go
go func() {
    result, err := deleteHandler(ctxBob, makeRequest(...))
    if err != nil {
        errs <- err
        return
    }
    if result == nil {
        errs <- nil
        return
    }
    errs <- nil   // result.IsError is never checked
}()
```

If `DeleteTrainingTargetHandler` returns a `ToolResultError` (e.g., because the mock server
unexpectedly returned a non-2xx status), the test reports success. The same pattern exists in the
`listHandler` goroutine. The test is checking race-detector cleanliness, but its assertions are too
weak to detect handler-level regressions under concurrency.

**Fix:** Send `result.IsError` as a signal and check it:

```go
go func() {
    result, err := deleteHandler(ctxBob, makeRequest(...))
    if err != nil {
        errs <- err
        return
    }
    if result == nil || result.IsError {
        errs <- fmt.Errorf("deleteHandler returned error result: %s", resultText(result))
        return
    }
    errs <- nil
}()
```

---

### WR-04: `containsSubstring` reimplements `strings.Contains` despite `"strings"` being imported

**File:** `internal/polar/client_training_targets_test.go:452-462`

**Issue:** The function comment states "strings.Contains is not available without import in test
helper scope," but `"strings"` is imported at line 11 of the same file. The implementation is a
manual O(n·m) substring search that is equivalent to `strings.Contains` but less readable and not
standard. This introduces dead code that will confuse maintainers.

```go
// containsSubstring is a local helper (strings.Contains is not available without import in test helper scope).
func containsSubstring(s, substr string) bool { ... }
```

**Fix:** Delete `containsSubstring` and replace every call site with `strings.Contains`.

---

## Info

### IN-01: SKILL.md describes `get_user_info` as tool #1 with "call this FIRST" guidance, but does not clarify that it is optional for create/list/delete

**File:** `skill/polar-coach/SKILL.md:27-29`

**Issue:** The skill says to call `get_user_info` "FIRST if you're unsure whether the user has
linked their account." This could lead a model to always prepend a `get_user_info` call before
every create/list/delete invocation, adding unnecessary latency. The three Phase 3 tools already
return a `No Polar account linked` message with an `/oauth/login` hint when the account is not
linked. The guidance should clarify when the preflight check is genuinely needed versus when it is
redundant.

**Fix:** Add a sentence such as: "You do not need to call `get_user_info` before every tool
invocation — `create_training_target`, `list_training_targets`, and `delete_training_target` each
return their own link hint if the account is not connected."

---

### IN-02: `ExerciseTarget.Idx` is always hard-coded to `0` with no comment explaining multi-exercise workouts

**File:** `internal/mcp/mcp_create_training_target.go:252`

**Issue:** The Polar API type includes `Idx int64` for ordering exercises within a session. The
handler always sends `Idx: 0` with a single-element `Exercise` slice:

```go
Exercise: []polar.ExerciseTarget{{
    Idx:  0,
    Type: "PHASED",
    ...
}},
```

This is intentional for v1 (single-exercise sessions only). A comment noting that multi-exercise
sessions are out of scope would prevent a future contributor from trying to add a second exercise
block without understanding the schema implications.

**Fix:** Add a comment above the `Exercise` slice literal:

```go
// Phase 3 creates single-exercise sessions only; Idx 0 is the sole exercise.
// Multi-exercise sessions are out of scope for v1.
Exercise: []polar.ExerciseTarget{{
```

---

### IN-03: `TestCreateTrainingTarget_NoLinkedAccount` passes an empty `phases` array but expects a `no Polar account` response, not an `at least one phase` error — order dependency is invisible

**File:** `internal/mcp/mcp_create_training_target_test.go:65-88`

**Issue:** The test passes `"phases": []any{}` along with no linked account. It relies on the
handler checking for a linked Polar account before it validates the phases array. If the handler
validation order ever changes (e.g., phases validated eagerly before the DB lookup), the test would
still compile and run but would assert the wrong branch, silently masking the real missing-account
path.

This same pattern appears in `TestCreateTrainingTarget_NoToken` (line 101). The implicit
ordering dependency is invisible without reading the handler source.

**Fix:** Either omit the `phases` key entirely from these tests (since the branch under test
returns before reaching phases validation), or add a comment explaining the dependency:

```go
// phases is intentionally omitted/invalid here — the handler returns
// before reaching phases validation when no account is linked.
```

---

_Reviewed: 2026-05-11T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
