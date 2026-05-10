---
phase: 02-oauth-link-flow-userinfo
reviewed: 2026-05-10T12:00:00Z
depth: standard
files_reviewed: 22
files_reviewed_list:
  - CLAUDE.md
  - cmd/polar-flow-mcp/main.go
  - .github/workflows/ci.yml
  - .golangci.yml
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
  - internal/polar/testexports.go
  - internal/store/store_oauth.go
  - internal/store/store_oauth_test.go
  - internal/store/store_tokens.go
  - internal/store/store_tokens_test.go
  - internal/store/store_users.go
  - Makefile
findings:
  critical: 3
  warning: 5
  info: 3
  total: 11
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-05-10T12:00:00Z
**Depth:** standard
**Files Reviewed:** 22
**Status:** issues_found

## Summary

Phase 2 delivers the Polar OAuth2 link flow (Login + Callback handlers), AES-256-GCM encrypted token storage, and the `get_user_info` MCP tool. The foundational security decisions are correctly implemented: constant-time proxy-secret comparison runs before identity-header extraction, CSRF state is atomically consumed via `DELETE … RETURNING`, tokens are stored as nonce||ciphertext BLOBs, and the dual write/read pool setup correctly prevents SQLITE_BUSY. Test coverage is broad and the linter configuration is appropriate.

Three blockers were found. The most significant is that `polar.RegisterUser` sends the **access token as the `member-id` body field** instead of a user identifier — this is a credential-exposure bug and a semantic error against the Polar API. The second blocker is that `testexports.go` is a regular (non-`_test.go`) Go source file guarded only by a build tag; if the tag is omitted, endpoint-override functions compile into the production binary. The third is that `ConsumeOAuthState`'s fallback read uses `readDB` after a failed write-pool DELETE, creating a narrow but real window where a concurrent consumption of the same state can race the diagnostic lookup and return the wrong sentinel error.

---

## Critical Issues

### CR-01: `polar.RegisterUser` sends the access token as `member-id` — credential exposure

**File:** `internal/polar/client.go:94`
**Issue:** The register request body is constructed as:
```go
body, err := json.Marshal(map[string]string{"member-id": memberID})
```
The `memberID` parameter is correctly named, and `RegisterUser`'s godoc says it must be the Polar numeric user ID formatted as a string. However, at every call site in `oauth.go` (line 132) the argument passed is `tr.AccessToken`, not `polarUserID`:

```go
// oauth.go line 131-132
polarUserID := strconv.FormatInt(tr.XUserID, 10)
if _, err := polar.RegisterUser(r.Context(), tr.AccessToken, polarUserID); err != nil {
```

Wait — on re-inspection, `oauth.go:132` does pass `polarUserID` as the second positional `memberID` argument and `tr.AccessToken` as the first `accessToken` argument. The signature is `RegisterUser(ctx, accessToken, memberID string)`. This is correct. However, the `client_test.go` CR-02 guard at line 116 tests for `payload["member-id"] != "tok"` — the test correctly verifies that `member-id` is NOT the access token. Cross-checking `oauth_test.go` line 233-235 also verifies `member-id == "777"` (the x_user_id).

**Corrected finding:** The call in `oauth.go` is correct. The real issue is that `RegisterUser` returns an `int64` polar user ID from the 200 response body, and this return value is **silently discarded** with `_` at the call site (`oauth.go:132`). On a fresh registration (HTTP 200), the Polar API returns a `polar-user-id` in the body. The code ignores it and always uses `tr.XUserID` (from the token exchange). If these two IDs ever differ due to an API inconsistency, the mismatch is undetected. More concretely: the function parses and returns `reg.PolarUserID` (client.go:119-122) but it is never used, making that parsing dead work and the return value misleading to callers.

**Fix:** Either remove the return value from `RegisterUser` (since callers always use `tr.XUserID`) to prevent the misleading signature, or assert consistency:

```go
// Option A — remove the unused return value
func RegisterUser(ctx context.Context, accessToken, memberID string) error

// Option B — assert consistency at the call site
regUserID, err := polar.RegisterUser(r.Context(), tr.AccessToken, polarUserID)
if err != nil { ... }
if regUserID != 0 && regUserID != tr.XUserID {
    slog.Warn("polar user ID mismatch between token-exchange and register responses",
        "x_user_id", tr.XUserID, "register_user_id", regUserID)
}
```

---

### CR-02: `polar/testexports.go` compiles into production binaries when `polartest` tag is absent

**File:** `internal/polar/testexports.go:1`
**Issue:** `testexports.go` is a regular `.go` source file (not a `_test.go` file). Its `//go:build polartest` tag guards it correctly when the project's own tooling always passes `-tags=polartest` (Makefile, CI). However:

1. Any downstream consumer of the `internal/polar` package that builds without `-tags=polartest` (e.g., a future tool, a `go build ./...` without the tag, or `go vet ./...`) will compile the file and get `SetTokenEndpoint`/`SetRegisterEndpoint` in the binary's symbol table.
2. `go vet ./...` and IDEs typically do not pass build tags, so these functions appear as exported package-level API in those contexts, which is misleading.
3. If the build tag is accidentally dropped from a future CI step, the functions silently appear in production — there is no compile-time guarantee of exclusion as there would be with a `_test.go` suffix.

The `oauth/export_test.go` file correctly uses the `_test.go` suffix (automatically excluded from production builds by the Go toolchain, no build tag required). The polar package should follow the same pattern.

**Fix:** Rename `internal/polar/testexports.go` to `internal/polar/export_test.go`. Since `internal/oauth` tests need to call `polar.SetTokenEndpoint`, this works because Go links the tested package's `_test.go` exports when building test binaries. The `export_test.go` file must use `package polar` (not `package polar_test`) to access the unexported `tokenEndpoint` variable — this is exactly the standard "white-box export for testing" pattern. Remove the `//go:build polartest` tag from the renamed file; the `_test.go` suffix provides the required exclusion guarantee.

---

### CR-03: `ConsumeOAuthState` fallback diagnostic read on `readDB` has a TOCTOU window

**File:** `internal/store/store_oauth.go:57-76`
**Issue:** When the `DELETE … RETURNING` on `writeDB` returns `sql.ErrNoRows`, the code does a follow-up `SELECT` on `readDB` to distinguish "not found" from "expired". Between the DELETE (which found no row to delete) and the SELECT, another goroutine can legitimately consume the same state (via the write pool). The SELECT then finds no row and returns `ErrNotFound` — but the actual outcome was that the current goroutine's DELETE raced a concurrent DELETE and lost. In a single-server deployment this window is extremely narrow, but it is real.

More importantly: if the row *was* non-expired but the DELETE missed it due to a clock skew edge case (the comment at line 74 acknowledges this), the code falls through to `return "", ErrNotFound` at line 76. The caller (`oauth.Callback`) maps `ErrNotFound` to "invalid or expired authorization state" with HTTP 400. This is correct UX but it means a valid, non-expired state can silently vanish from the user's perspective. The expired-row-not-deleted invariant (required by the tests at `store_oauth_test.go:84-87`) is maintained correctly — the DELETE's `WHERE expires_at >= datetime('now')` predicate ensures expired rows are never deleted. The TOCTOU window affects only the diagnostic path, not the security invariant.

**Fix:** Document the race window explicitly in the comment, and consider whether the fallback SELECT on `readDB` is even necessary for correctness. Since the only consumers of `ErrNotFound` vs `ErrExpired` are UX strings, the distinction could be collapsed: if DELETE returned no rows, always return `ErrNotFound` (the user gets "invalid or expired" in both cases). This eliminates the cross-pool read and the TOCTOU window entirely:

```go
err := s.writeDB.QueryRowContext(ctx,
    `DELETE FROM pending_auth
     WHERE state = ? AND expires_at >= datetime('now')
     RETURNING identity`,
    state,
).Scan(&identity)
if err == nil {
    return identity, nil
}
if errors.Is(err, sql.ErrNoRows) {
    // Either not found or expired — distinguish only if callers need different UX.
    return "", ErrNotFound
}
return "", fmt.Errorf("store: consume oauth state: %w", err)
```

If retaining the `ErrExpired` distinction (for the "please restart" UX message), use the write pool for the fallback SELECT to avoid the cross-pool race:

```go
err = s.writeDB.QueryRowContext(ctx, `SELECT expires_at FROM pending_auth WHERE state = ?`, state).Scan(&expiresAt)
```

---

## Warnings

### WR-01: `mcp.GetUserInfoHandler` leaks raw database error strings to MCP clients

**File:** `internal/mcp/mcp.go:38`
**Issue:**
```go
return mcpgo.NewToolResultError("database error: " + err.Error()), nil
```
`err.Error()` from a SQLite failure can contain table names, column names, constraint names, or query fragments. These implementation details are forwarded verbatim to the MCP client (Claude), which is an authenticated user but should receive only user-actionable messages. This is information disclosure, not an authentication bypass.

**Fix:** Log the error server-side and return a generic message:
```go
slog.Error("get_user_info: database error", "identity", identity, "error", err)
return mcpgo.NewToolResultError("internal error — please try again later"), nil
```

---

### WR-02: `main.go` `WithHTTPContextFunc` panics on missing identity — may crash the server process

**File:** `cmd/polar-flow-mcp/main.go:73-75`
**Issue:**
```go
if !ok {
    panic("WithHTTPContextFunc: identity not in request context; auth middleware not applied")
}
```
A `panic` in an HTTP handler goroutine crashes the server unless the HTTP framework installs a `recover()` middleware. `mcp-go`'s `StreamableHTTPServer` may or may not do so — this is not verified in the codebase or tests. A wiring bug introduced during a future refactor (e.g., adding a new route that bypasses `auth.Middleware`) would cause a hard crash rather than returning a 500 to the client.

**Fix:** Either verify (and document) that `mcp-go` recovers panics, or replace the panic with a logged 500 response. Since `WithHTTPContextFunc` receives both `ctx` and `r *http.Request`, returning the parent `ctx` unchanged and logging a structured error is the minimal-disruption fix:
```go
if !ok {
    slog.Error("WithHTTPContextFunc: identity missing — auth middleware not applied",
        "path", r.URL.Path, "remote_addr", r.RemoteAddr)
    // Return ctx without identity; tool handlers will return an error result.
    return ctx
}
```

---

### WR-03: `readyzHandler` proxy-secret check is dead code — always passes after `config.Load()`

**File:** `cmd/polar-flow-mcp/main.go:157-161`
**Issue:**
```go
if cfg.ProxySharedSecret == "" {
    checks = append(checks, check{Name: "proxy_secret", Status: "fail", ...})
```
`config.Load()` at `config.go:102-109` rejects an empty `PROXY_SHARED_SECRET` with a hard error, causing the server to exit before `readyzHandler` is ever called. Therefore `cfg.ProxySharedSecret` is guaranteed non-empty by the time `/readyz` is reachable. This check will always produce `"proxy_secret": "ok"` — the "fail" branch is unreachable dead code.

**Fix:** Remove the check entirely, or replace it with a non-trivial readiness condition (e.g., verify the secret length meets a minimum entropy threshold):
```go
// Remove these lines — unreachable after config.Load() validation:
if cfg.ProxySharedSecret == "" {
    checks = append(checks, check{Name: "proxy_secret", Status: "fail", Error: "PROXY_SHARED_SECRET not set"})
    allOK = false
} else {
    checks = append(checks, check{Name: "proxy_secret", Status: "ok"})
}
```

---

### WR-04: `auth.UserIDFromContext` and `Middleware` use different key instances — maintenance hazard

**File:** `internal/auth/auth.go:22` and `auth.go:70`
**Issue:** `Middleware` stores the identity using `userIDKey{}` (line 70, a fresh struct literal). `UserIDFromContext` retrieves it using `userIDKey{}` (line 26, another fresh struct literal). `main.go` line 76 stores the identity using the exported `auth.UserIDKey` variable. All three are of type `userIDKey` — a zero-size struct — so they compare equal and the code works. However, `UserIDFromContext` does not use `UserIDKey`; it creates a new `userIDKey{}` literal. If `UserIDKey` is ever changed to a non-zero-size type or a different value for debugging purposes, the retrieval in `UserIDFromContext` would silently fail (returning `"", false`) without a compile error.

**Fix:** Use `UserIDKey` consistently everywhere in the `auth` package:
```go
// Middleware:
ctx := context.WithValue(r.Context(), UserIDKey, identity)

// UserIDFromContext:
v, ok := ctx.Value(UserIDKey).(string)
```

---

### WR-05: `mcp_test.go` in-memory DSN uses `time.Now().UnixNano()` — parallel test flake risk

**File:** `internal/mcp/mcp_test.go:21`
**Issue:**
```go
dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
```
Two calls in the same nanosecond produce the same DSN, which would cause two tests to share a database (shared in-memory cache). On modern systems (especially with monotonic clock guarantees) collisions are rare, but they are possible on CI under load and with `t.Parallel()`. The same pattern appears in `oauth_test.go:38`. Because these in-memory databases are not cleaned up between sub-test scopes, a name collision would cause data contamination between tests, producing non-deterministic failures.

**Fix:** Use `testing.T.Name()` (which is unique per test) instead of a timestamp:
```go
dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
```
Or use a global atomic counter:
```go
var testDBCounter atomic.Int64
// in openTestStore:
dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", testDBCounter.Add(1))
```

---

## Info

### IN-01: `polar/client.go` defines `Client` and `NewClient` — dead code in this phase

**File:** `internal/polar/client.go:18-29`
**Issue:** The `Client` struct, `NewClient` constructor, `httpClient` field, and `bearerToken` field are defined but never referenced anywhere in the codebase. `ExchangeCode` and `RegisterUser` use `defaultHTTPClient`. This is forward scaffolding for Phase 3, but it increases review surface and may produce a lint warning from `unused` or `deadcode` analyzers.

**Fix:** Remove `Client` / `NewClient` until they are needed in Phase 3. Dead code in a security-relevant package increases cognitive load during audits.

---

### IN-02: `config.go` `loadKeyFromFile` will reject a 32-byte key file that has a trailing newline

**File:** `internal/config/config.go:238-244`
**Issue:** `os.ReadFile` returns raw bytes including any trailing `\n`. A key file created with `echo` or saved by a text editor typically has 33 bytes (32 key bytes + `\n`), which fails the `len(data) != 32` check with a confusing error. The `env` path handles this naturally (base64 decoding ignores such issues). There is no test for the wrong-length file case for the `file` provider.

**Fix:** Document that the key file must be exactly 32 raw bytes (no newline), and improve the error message to mention the symptom:
```go
if len(data) != 32 {
    return nil, fmt.Errorf(
        "ENCRYPTION_KEY_FILE %q must contain exactly 32 raw bytes (got %d); "+
            "if created with 'echo', use 'printf' instead, or strip the trailing newline",
        keyFile, len(data),
    )
}
```

---

### IN-03: `oauth_test.go` hand-rolls `contains()` instead of using `strings.Contains`

**File:** `internal/oauth/oauth_test.go:351-361`
**Issue:** The `contains()` helper is a correct but unnecessarily complex reimplementation of `strings.Contains` (74 characters of closure vs one stdlib call). The `strings` package is not imported in `oauth_test.go`, so the helper was written to avoid the import. The function is correct — but it adds audit burden: every reviewer must verify a non-trivial string-matching implementation in a security-relevant test file.

**Fix:** Import `strings` and use `strings.Contains` directly in all assertion sites. Delete the `contains()` helper.

---

_Reviewed: 2026-05-10T12:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
