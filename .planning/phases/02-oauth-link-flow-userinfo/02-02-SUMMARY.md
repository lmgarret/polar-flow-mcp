---
phase: 2
plan: "02-02"
subsystem: polar-client+oauth-handlers+crypto
tags: [oauth, polar-api, tdd, handlers, crypto, aes-gcm, csrf]
dependency_graph:
  requires: [02-01-SUMMARY.md]
  provides: [polar-exchange-code, polar-register-user, oauth-login-handler, oauth-callback-handler, bytes-key-provider, main-wiring]
  affects: [02-03-PLAN.md]
tech_stack:
  added: []
  patterns: [tdd-red-green, export-for-test-cross-pkg, atomic-csrf-consume, aes-gcm-nonce-ciphertext]
key_files:
  created:
    - internal/polar/client_test.go
    - internal/polar/export_test.go
    - internal/polar/testexports.go
    - internal/crypto/bytes_provider.go
    - internal/crypto/bytes_provider_test.go
    - internal/oauth/oauth.go
    - internal/oauth/export_test.go
    - internal/oauth/oauth_test.go
  modified:
    - internal/polar/client.go
    - cmd/polar-flow-mcp/main.go
decisions:
  - "SetTokenEndpoint/SetRegisterEndpoint moved to testexports.go (non-test file) so oauth_test package can call polar.Set* — export_test.go pattern only works within the same package test binary"
  - "409 from RegisterUser returns (0, nil); polar_user_id always derived from TokenResponse.XUserID"
  - "BytesKeyProvider validates exactly 32 bytes; returns fmt.Errorf mentioning byte count"
  - "htmlError uses html.EscapeString(msg); all msg values are static literals (defense-in-depth)"
  - "All test requests use httptest.NewRequestWithContext per noctx linter requirements"
metrics:
  duration: "18 min"
  completed_date: "2026-05-10"
  tasks_completed: 2
  files_created: 8
  files_modified: 2
---

# Phase 2 Plan 02: Polar Client + OAuth Handlers + main.go Wiring Summary

**One-liner:** Full Polar OAuth2 link flow — ExchangeCode/RegisterUser HTTP functions, stateful Handlers struct (Login/Callback), AES-256-GCM token encryption, BytesKeyProvider bridge, and main.go wired end-to-end with CSRF state validation and identity binding.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Polar client ExchangeCode and RegisterUser + tests | 264804d | internal/polar/client.go, client_test.go, export_test.go |
| 2 | BytesKeyProvider + OAuth Handlers + main.go wiring + tests | ba4b000 | internal/crypto/bytes_provider.go+test, internal/oauth/oauth.go+export_test+oauth_test, internal/polar/testexports.go, cmd/polar-flow-mcp/main.go |

## What Was Built

### Task 1: Polar Client Functions

**internal/polar/client.go** — appended to existing Client struct:

- `TokenResponse` struct: `AccessToken`, `TokenType`, `XUserID int64` (JSON `x_user_id`)
- `ExchangeCode(ctx, clientID, clientSecret, code, redirectURL)` — POST to `polarremote.com/v2/oauth2/token` with Basic auth and form body; returns `*TokenResponse` or error with status code and body
- `RegisterUser(ctx, accessToken)` — POST to `polaraccesslink.com/v3/users` with Bearer token; HTTP 409 returns `(0, nil)` (idempotent); other non-200 returns error
- Package-level vars `tokenEndpoint`, `registerEndpoint`, `defaultHTTPClient` with `//nolint:gochecknoglobals`
- All requests use `http.NewRequestWithContext` (noctx compliant)

Five tests: `TestExchangeCode_Success` (asserts Basic auth header, Content-Type, body params), `TestExchangeCode_NonOKStatus` (400 + body in error), `TestRegisterUser_Success` (asserts Bearer header + member-id body), `TestRegisterUser_Conflict` (409 → 0,nil), `TestRegisterUser_OtherError` (500 → error).

### Task 2: BytesKeyProvider + OAuth Handlers + Wiring

**internal/crypto/bytes_provider.go** — `BytesKeyProvider` implementing `KeyProvider` interface:
- `NewBytesKeyProvider(key []byte)` — wraps already-validated raw bytes
- `Key()` — validates exactly 32 bytes, returns `fmt.Errorf` naming actual length on mismatch

**internal/oauth/oauth.go** — complete replacement of stub:
- `Handlers` struct with `cfg`, `st`, `cipher` fields
- `NewHandlers(cfg, st, cipher)` constructor
- `Login` handler: extracts identity from context, generates 32-byte random state (64-char hex), stores in `pending_auth` with 10-min expiry, redirects to `flow.polar.com/oauth2/authorization` with all required OAuth params
- `Callback` handler: validates state (ErrNotFound→400, ErrExpired→400), checks identity binding (mismatch→400+slog.Warn), exchanges code, registers user (409=ok), derives `polar_user_id = strconv.FormatInt(XUserID, 10)`, upserts user+encrypted token
- `htmlSuccess` / `htmlError` helpers with `Content-Type: text/html; charset=utf-8`; `htmlError` uses `html.EscapeString` on message

**cmd/polar-flow-mcp/main.go** — wiring:
- Added `crypto` import
- After `store.Open`: `cipher := crypto.NewCipher(crypto.NewBytesKeyProvider(cfg.EncryptionKey))`
- Before mux: `oauthHandlers := oauth.NewHandlers(cfg, st, cipher)`
- Replaced `oauth.LoginHandler`/`oauth.CallbackHandler` package-level stubs with `oauthHandlers.Login`/`oauthHandlers.Callback`

Seven oauth tests covering: redirect shape + state stored, no-identity→500, invalid state→400, expired state→400, identity mismatch→400, success (users+tokens persisted+replay→400), 409-conflict uses XUserID.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Moved Set* functions from export_test.go to testexports.go**
- **Found during:** Task 2 implementation
- **Issue:** Plan instructed creating `export_test.go` (package polar) with `SetTokenEndpoint`/`SetRegisterEndpoint`. Files ending in `_test.go` are only compiled in the package's own test binary — not visible to `internal/oauth` test binaries calling `polar.SetTokenEndpoint`. This caused a compile error.
- **Fix:** Created `testexports.go` (non-test file, package polar) with the setter functions. Kept `export_test.go` as a marker file (package polar) with a documentation comment. Functions are safe to expose because `polar` is an internal package unreachable outside the module.
- **Files modified:** internal/polar/testexports.go (created), internal/polar/export_test.go (marker only)
- **Commit:** ba4b000

**2. [Rule 1 - Bug] Fixed noctx lint errors in oauth_test.go**
- **Found during:** Task 2 lint run
- **Issue:** All `httptest.NewRequest(...)` calls in `oauth_test.go` triggered the `noctx` linter (same rule that applies to production code).
- **Fix:** Replaced all 8 occurrences with `httptest.NewRequestWithContext(context.Background(), ...)`.
- **Files modified:** internal/oauth/oauth_test.go
- **Commit:** ba4b000

## Test Results

```
ok  github.com/lm/polar-flow-mcp/internal/crypto  0.007s
ok  github.com/lm/polar-flow-mcp/internal/oauth   0.018s
ok  github.com/lm/polar-flow-mcp/internal/polar   0.007s
ok  github.com/lm/polar-flow-mcp/internal/store   0.029s
ok  github.com/lm/polar-flow-mcp/internal/auth    0.004s
ok  github.com/lm/polar-flow-mcp/internal/config  0.004s
```

Lint: `0 issues` on all packages.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints introduced beyond those in the plan's threat model. All OAuth handler paths are gated by `auth.Middleware` (X-Proxy-Secret + identity header) in `main.go`. No user input is reflected into HTML responses without `html.EscapeString`. Client secrets never appear in log lines or response bodies.

## Self-Check: PASSED

- internal/polar/client.go: FOUND
- internal/polar/client_test.go: FOUND
- internal/polar/export_test.go: FOUND
- internal/polar/testexports.go: FOUND
- internal/crypto/bytes_provider.go: FOUND
- internal/crypto/bytes_provider_test.go: FOUND
- internal/oauth/oauth.go: FOUND
- internal/oauth/export_test.go: FOUND
- internal/oauth/oauth_test.go: FOUND
- cmd/polar-flow-mcp/main.go: FOUND (modified)
- Commit 264804d: FOUND
- Commit ba4b000: FOUND
