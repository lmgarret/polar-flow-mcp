# Project State

**Project:** polar-flow-mcp
**Milestone:** v1.0 — Initial Release
**Status:** Planning complete, ready to build

## Current Phase

**None** — not started. Run `/gsd-discuss-phase 1` to begin.

## Phase Progress

| # | Phase | Status |
|---|-------|--------|
| 1 | Project Foundation and Security Skeleton | Not started |
| 2 | OAuth Link Flow + UserInfo | Not started |
| 3 | Core MCP Tools + Bundled Skill | Not started |
| 4 | Documentation, FOSS Hygiene, Release | Not started |

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-05-03)

**Core value:** A user can say "create a 5×1km threshold session for Thursday" in Claude and have it appear in Polar Flow — zero context-switching, zero manual UI navigation.
**Current focus:** None — planning phase complete

## Performance Metrics

- Plans completed: 0 / 14
- Phases completed: 0 / 4
- Requirements delivered: 0 / 40

## Accumulated Context

### Key Decisions Locked

- MCP transport: StreamableHTTP (`NewStreamableHTTPServer` + `WithHTTPContextFunc`)
- Auth model: proxy contract only; fail-closed (`AUTH_PROXY` ≠ `"unconfigured"` or refuse to start)
- SQLite driver: `modernc.org/sqlite` (pure Go, CGO disabled) — NOT `mattn/go-sqlite3`
- golang-migrate sqlite driver suffix: `sqlite` NOT `sqlite3` (CGO impact is total)
- Crypto: AES-256-GCM, nonce (12 bytes) || ciphertext stored as single BLOB column
- Final image: `FROM scratch` with `ca-certificates.crt` copied from builder
- Context key: unexported struct type (prevents cross-package collision)
- Middleware ordering: `subtle.ConstantTimeCompare` secret check BEFORE identity header read

### Open Questions (resolve during Phase 1 planning)

- `WithHTTPContextFunc` per-request vs per-session when `StreamableHTTPServer` is mounted as `http.Handler` — safe fallback: extract identity in auth middleware via `context.WithValue`
- Polar API live training target JSON shapes — validate in Phase 2 before Phase 3 builds on them

### Blockers

None.

---
*Initialized: 2026-05-03*
