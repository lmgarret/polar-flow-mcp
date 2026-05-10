---
phase: 02-oauth-link-flow-userinfo
reviewed: 2026-05-10T00:00:00Z
depth: standard
files_reviewed: 20
files_reviewed_list:
  - cmd/polar-flow-mcp/main.go
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/crypto/bytes_provider.go
  - internal/crypto/bytes_provider_test.go
  - internal/mcp/mcp.go
  - internal/mcp/mcp_test.go
  - internal/oauth/export_test.go
  - internal/oauth/oauth.go
  - internal/oauth/oauth_test.go
  - internal/polar/client.go
  - internal/polar/client_test.go
  - internal/polar/export_test.go
  - internal/polar/testexports.go
  - internal/store/store_oauth.go
  - internal/store/store_oauth_test.go
  - internal/store/store_tokens.go
  - internal/store/store_tokens_test.go
  - internal/store/store_users.go
  - internal/store/store_users_test.go
findings:
  critical: 4
  warning: 5
  info: 3
  total: 12
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-05-10T00:00:00Z
**Depth:** standard
**Files Reviewed:** 20
**Status:** issues_found

## Summary

Phase 2 implements the Polar OAuth2 link flow (Login + Callback handlers), encrypted token storage, and the `get_user_info` MCP tool. The overall architecture is sound: constant-time secret comparison runs before identity header extraction, CSRF state is consumed atomically via `DELETE … RETURNING`, tokens are stored as AES-256-GCM encrypted BLOBs, and the dual write/read pool setup correctly prevents SQLITE_BUSY. Test coverage is broad.

Four blockers were found:

1. **`polar.RegisterUser` is used incorrectly in the callback** — the function's own documented contract says callers MUST use `x_user_id` from `TokenResponse` for both the 200 and 409 paths, yet the function signature returns an `int64` that is silently discarded at every call site and the body always uses `tr.XUserID`. The discarded return value on a 200 response means the `polar-user-id` from the register response is never validated against `x_user_id`. More critically, the `Callback` handler ignores `RegisterUser`'s return value entirely (stores `(_, err)`) so if the implementation ever changes to return the body's user ID, the mismatch would silently go undetected. This is currently harmless but is a latent logic error.

2. **`testexports.go` ships in production binaries** — `SetTokenEndpoint` and `SetRegisterEndpoint` mutate package-level globals in a non-test file, so they are compiled into production builds and exported as part of the `internal/polar` API surface. If any production code (or a future CLI flag) calls them, it silently redirects all token exchanges.

3. **`ConsumeOAuthState` deletes the row before checking expiry** — the expired state row is destroyed even when returning `ErrExpired`. A caller receiving `ErrExpired` cannot retry because the state is already gone. More critically, the `Callback` handler responds to `ErrExpired` with a user-friendly "try again" message, but the state has already been irrevocably consumed; there is nothing to retry. The user must restart from `/oauth/login`. This is incorrect UX at minimum; it also means an attacker who races a stale-state submission can consume a legitimate in-flight state.

4. **`polar.RegisterUser` passes `accessToken` as `member-id`** — the Polar AccessLink API's register endpoint expects the `member-id` field to be the end-user's Polar member ID, not the bearer access token. Sending the raw access token in the JSON body is both semantically wrong and a credential exposure (the token appears in request logs on the Polar side). This warrants live-API verification against the actual Polar docs before Phase 3 depends on it.

---

## Critical Issues

### CR-01: `ConsumeOAuthState` deletes the row before validating expiry — race enables state hijacking

**File:** `internal/store/store_oauth.go:37-53`

**Issue:** `DELETE … RETURNING` atomically removes the row unconditionally. The expiry check at line 50 runs *after* the row is gone. An expired state row is consumed (deleted) even when it should be rejected. More dangerously: two concurrent requests racing the same valid state will both attempt the delete; the second request gets `ErrNotFound` as intended, but only because the first delete already won — not because of any two-phase validation. The problem is that a request with a *different, legitimate* `state` that arrives between the delete and the expiry check cannot be distinguished from the expired case. The real hazard: a user who receives `ErrExpired` is told "try again" (oauth.go:98) but the state is already consumed; their only path is to restart the full flow. If an attacker can observe the expiry window, they can submit a known-expired state to consume it and then replay the code before the legitimate user processes it (the code is not single-use on Polar's side until the token exchange succeeds).

**Fix:** Check expiry inside the transaction, or use a two-step approach: SELECT first, validate, then DELETE. Alternatively, include the expiry predicate in the DELETE and infer "expired vs not found" from a prior read:

```sql
-- Option A: select first, then delete
SELECT identity, expires_at FROM pending_auth WHERE state = ?
-- if found and not expired:
DELETE FROM pending_auth WHERE state = ?
-- if found and expired:
DELETE FROM pending_auth WHERE state = ?
-- return ErrExpired
```

Or, keep the single-statement approach but use a conditional:

```sql
DELETE FROM pending_auth WHERE state = ? RETURNING identity, expires_at,
    CASE WHEN expires_at < datetime('now') THEN 1 ELSE 0 END AS is_expired
```

Then check `is_expired` before returning. Either way, the UX message in `oauth.go` at the `ErrExpired` branch must not tell users to "try again" — the state is gone; they must restart from `/oauth/login`.

---

### CR-02: `polar.RegisterUser` sends the access token as `member-id` (credential exposure + semantic error)

**File:** `internal/polar/client.go:89`

**Issue:** Line 89 constructs the register request body as:
```go
body, _ := json.Marshal(map[string]string{"member-id": accessToken})
```
The Polar AccessLink v3 `/v3/users` registration endpoint expects `member-id` to be the user's Polar member identifier, not the OAuth2 access token. Sending the raw bearer token in the JSON body exposes the credential in Polar's request logs and is semantically incorrect. Additionally, the `json.Marshal` error is silently discarded (`body, _ := ...`). If marshalling somehow fails, `body` is nil and the POST sends an empty body, which will produce a confusing API error.

**Fix:**

```go
// marshal error must not be silently discarded
body, err := json.Marshal(map[string]string{"member-id": accessToken})
if err != nil {
    return 0, fmt.Errorf("polar: marshal register body: %w", err)
}
req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerEndpoint, bytes.NewReader(body))
```

The `member-id` field content must be verified against live Polar API documentation before Phase 3 ships — if it is supposed to be the user's member ID (not the token), the correct value is not available at this point in the flow.

---

### CR-03: `testexports.go` is not a `_test.go` file — `SetTokenEndpoint`/`SetRegisterEndpoint` compile into production binaries

**File:** `internal/polar/testexports.go:1-17`

**Issue:** `testexports.go` is a regular Go source file (no `_test.go` suffix). It exports `SetTokenEndpoint` and `SetRegisterEndpoint`, which mutate package-level globals (`tokenEndpoint`, `registerEndpoint`). These symbols compile into every binary that imports `internal/polar`, including the production server. The `export_test.go` file at `internal/polar/export_test.go` is a `package polar` file that exists solely to document that these functions come from `testexports.go` — this is backwards from the standard Go pattern. The standard pattern is to put test-only exports in `export_test.go` (a `_test.go` file in the same package), which the compiler automatically excludes from production builds.

The stated reason (accessed from `internal/oauth` tests) is valid, but the correct solution is `internal/polar/export_test.go` with `package polar` — Go test binaries for `internal/oauth` will link the `polar` package's test exports when `polar` appears in the test binary's dependency graph. Actually for cross-package access, an alternative pattern is:

**Fix:** Rename `testexports.go` to a file with a build constraint:

```go
//go:build ignore
```

Or, place the variable mutation behind a `testing.TB` gate, or use the standard approach: if cross-package test access is truly needed, expose the overridable targets through an interface injected at construction time rather than package globals, which also eliminates the need for these setter functions entirely.

---

### CR-04: `oauth.Callback` silently discards `RegisterUser`'s return value — `polar_user_id` source inconsistency

**File:** `internal/oauth/oauth.go:131`

**Issue:** Line 131:
```go
if _, err := polar.RegisterUser(r.Context(), tr.AccessToken); err != nil {
```
The `RegisterUser` function is documented to return the `polar-user-id` from the 200 response body. The callback discards it with `_` and always uses `tr.XUserID` (line 136) regardless of what `RegisterUser` returns. On a 200 (new registration), `tr.XUserID` from the token exchange and `reg.PolarUserID` from the register response should be the same value — but there is no assertion of this. If they ever differ (Polar API inconsistency, test mock mismatch), the code silently uses the wrong one. The comment at line 130 says "409 = idempotent; use x_user_id from token exchange in all cases" which is the correct intent, but the implementation of `RegisterUser` also parses and returns the 200 body's user ID (client.go:111-117), making the return value misleadingly useful.

**Fix:** Either:

(a) Make `RegisterUser` return no user ID (it is always discarded by design), simplifying the signature to `func RegisterUser(ctx context.Context, accessToken string) error`, or

(b) Assert that the returned user ID matches `tr.XUserID` on 200 to catch API inconsistencies:

```go
regUserID, err := polar.RegisterUser(r.Context(), tr.AccessToken)
if err != nil {
    // ... error handling
}
if regUserID != 0 && regUserID != tr.XUserID {
    slog.Warn("polar user ID mismatch between token and register responses",
        "token_x_user_id", tr.XUserID, "register_user_id", regUserID)
}
polarUserID := strconv.FormatInt(tr.XUserID, 10)
```

---

## Warnings

### WR-01: `auth.UserIDFromContext` uses a freshly allocated `userIDKey{}` — differs from stored `UserIDKey`

**File:** `internal/auth/auth.go:25-28` and `main.go:76`

**Issue:** `Middleware` stores the identity using `userIDKey{}` (line 70 of auth.go) as the context key. `UserIDFromContext` also retrieves with `userIDKey{}` (line 26 of auth.go) — both are unexported struct values so they compare equal via struct equality. This is correct *within* the `auth` package. However, `main.go` line 76 stores the identity using the **exported** `auth.UserIDKey` variable:

```go
return context.WithValue(ctx, auth.UserIDKey, id)
```

`auth.UserIDKey` is defined as `var UserIDKey = userIDKey{}`. This is a `var`, not a `const`. Two `userIDKey{}` struct literals compare equal in Go (zero-size structs with no fields always compare equal), so functionally this works. But if `UserIDKey` is ever changed to a different type (e.g., `string` key for debugging), the mismatch between `WithHTTPContextFunc` using `auth.UserIDKey` and `UserIDFromContext` using the internal `userIDKey{}` literal would silently break without a compile error, since both implement `any`. The discrepancy is an unnecessary maintenance hazard.

**Fix:** Have `UserIDFromContext` use `UserIDKey` explicitly, or have `Middleware` also use `UserIDKey`, eliminating the dual-literal pattern:

```go
// In Middleware:
ctx := context.WithValue(r.Context(), UserIDKey, identity)

// In UserIDFromContext:
v, ok := ctx.Value(UserIDKey).(string)
return v, ok
```

---

### WR-02: `oauth/oauth_test.go` `contains()` reimplements `strings.Contains` incorrectly for empty substrings

**File:** `internal/oauth/oauth_test.go:344-354`

**Issue:** The hand-rolled `contains` function has a special case `len(substr) == 0` returning true, but the outer condition `len(s) >= len(substr)` gates the entire expression. For `s=""` and `substr=""`, `len(s) >= len(substr)` is `0 >= 0 = true`, then `s == substr` is `"" == ""` = true, so it returns true. For `s="abc"` and `substr=""`, the outer condition is true, then `s == substr` is false, then `len(substr) == 0` is true, so returns true. This matches `strings.Contains` semantics, so the logic is not wrong per se. The bug is that the function exists at all — it duplicates `strings.Contains` and is harder to audit. More importantly, the implementation has O(n×m) complexity via the inner loop but this is test code so performance is out of scope. The real issue is that this custom implementation increases the cognitive burden of auditing security-relevant test assertions.

**Fix:** Replace with `strings.Contains`:

```go
// Delete the custom contains() and use strings.Contains directly in assertions:
if !strings.Contains(rr.Body.String(), "invalid or expired") {
```

---

### WR-03: `polar.testexports.go` globals are not safe for concurrent test execution

**File:** `internal/polar/testexports.go:6-17`

**Issue:** `SetTokenEndpoint` and `SetRegisterEndpoint` mutate package-level `var` globals without synchronization. If tests run in parallel (`t.Parallel()`), concurrent mutation and reads of `tokenEndpoint` / `registerEndpoint` in `defaultHTTPClient.Do(req)` constitute a data race. The `-race` flag required by `go test -race` in the pre-commit checklist will flag this. Currently the tests do not call `t.Parallel()`, so this does not trigger, but it is a latent defect.

**Fix:** Use `sync/atomic` or a mutex to guard the endpoint variables, or refactor to inject the endpoint via function parameter / constructor to avoid shared mutable globals entirely.

---

### WR-04: `mcp.GetUserInfoHandler` returns database error text directly to the MCP client

**File:** `internal/mcp/mcp.go:38-39`

**Issue:**
```go
return mcpgo.NewToolResultError("database error: " + err.Error()), nil
```
`err.Error()` may contain internal details such as SQLite error codes, table names, or query fragments. These leak schema/implementation details to the MCP client (Claude), which is an authenticated user but should not receive raw database errors. This is an information disclosure issue, not an authentication bypass.

**Fix:** Log the full error server-side and return a generic message to the client:

```go
slog.Error("get_user_info: database error", "identity", identity, "error", err)
return mcpgo.NewToolResultError("internal error — please try again later"), nil
```

---

### WR-05: `main.go` panics in `WithHTTPContextFunc` if identity is missing — no graceful recovery

**File:** `cmd/polar-flow-mcp/main.go:73-75`

**Issue:**
```go
if !ok {
    panic("WithHTTPContextFunc: identity not in context; auth middleware not applied")
}
```
A panic in a goroutine serving an HTTP request will crash the entire server unless the HTTP framework has a recovery middleware. The `mcp-go` `StreamableHTTPServer` may or may not install a `recover()` middleware. If it does not, any request that reaches `WithHTTPContextFunc` without the identity (e.g., a wiring bug during a future refactor that touches route registration order) will crash the server process. The comment acknowledges this is a "wiring bug" scenario, but the response (process crash) is more severe than necessary.

**Fix:** Return a 500 response instead of panicking, or verify that `mcp-go`'s StreamableHTTPServer installs a panic-recovery middleware and document that assumption:

```go
if !ok {
    slog.Error("WithHTTPContextFunc: identity not in request context — auth middleware not applied",
        "path", r.URL.Path)
    http.Error(w, "internal server error", http.StatusInternalServerError)
    return ctx
}
```

---

## Info

### IN-01: `polar/client.go` `Client` struct and `NewClient` are defined but never used

**File:** `internal/polar/client.go:18-29`

**Issue:** `Client`, `NewClient`, and the `bearerToken` field are defined but no code in this phase calls `NewClient` or uses a `Client` instance. This is dead code. `ExchangeCode` and `RegisterUser` use the package-level `defaultHTTPClient`, not a `Client`. This will cause a lint warning under `deadcode` or `unused` linters.

**Fix:** Either remove `Client` / `NewClient` if they are intended for Phase 3 (add them then), or add a `// TODO: Phase 3 uses this` comment and suppress the lint warning. Introducing dead code in a phase that has not yet needed it increases review surface unnecessarily.

---

### IN-02: `config.go` `loadKeyFromFile` does not trim a trailing newline from the key file

**File:** `internal/config/config.go:225-246`

**Issue:** `os.ReadFile` returns the raw bytes of the key file including any trailing newline. The check `len(data) != 32` will reject a 32-byte key file that has a trailing `\n` (33 bytes), producing a confusing error. Operators using `echo` or text editors to create key files frequently add trailing newlines. The `env` path (base64) does not have this problem since base64 decoding naturally ignores padding issues and length is checked after decode.

**Fix:** Document that the key file must be exactly 32 raw bytes with no newline, and add a helpful error message:

```go
if len(data) != 32 {
    return nil, fmt.Errorf(
        "ENCRYPTION_KEY_FILE %q must contain exactly 32 bytes (got %d); "+
            "if the file has a trailing newline, remove it: truncate -s 32 <file>",
        keyFile, len(data),
    )
}
```

---

### IN-03: `store_oauth_test.go` is in `package store` (white-box) but uses `WriteDB()` accessor — pattern inconsistency

**File:** `internal/store/store_oauth_test.go:71`

**Issue:** The test file is declared `package store` (white-box), giving direct access to unexported fields. Yet it uses the exported `s.WriteDB()` accessor (line 41) and `s.ReadDB()` (line 104 of the oauth_test). This is not a bug, but mixing white-box test style (`package store`) with exported-accessor-only access (`WriteDB()`) is inconsistent and makes it unclear what the test boundary is. Tests in `package store` should either access `s.writeDB` directly or be moved to `package store_test`.

**Fix:** Pick one style: rename to `package store_test` and use only exported methods, or stay `package store` and access unexported fields directly where needed. The current mix is harmless but confusing to future contributors.

---

_Reviewed: 2026-05-10T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
