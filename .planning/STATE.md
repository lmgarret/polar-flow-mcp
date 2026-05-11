---
gsd_state_version: 1.0
milestone: v1.0.0
milestone_name: milestone
current_plan: 03-03 (polar-coach skill)
status: executing
last_updated: "2026-05-11T15:23:02.894Z"
progress:
  total_phases: 4
  completed_phases: 3
  total_plans: 12
  completed_plans: 12
  percent: 100
---

# Project State

**Project:** polar-flow-mcp
**Milestone:** v1.0 — Initial Release
**Status:** Ready to execute

## Current Phase

**Phase 3: Core MCP Tools + Bundled Skill** — COMPLETE (3/3 plans done).

**Previous completed:** Phase 2: OAuth Link Flow + UserInfo — COMPLETE (4/4 plans done).

**Current plan:** 04-01 (Phase 4: Documentation, FOSS Hygiene, Release)

## Phase Progress

| # | Phase | Status |
|---|-------|--------|
| 1 | Project Foundation and Security Skeleton | COMPLETE (5/5 plans done) |
| 2 | OAuth Link Flow + UserInfo | COMPLETE (4/4 plans done) |
| 3 | Core MCP Tools + Bundled Skill | COMPLETE (3/3 plans done) |
| 4 | Documentation, FOSS Hygiene, Release | Not started |

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-05-03)

**Core value:** A user can say "create a 5×1km threshold session for Thursday" in Claude and have it appear in Polar Flow — zero context-switching, zero manual UI navigation.
**Current focus:** Phase 4 — Documentation, FOSS Hygiene, Release

## Performance Metrics

- Plans completed: 12 / 14
- Phases completed: 3 / 4
- Requirements delivered: 33 / 40

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
| 02-04 gap closure CR-01/02/03 | 20 min | 3/3 | 11 modified, 1 deleted |
| 03-01 create_training_target | 6 min | 3/3 | 3 created, 7 modified |
| 03-02 list+delete training targets | 5 min | 2/2 | 4 created, 3 modified |
| 03-03 polar-coach skill | 8 min | 1/1 | 1 created |

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
- `ConsumeOAuthState` uses conditional `DELETE WHERE expires_at >= datetime('now') RETURNING` + diagnostic SELECT; expired rows are NOT deleted (CR-01 fix)
- `UpsertToken` resolves `user_id` FK via `(SELECT id FROM users WHERE identity=?)` subquery; rejects missing users
- `polar_user_id` stored as TEXT per schema ground truth (not int64)
- `SetTokenEndpoint`/`SetRegisterEndpoint` in `testexports.go` gated behind `//go:build polartest`; all `go test` invocations pass `-tags=polartest`; golangci-lint `run.build-tags: [polartest]` — excludes symbols from production binaries (CR-03)
- `polar_user_id` always from `TokenResponse.XUserID` (not 409 body) — 409 response body omits user object
- `BytesKeyProvider` validates exactly 32 bytes at `Key()` call time, not at construction
- `GetUserInfoHandler` exported (not unexported) — allows cross-package test invocation without live server
- Tool handlers read identity exclusively from context via `auth.UserIDFromContext`; `CallToolRequest` params ignored (T-02-03-01)
- `return` required after `t.Fatal(nil-guard)` in tests to satisfy SA5011 staticcheck flow analysis
- JSON round-trip (GetArguments() -> json.Marshal -> json.Unmarshal) for phases array parsing; robust regardless of mcp-go internal Arguments representation
- buildRepeatPhase/flatToTree extracted to satisfy gocyclo<=15 (handler body was cyclomatic 29 inline)
- phaseInput.HRZone as float64 — JSON numbers from map[string]any are float64; cast to int for resolveZone
- `store.GetEncryptedToken` uses read pool (s.readDB) via JOIN polar_tokens/users — same pattern as GetPolarUserID
- `SetTrainingTargetsBaseURL` gated behind `//go:build polartest` — production binary excludes test setter
- `polar.ErrTargetNotFound` sentinel exported; handlers use `errors.Is` for 404 discrimination (informational, not error)
- `url.PathEscape(targetID)` in DeleteTrainingTarget prevents path injection (T-03-02-07)
- ListTrainingTargets uses two-pass JSON unmarshal (array first, then wrapped-object) to tolerate both assumed Polar response shapes
- SKILL.md authored in Wave 3 after mcp.go registrations are stable — tool names verified verbatim via grep before writing
- Iterative ask-then-create pattern for marathon plans: user stays in control of batch target creation (T-03-03-04 mitigation)
- Intensity labels preferred over hr_zone integers in polar-coach skill per D-06

### Open Questions

- Polar API live training target JSON shapes — validate against live account (03-01 builds on ASSUMED v4 swagger schema)

### Resolved (Phase 1 context)

- `WithHTTPContextFunc` per-request vs per-session → **resolved: auth middleware only** (guaranteed per-request, no mcp-go internals dependency)
- Default identity header → **`Remote-User`**, configurable via `IDENTITY_HEADER`
- Missing identity header behavior → **403 + warning log**
- Phase 1 test scope → **unit (crypto + config) + httptest integration (startup, /healthz, /readyz, middleware)**

### Blockers

None.

---
*Initialized: 2026-05-03*
*Last session: 2026-05-11 — Completed 03-03-PLAN.md (polar-coach skill — SKILL.md 227 lines; HR zone table; 5 worked examples; safe degradation; both install paths; all 4 tool names verified verbatim; Phase 3 COMPLETE)*
