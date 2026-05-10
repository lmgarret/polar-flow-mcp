---
gsd_state_version: 1.0
milestone: v1.0.0
milestone_name: milestone
status: executing
last_updated: "2026-05-10T20:30:00.000Z"
progress:
  total_phases: 4
  completed_phases: 2
  total_plans: 8
  completed_plans: 8
  percent: 100
---

# Project State

**Project:** polar-flow-mcp
**Milestone:** v1.0 — Initial Release
**Status:** Ready to execute

## Current Phase

**Phase 2: OAuth Link Flow + UserInfo** — COMPLETE (3/3 plans done).

**Previous:** Phase 1: Project Foundation and Security Skeleton — 5/5 plans COMPLETE.

**Next:** Phase 3: Core MCP Tools + Bundled Skill

## Phase Progress

| # | Phase | Status |
|---|-------|--------|
| 1 | Project Foundation and Security Skeleton | COMPLETE (5/5 plans done) |
| 2 | OAuth Link Flow + UserInfo | COMPLETE (3/3 plans done) |
| 3 | Core MCP Tools + Bundled Skill | Not started |
| 4 | Documentation, FOSS Hygiene, Release | Not started |

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-05-03)

**Core value:** A user can say "create a 5×1km threshold session for Thursday" in Claude and have it appear in Polar Flow — zero context-switching, zero manual UI navigation.
**Current focus:** Phase 3 — Core MCP Tools + Bundled Skill

## Performance Metrics

- Plans completed: 8 / 14
- Phases completed: 2 / 4
- Requirements delivered: 22 / 40

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| 01-01 scaffold | 4 min | 2/2 | 13 created |
| 01-02 config validation | 8 min | 2/2 | 3 modified |
| 01-03 crypto+store | 18 min | 2/2 | 8 created |
| 01-04 auth middleware+HTTP server | 3 min | 2/2 | 4 modified |
| 01-05 Docker + CI | 5 min | 2/2 | 3 created |
| 02-01 config+store oauth foundations | 12 min | 2/2 | 8 created/modified |
| 02-02 polar client + oauth handlers | 18 min | 2/2 | 10 created/modified |
| 02-03 get_user_info MCP tool | 5 min | 1/1 | 1 created, 2 modified |

## Accumulated Context

### Key Decisions Locked

- MCP transport: StreamableHTTP (`NewStreamableHTTPServer` + `WithHTTPContextFunc`)
- Auth model: proxy contract only; fail-closed (`AUTH_PROXY` ≠ `"unconfigured"` or refuse to start)
- SQLite driver: `modernc.org/sqlite` (pure Go, CGO disabled) — NOT `mattn/go-sqlite3`
- golang-migrate sqlite driver suffix: `sqlite` NOT `sqlite3` (CGO impact is total)
- Crypto: AES-256-GCM, nonce (12 bytes) || ciphertext stored as single BLOB column
- Final image: `FROM scratch` (11.46 MB) with `ca-certificates.crt` copied from builder; `golang:1.26-alpine` builder
- CI: three jobs (test + lint → docker); golangci-lint-action@v9 v2.11; ghcr.io push with latest + sha-* tags
- docker-compose.yml ships AUTH_PROXY=unconfigured fail-closed sentinel (deliberate UX contract for operators)
- Context key: unexported struct type (prevents cross-package collision)
- Middleware ordering: `subtle.ConstantTimeCompare` secret check BEFORE identity header read

- EncryptionKey stored as `[]byte` in Config struct; crypto package (Plan 03) constructs KeyProvider from it
- `config.ProxySecretHeader = "X-Proxy-Secret"` — constant lives in config package (auth imports config)
- Non-loopback BIND_ADDRESS emits `slog.Warn` at startup (threat T-02-04 mitigation)
- Migrations placed in `internal/store/migrations/` (not repo root) — simplifies `//go:embed` path
- WAL pragma check in tests accepts `"memory"` for `:memory:` DSN — SQLite in-memory DB always returns `memory` for journal_mode; file DSNs return `wal`
- `EnvKeyProvider.Key()` tries StdEncoding then URLEncoding base64 — operator-friendly
- `DatabasePath` added to Config; loaded from `DATABASE_PATH` env var, defaults to `"polar.db"`
- Both `/mcp` and `/mcp/` routes registered separately to handle ServeMux prefix matching
- `WithHTTPContextFunc` used as defense-in-depth only; `auth.Middleware` is authoritative identity injection (D-01)
- `ErrNotFound` and `ErrExpired` are exported sentinel errors from the store package
- `ConsumeOAuthState` uses `DELETE...RETURNING` for atomic single-use CSRF state consumption
- `UpsertToken` resolves `user_id` FK via `(SELECT id FROM users WHERE identity=?)` subquery; rejects missing users
- `polar_user_id` stored as TEXT per schema ground truth (not int64)
- `SetTokenEndpoint`/`SetRegisterEndpoint` in `testexports.go` (non-test file) — only way to expose package vars to external test binaries; `export_test.go` pattern only works within same package test binary
- `polar_user_id` always from `TokenResponse.XUserID` (not 409 body) — 409 response body omits user object
- `BytesKeyProvider` validates exactly 32 bytes at `Key()` call time, not at construction
- `GetUserInfoHandler` exported (not unexported) — allows cross-package test invocation without live server
- Tool handlers read identity exclusively from context via `auth.UserIDFromContext`; `CallToolRequest` params ignored (T-02-03-01)
- `return` required after `t.Fatal(nil-guard)` in tests to satisfy SA5011 staticcheck flow analysis

### Open Questions

- Polar API live training target JSON shapes — validate in Phase 2 before Phase 3 builds on them

### Resolved (Phase 1 context)

- `WithHTTPContextFunc` per-request vs per-session → **resolved: auth middleware only** (guaranteed per-request, no mcp-go internals dependency)
- Default identity header → **`Remote-User`**, configurable via `IDENTITY_HEADER`
- Missing identity header behavior → **403 + warning log**
- Phase 1 test scope → **unit (crypto + config) + httptest integration (startup, /healthz, /readyz, middleware)**

### Blockers

None.

---
*Initialized: 2026-05-03*
*Last session: 2026-05-10 — Completed 02-03-PLAN.md (get_user_info MCP tool; Phase 2 gate green: build + race tests + lint all pass)*
