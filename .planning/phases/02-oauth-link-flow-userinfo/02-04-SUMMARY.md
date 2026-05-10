---
phase: "02-oauth-link-flow-userinfo"
plan: "02-04"
subsystem: "store/oauth + polar/client + oauth/handlers"
tags: [bug-fix, security, cr-01, cr-02, cr-03, gap-closure]
dependency_graph:
  requires: ["02-01", "02-02", "02-03"]
  provides: ["OAUTH-02", "OAUTH-04", "OAUTH-05"]
  affects: ["internal/store/store_oauth.go", "internal/polar/client.go", "internal/oauth/oauth.go"]
tech_stack:
  added: []
  patterns:
    - "Conditional SQLite DELETE (WHERE expires_at >= datetime('now')) for expiry-gated atomic consume"
    - "Build-tag gating (//go:build polartest) for test-only package-level var mutators"
    - "golangci-lint run.build-tags for lint consistency with test build tags"
key_files:
  created: []
  modified:
    - internal/store/store_oauth.go
    - internal/store/store_oauth_test.go
    - internal/polar/client.go
    - internal/polar/client_test.go
    - internal/oauth/oauth.go
    - internal/oauth/oauth_test.go
    - internal/polar/testexports.go
    - Makefile
    - .github/workflows/ci.yml
    - .golangci.yml
    - CLAUDE.md
  deleted:
    - internal/polar/export_test.go
decisions:
  - "CR-01: Expired OAuth state rows are NOT deleted on ConsumeOAuthState — retained for diagnostics; conditional DELETE (WHERE expires_at >= datetime('now')) + diagnostic SELECT distinguishes ErrNotFound vs ErrExpired"
  - "CR-02: RegisterUser takes explicit memberID string; member-id JSON field receives the Polar x_user_id (formatted string), not the OAuth2 access token; marshal errors are surfaced"
  - "CR-03: testexports.go gated behind //go:build polartest; golangci-lint run.build-tags set to polartest; all go test invocations pass -tags=polartest"
metrics:
  duration: "~20 min"
  completed: "2026-05-10"
  tasks_completed: 3
  files_modified: 11
---

# Phase 2 Plan 04: Gap Closure (CR-01 / CR-02 / CR-03) Summary

**One-liner:** Fixed three BLOCKER security/correctness gaps: expired-state-delete race (CR-01), credential exposure via member-id field (CR-02), and test-only symbols leaking into production binaries (CR-03).

## What Was Built

### CR-01: ConsumeOAuthState — check expiry BEFORE delete

**Problem:** The original implementation issued `DELETE FROM pending_auth WHERE state = ? RETURNING identity, expires_at`, then checked `time.Now().UTC().After(expiresAt)`. This deleted the row unconditionally and only then checked expiry. An expired row was silently consumed (deleted), preventing operators from seeing it and creating a narrow state-hijacking window.

**Fix:** Replaced the unconditional DELETE with a conditional DELETE: `DELETE FROM pending_auth WHERE state = ? AND expires_at >= datetime('now') RETURNING identity`. SQLite evaluates the WHERE inside the DELETE atomically, so non-expired rows are consumed in one statement with no SELECT-then-DELETE race. When the DELETE returns no rows (sql.ErrNoRows), a diagnostic SELECT on readDB determines whether the state is absent (ErrNotFound) or expired (ErrExpired). Expired rows are left in place for operators and future TTL sweeps.

**UX:** The ErrExpired branch message was updated from "please try again" to "please restart from /oauth/login" so users understand they must initiate a new flow.

**Tests:** Updated `TestConsumeOAuthState_Expired` to assert that the expired row SURVIVES (second call returns ErrExpired, not ErrNotFound). Added `TestConsumeOAuthState_Expired_DoesNotDelete` as an explicit CR-01 regression guard that also checks `COUNT(*) == 1` after the call.

### CR-02: RegisterUser — accept explicit memberID, not access token

**Problem:** The original `RegisterUser(ctx, accessToken string)` sent `{"member-id": accessToken}` — the OAuth2 bearer token — in the JSON body. Polar's API expected the partner user identifier (the numeric Polar user ID). This was semantically wrong and constituted a credential leak into Polar's request logs. Additionally, the `json.Marshal` error was silently discarded with `body, _ := json.Marshal(...)`.

**Fix:** Changed signature to `RegisterUser(ctx context.Context, accessToken, memberID string) (int64, error)`. The body now marshals `{"member-id": memberID}`. The marshal error is captured and returned: `body, err := json.Marshal(...); if err != nil { return 0, fmt.Errorf("polar: marshal register body: %w", err) }`. The `accessToken` is used only in the `Authorization: Bearer` header.

**Caller (oauth.go):** Moved `polarUserID := strconv.FormatInt(tr.XUserID, 10)` before the `RegisterUser` call and passed it as the `memberID` argument. The single variable is reused for both registration and `UpsertUser`.

**Tests:** Updated all three `TestRegisterUser_*` tests to use the new 3-argument call `("tok", "12345")`. `TestRegisterUser_Success` explicitly asserts `payload["member-id"] != "tok" && payload["member-id"] == "12345"` as the CR-02 regression guard. `TestCallbackHandler_Success` in oauth_test.go now reads the registration request body and asserts `member-id == "777"` (matching the `x_user_id: 777` returned by the token mock).

### CR-03: Test-only endpoint setters behind polartest build tag

**Problem:** `internal/polar/testexports.go` was a plain Go source file (no build constraints). `SetTokenEndpoint` and `SetRegisterEndpoint` were compiled into production binaries, expanding the attack surface unnecessarily.

**Earlier failed attempt context:** A previous attempt put these functions in a `_test.go` file inside the `polar` package. That would have worked for polar's own tests but made the functions invisible to external test packages like `oauth_test`. `_test.go` files in package P are NOT included when other packages import P in their test binaries.

**Chosen fix:** Added `//go:build polartest` as the first line of `internal/polar/testexports.go`. Production builds (`go build`) do not set this tag, so the file is excluded. All `go test` invocations now pass `-tags=polartest` so the file is included when testing.

**Build system updates:**
- `.golangci.yml`: Added `run.build-tags: [polartest]` so golangci-lint includes the file during analysis (without this, lint reported `undefined: polar.SetTokenEndpoint` in test files).
- `Makefile`: `test` target updated to `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...`.
- `.github/workflows/ci.yml`: Test step updated to `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...`.
- `CLAUDE.md`: Pre-Commit Checklist item 2 updated to use `-tags=polartest`.
- Deleted `internal/polar/export_test.go` — this was a doc-only stub that was now misleading.

**Verification:** `CGO_ENABLED=0 go build ./cmd/polar-flow-mcp && go tool nm ./polar-flow-mcp | grep -E "polar\.(SetTokenEndpoint|SetRegisterEndpoint)"` returned no matches — the symbols are absent from the production binary.

## Verification Transcript

```
$ ~/go/bin/golangci-lint run ./...
0 issues.

$ CGO_ENABLED=0 go test -tags=polartest -count=1 ./...
?   	github.com/lm/polar-flow-mcp/cmd/polar-flow-mcp	[no test files]
ok  	github.com/lm/polar-flow-mcp/internal/auth
ok  	github.com/lm/polar-flow-mcp/internal/config
ok  	github.com/lm/polar-flow-mcp/internal/crypto
ok  	github.com/lm/polar-flow-mcp/internal/mcp
ok  	github.com/lm/polar-flow-mcp/internal/oauth
ok  	github.com/lm/polar-flow-mcp/internal/polar
ok  	github.com/lm/polar-flow-mcp/internal/store

$ CGO_ENABLED=0 go build ./cmd/polar-flow-mcp
(success)

$ go tool nm ./polar-flow-mcp | grep -E "polar\.(SetTokenEndpoint|SetRegisterEndpoint)"
(no output — symbols absent)
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated TestConsumeOAuthState_Expired to match new CR-01 behavior**
- **Found during:** Task 1
- **Issue:** The existing `TestConsumeOAuthState_Expired` test asserted that a second call after expiry returns `ErrNotFound` (old "delete on expiry" behavior). This conflicts with the new CR-01 contract where expired rows are NOT deleted — second call must still return `ErrExpired`.
- **Fix:** Updated the test's second-call assertion from `ErrNotFound` to `ErrExpired` to reflect the correct new invariant.
- **Files modified:** `internal/store/store_oauth_test.go`
- **Commit:** d6ad6e3

**2. [Rule 3 - Blocking] Added run.build-tags to .golangci.yml**
- **Found during:** Task 3 verification
- **Issue:** After adding `//go:build polartest` to `testexports.go`, `golangci-lint run ./...` failed with `undefined: polar.SetTokenEndpoint` because the linter was not passing the tag to the Go type checker.
- **Fix:** Added `run: build-tags: [polartest]` to `.golangci.yml`.
- **Files modified:** `.golangci.yml`
- **Commit:** d23eb67

**3. [Rule 2 - Missing] Added io import to oauth_test.go**
- **Found during:** Task 2 (adding registration mock body assertion)
- **Issue:** The updated `TestCallbackHandler_Success` mock needed `io.ReadAll` to read the request body, but `io` was not imported.
- **Fix:** Added `"io"` to the import block.
- **Files modified:** `internal/oauth/oauth_test.go`
- **Commit:** d81923b

## Known Stubs

None.

## Threat Flags

None — all changes address existing threats already catalogued in the plan's threat model (T-02-04-01 through T-02-04-03).

## Self-Check: PASSED

- `internal/store/store_oauth.go` — exists, contains `expires_at >= datetime('now')`
- `internal/polar/client.go` — exists, contains `memberID string` in RegisterUser signature
- `internal/polar/testexports.go` — exists, first line is `//go:build polartest`
- `internal/polar/export_test.go` — deleted (confirmed absent)
- `internal/store/store_oauth_test.go` — contains `TestConsumeOAuthState_Expired_DoesNotDelete`
- Commits: d6ad6e3 (CR-01), d81923b (CR-02), d23eb67 (CR-03) — all verified in git log
