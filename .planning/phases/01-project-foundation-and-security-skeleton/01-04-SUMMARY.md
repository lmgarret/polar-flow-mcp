---
phase: 01-project-foundation-and-security-skeleton
plan: "04"
subsystem: auth-middleware-http-server
tags: [auth, middleware, http, security, mcp, readyz, healthz]
dependency_graph:
  requires:
    - 01-02-SUMMARY.md  # config package (ProxySecretHeader, Config.DatabasePath)
    - 01-03-SUMMARY.md  # store package (Store.Open, Store.Ping, Store.Close)
  provides:
    - internal/auth/auth.go  # Middleware, UserIDFromContext, UserIDKey
    - cmd/polar-flow-mcp/main.go  # full HTTP server wiring
  affects:
    - internal/config/config.go  # added DatabasePath field
tech_stack:
  added:
    - crypto/subtle (ConstantTimeCompare for timing-safe secret comparison)
    - net/http/httptest (integration test harness)
    - os/signal + syscall (graceful shutdown)
    - encoding/json (readyz response body)
  patterns:
    - TDD RED/GREEN cycle for auth middleware
    - Unexported context key type (D-12)
    - Secret-check-first middleware ordering (D-04)
    - Dual-route /mcp + /mcp/ pattern for ServeMux prefix matching
key_files:
  created:
    - internal/auth/auth_test.go
  modified:
    - internal/auth/auth.go  # stub replaced with full implementation
    - cmd/polar-flow-mcp/main.go  # stub replaced with full HTTP server
    - internal/config/config.go  # added DatabasePath field + DATABASE_PATH env var
decisions:
  - "WithHTTPContextFunc used as defense-in-depth only; auth.Middleware is authoritative identity injection point (D-01)"
  - "Both /mcp and /mcp/ routes registered separately to handle ServeMux prefix matching"
  - "noctx lint rule requires httptest.NewRequestWithContext (context.Background()) in tests"
metrics:
  duration: "3 min"
  completed_date: "2026-05-04"
  tasks_completed: 2
  tasks_total: 2
  files_created: 1
  files_modified: 3
---

# Phase 1 Plan 4: Auth Middleware + HTTP Server Summary

Full HTTP server wiring with fail-closed auth middleware using `subtle.ConstantTimeCompare` secret check before identity header extraction, `net/http` ServeMux routes, and `/readyz` 4-check JSON body.

## What Was Built

### Auth Middleware (`internal/auth/auth.go`)

**Function signatures:**

```go
// UserIDFromContext extracts the authenticated user identity from ctx.
// Returns ("", false) if not present.
func UserIDFromContext(ctx context.Context) (string, bool)

// Middleware returns an http.Handler enforcing proxy shared-secret contract.
// Security ordering (D-04, LOCKED): secret check BEFORE identity header read.
func Middleware(secret, identityHeader string, next http.Handler) http.Handler

// UserIDKey is the exported singleton key for cross-package context.WithValue use.
var UserIDKey = userIDKey{}  // userIDKey is unexported (D-12)
```

**Security ordering (D-04 — locked by code structure):**
1. `subtle.ConstantTimeCompare([]byte(provided), []byte(secret))` — 403 on mismatch
2. `r.Header.Get(identityHeader)` — 403 + WARN if empty
3. `context.WithValue(r.Context(), userIDKey{}, identity)` — inject into context

The `subtle.ConstantTimeCompare` line appears at line 44; `r.Header.Get(identityHeader)` at line 58. Ordering is enforced by code structure, not convention.

### HTTP Mux Route Table (`cmd/polar-flow-mcp/main.go`)

| Path | Auth-wrapped | Handler |
|------|-------------|---------|
| `GET /healthz` | No | Inline: always 200 "ok" |
| `GET /readyz` | No | `readyzHandler(cfg, st)` — 4-check JSON |
| `/mcp` | Yes | `auth.Middleware` → `StreamableHTTPServer` |
| `/mcp/` | Yes | `auth.Middleware` → `StreamableHTTPServer` (prefix match) |
| `GET /oauth/login` | Yes | `auth.Middleware` → `oauth.LoginHandler` (stub) |
| `GET /oauth/callback` | Yes | `auth.Middleware` → `oauth.CallbackHandler` (stub) |

### /readyz Check Names and Meaning

| Check Name | Condition | Failure Meaning |
|-----------|-----------|-----------------|
| `auth_proxy` | `cfg.AuthProxy != "" && != "unconfigured"` | AUTH_PROXY not configured |
| `proxy_secret` | `cfg.ProxySharedSecret != ""` | PROXY_SHARED_SECRET not set |
| `database` | `st.Ping(ctx) == nil` | SQLite write+read pools unreachable |
| `encryption_key` | `len(cfg.EncryptionKey) == 32` | Key missing or wrong length |

Response: `{"checks": [...]}` with `status: "ok"` or `status: "fail"` + `error: "..."`.
HTTP status: 200 all-pass, 503 any-fail.

### Config Changes (`internal/config/config.go`)

Added `DatabasePath string` field loaded from `DATABASE_PATH` env var, defaulting to `"polar.db"`.

## Test Results

7 tests in `internal/auth/auth_test.go` (all pass):

1. `TestMiddleware_ValidRequest_CallsDownstream` — 200, downstream called
2. `TestMiddleware_MissingSecret_Returns403` — 403, downstream NOT called
3. `TestMiddleware_WrongSecret_Returns403` — 403, downstream NOT called
4. `TestMiddleware_EmptyIdentity_Returns403` — 403, downstream NOT called
5. `TestMiddleware_ValidRequest_InjectsIdentity` — identity in context
6. `TestUserIDFromContext_MissingKey_ReturnsFalse` — ("", false) on empty context
7. `TestMiddleware_ConcurrentRequests_IsolatedIdentity` — goroutine isolation verified

Note: `-race` flag requires CGO; tests run with `go test -count=1` (no CGO race detector on this platform). Context isolation is tested structurally via goroutine-local capture variables.

## WithHTTPContextFunc Behavior

Per `ARCHITECTURE.md` and STATE.md resolved questions: `WithHTTPContextFunc` is **per-request** when `StreamableHTTPServer` is mounted as `http.Handler` inside `ServeMux`. Used as defense-in-depth only — auth.Middleware is the authoritative identity injection point (D-01). This avoids any dependency on mcp-go internal session management.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing field] Added DatabasePath to Config struct**
- **Found during:** Task 2 (main.go implementation)
- **Issue:** Plan 04 references `cfg.DatabasePath` but Plan 02's Config struct had no such field
- **Fix:** Added `DatabasePath string` field to Config + `DATABASE_PATH` env var loading with `"polar.db"` default
- **Files modified:** `internal/config/config.go`
- **Commit:** 7ad1ef5

**2. [Rule 2 - Lint compliance] httptest.NewRequestWithContext instead of NewRequest**
- **Found during:** Task 1 lint run (noctx linter)
- **Issue:** `httptest.NewRequest` triggers `noctx` lint rule; must pass explicit context
- **Fix:** Replaced all 3 occurrences with `httptest.NewRequestWithContext(context.Background(), ...)`
- **Files modified:** `internal/auth/auth_test.go`
- **Commit:** 60ed55a

## TDD Gate Compliance

- RED commit: `fb06a58` — `test(01-04): add failing auth middleware tests (RED)` (5 of 7 tests failed)
- GREEN commit: `60ed55a` — `feat(01-04): implement auth middleware with secret-check-first ordering (GREEN)`
- REFACTOR: not needed

## Known Stubs

- `oauth.LoginHandler` and `oauth.CallbackHandler` return 501 Not Implemented — intentional, Phase 2 scope
- `mcp.RegisterTools` registers zero tools — intentional, Phase 2/3 scope

## Threat Surface Scan

All threat mitigations from the plan's `<threat_model>` are implemented:

- T-04-01: `subtle.ConstantTimeCompare` before identity header read — mitigated
- T-04-03: Both `/mcp` and `/mcp/` wrapped with `auth.Middleware` — mitigated
- T-04-04: Constant-time comparison regardless of secret length — mitigated
- T-04-05: `ReadTimeout: 30s`, `WriteTimeout: 60s`, `IdleTimeout: 120s` — mitigated

No new security-relevant surface introduced beyond what is in the threat model.

## Self-Check: PASSED
