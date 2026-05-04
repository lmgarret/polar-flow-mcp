# Project State

**Project:** polar-flow-mcp
**Milestone:** v1.0 — Initial Release
**Status:** Planning complete, ready to build

## Current Phase

**Phase 1: Project Foundation and Security Skeleton** — 5 plans created, ready to execute.

## Phase Progress

| # | Phase | Status |
|---|-------|--------|
| 1 | Project Foundation and Security Skeleton | Ready to execute (5 plans) |
| 2 | OAuth Link Flow + UserInfo | Not started |
| 3 | Core MCP Tools + Bundled Skill | Not started |
| 4 | Documentation, FOSS Hygiene, Release | Not started |

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-05-03)

**Core value:** A user can say "create a 5×1km threshold session for Thursday" in Claude and have it appear in Polar Flow — zero context-switching, zero manual UI navigation.
**Current focus:** Phase 1 — planned 2026-05-04, ready to execute

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
