---
phase: 01-project-foundation-and-security-skeleton
plan: 02
subsystem: config
tags: [go, config, fail-closed, aes-256-gcm, slog, env-vars]

requires:
  - phase: 01-project-foundation-and-security-skeleton
    provides: Go module scaffold, package stubs including internal/config stub

provides:
  - config.Config struct with full field set (AuthProxy, ProxySharedSecret, IdentityHeader, BindAddress, KeyProviderType, EncryptionKey)
  - config.Load() fail-closed validation with actionable error messages
  - config.LogStartupBanner() slog startup banner
  - config.ProxySecretHeader constant ("X-Proxy-Secret")
  - main.go wired to Load() with slog.Error + os.Exit(1) on failure
  - Non-loopback bind address warning in main.go

affects:
  - 01-03-auth-middleware (reads ProxySecretHeader, Config.ProxySharedSecret, Config.IdentityHeader)
  - 01-04-http-server (reads Config.BindAddress)
  - all downstream phases that import internal/config

tech-stack:
  added: []
  patterns:
    - "Fail-closed startup: Load() validates all invariants before returning; main calls os.Exit(1) on any error"
    - "Actionable error messages: each error names the env var, the required format, and a generation command"
    - "Dual key provider: KEY_PROVIDER=env decodes ENCRYPTION_KEY base64; KEY_PROVIDER=file reads ENCRYPTION_KEY_FILE raw bytes"
    - "Startup banner via LogStartupBanner(cfg): auth_proxy, identity_header, secret_header, bind_address"
    - "Non-loopback BIND_ADDRESS emits slog.Warn at startup (threat T-02-04)"

key-files:
  created: []
  modified:
    - internal/config/config.go
    - cmd/polar-flow-mcp/main.go
    - .gitignore

key-decisions:
  - "EncryptionKey stored as []byte in Config struct (not as raw strings); crypto package reads it directly"
  - ".gitignore pattern /polar-flow-mcp (anchored to root) avoids matching cmd/polar-flow-mcp/ directory"

patterns-established:
  - "loadAuthFields / loadOptionalFields / loadEncryptionKey private helpers keep Load() linear and testable"
  - "Tests use clearConfigEnv(t) helper + t.Setenv() — no global state leakage between tests"

requirements-completed:
  - FOUND-02
  - FOUND-03
  - FOUND-04
  - FOUND-10

duration: 8min
completed: 2026-05-04
---

# Phase 1 Plan 02: Config Fail-Closed Validation Summary

**config.Load() validates AUTH_PROXY, PROXY_SHARED_SECRET, and 32-byte AES key at startup — server refuses to start with actionable slog errors on any misconfiguration**

## Performance

- **Duration:** 8 min
- **Started:** 2026-05-04T09:00:00Z
- **Completed:** 2026-05-04T09:08:00Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- Full Config struct with 6 exported fields, all validated before returning
- Fail-closed validation: AUTH_PROXY != empty/"unconfigured", PROXY_SHARED_SECRET non-empty, ENCRYPTION_KEY or ENCRYPTION_KEY_FILE exactly 32 bytes
- 9 unit tests covering all error paths and defaults (AUTH_PROXY, PROXY_SHARED_SECRET, ENCRYPTION_KEY length, key file, BIND_ADDRESS default, IDENTITY_HEADER default)
- main.go calls config.Load(), exits 1 with slog.Error on failure, logs startup banner on success, warns on non-loopback bind address

## Config Struct Fields

| Field | Env Var | Default | Notes |
|-------|---------|---------|-------|
| AuthProxy | AUTH_PROXY | — (required) | Must not be empty or "unconfigured" |
| ProxySharedSecret | PROXY_SHARED_SECRET | — (required) | Must be non-empty |
| IdentityHeader | IDENTITY_HEADER | "Remote-User" | Header injected by reverse proxy |
| BindAddress | BIND_ADDRESS | "127.0.0.1" | Non-loopback triggers slog.Warn |
| KeyProviderType | KEY_PROVIDER | "env" | "env" or "file" |
| EncryptionKey | ENCRYPTION_KEY / ENCRYPTION_KEY_FILE | — (required) | 32 bytes; base64 for env, raw for file |

## Constants

- `ProxySecretHeader = "X-Proxy-Secret"` — HTTP header name for the proxy shared secret

## Exact Error Messages

| Trigger | Error text |
|---------|-----------|
| AUTH_PROXY="" or "unconfigured" | "AUTH_PROXY must be set to the name of your reverse proxy (e.g. authelia, authentik, oauth2-proxy); see https://github.com/lm/polar-flow-mcp/docs/deployment for configuration" |
| PROXY_SHARED_SECRET="" | "PROXY_SHARED_SECRET must be set to a non-empty secret shared with your reverse proxy; see deployment docs" |
| ENCRYPTION_KEY="" (env mode) | "ENCRYPTION_KEY must be set to a base64-encoded 32-byte key; generate one with: openssl rand -base64 32" |
| ENCRYPTION_KEY invalid base64 | "ENCRYPTION_KEY is not valid base64: ...; regenerate with: openssl rand -base64 32" |
| ENCRYPTION_KEY decodes to != 32 bytes | "ENCRYPTION_KEY decoded length must be 32 bytes (got N); regenerate with: openssl rand -base64 32" |
| ENCRYPTION_KEY_FILE="" (file mode) | "ENCRYPTION_KEY_FILE must be set to the path of a file containing a 32-byte key" |
| ENCRYPTION_KEY_FILE unreadable | "failed to read ENCRYPTION_KEY_FILE \"path\": <os error>" |
| ENCRYPTION_KEY_FILE != 32 bytes | "ENCRYPTION_KEY_FILE \"path\" must contain exactly 32 bytes (got N)" |

## Task Commits

1. **Task 1: Config struct, Load(), and fail-closed validation (RED)** - `6360a4d` (test)
2. **Task 2: Implement config.Load() + wire main.go (GREEN)** - `3fea6fc` (feat)

## Files Created/Modified

- `internal/config/config.go` — Full Config struct, Load(), LogStartupBanner(), ProxySecretHeader constant
- `cmd/polar-flow-mcp/main.go` — Calls config.Load(), slog.Error+os.Exit(1) on failure, LogStartupBanner on success, non-loopback bind address warning
- `.gitignore` — Added /polar-flow-mcp (anchored, avoids matching cmd/ subdirectory)

## Decisions Made

- EncryptionKey stored as `[]byte` directly in Config (not as key provider type) — crypto package (Plan 03) receives the bytes and constructs KeyProvider implementations from them
- .gitignore pattern anchored to `/polar-flow-mcp` so `go build ./cmd/polar-flow-mcp` output in repo root is ignored without affecting `cmd/polar-flow-mcp/` source directory

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed .gitignore pattern collision**
- **Found during:** Task 2 (after building binary)
- **Issue:** `git add cmd/polar-flow-mcp/main.go` failed because `.gitignore` entry `polar-flow-mcp` (unanchored) matched the `cmd/polar-flow-mcp` directory
- **Fix:** Changed to `/polar-flow-mcp` (root-anchored) so it matches only the built binary in the repo root
- **Files modified:** .gitignore
- **Verification:** `git add cmd/polar-flow-mcp/main.go` succeeds; `git status` shows binary as untracked then ignored
- **Committed in:** 3fea6fc

**2. [Rule 1 - Bug] -race flag requires CGO on Linux**
- **Found during:** Task 1 verification
- **Issue:** `CGO_ENABLED=0 go test -race` exits with "go: -race requires cgo; enable cgo by setting CGO_ENABLED=1" on Linux/amd64
- **Fix:** Run tests as `CGO_ENABLED=0 go test -count=1 ./internal/config/...` (drop -race flag when CGO is disabled)
- **Files modified:** None (test invocation only)
- **Verification:** Tests pass with `-count=1` without `-race`

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both minor. Gitignore fix was prerequisite to staging files. Race-detector workaround is a Linux/CGO platform constraint that does not affect test coverage or correctness.

## Issues Encountered

None beyond the two deviations documented above.

## Known Stubs

None. config.Load() is fully implemented and all fields are wired. main.go uses `select {}` as a server placeholder pending Plan 04 HTTP server wiring — this is documented in the plan as intentional (Plan 04 replaces it).

## Next Phase Readiness

- Config package is fully implemented and tested; Plans 03 (crypto), 04 (HTTP/auth middleware), and 05 (MCP/SQLite) can all import and use config.Config
- ProxySecretHeader constant is available for the auth middleware (Plan 03/04)
- EncryptionKey []byte is available for crypto.NewKeyProvider (Plan 03)
- main.go select{} will be replaced in Plan 04 with the real HTTP server bind

---
*Phase: 01-project-foundation-and-security-skeleton*
*Completed: 2026-05-04*
