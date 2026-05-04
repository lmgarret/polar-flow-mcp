# Phase 1: Project Foundation and Security Skeleton - Context

**Gathered:** 2026-05-04
**Status:** Ready for planning

<domain>
## Phase Boundary

A compilable, containerizable Go server with all irreversible security and infrastructure decisions locked in — middleware ordering, crypto schema, SQLite concurrency model, and CI pipeline. Every subsequent phase builds on top of this stable, auditable base. Nothing user-visible ships in Phase 1 beyond `/healthz` and `/readyz`.

</domain>

<decisions>
## Implementation Decisions

### Identity Injection

- **D-01:** User identity is injected via **auth middleware only** — extracted from the proxy identity header after the shared-secret check passes, then stashed in `context.WithValue` using an unexported struct context key. `WithHTTPContextFunc` is NOT used for identity propagation (its per-request vs per-session behavior when mounted as `http.Handler` is unverified and unreliable).
- **D-02:** Default identity header is `Remote-User` (standard for Authelia, Authentik, oauth2-proxy). Configurable via `IDENTITY_HEADER` env var.
- **D-03:** Missing or empty identity header on any route except `/healthz` and `/readyz` returns **403** with a warning log entry (probable proxy misconfiguration, not an attack signal).
- **D-04:** Secret check (`subtle.ConstantTimeCompare` on `PROXY_SHARED_SECRET`) runs BEFORE the identity header is read — this ordering is locked and must not be inverted.

### Test Scope

- **D-05:** Phase 1 tests cover two layers:
  1. **Unit tests**: `crypto` package (KeyProvider `env` and `file` impls, AES-256-GCM round-trip, nonce uniqueness across 1000 encryptions), `config` package (fail-closed validation: `AUTH_PROXY=unconfigured`, empty `PROXY_SHARED_SECRET`, missing/bad key).
  2. **Integration tests** (via `net/http/httptest`): server startup behavior, `/healthz` always-200, `/readyz` 503-with-named-checks vs 200-when-all-satisfied, shared-secret middleware (403 on mismatch, 403 on missing identity).
- **D-06:** Nonce uniqueness test: generate 1000 encryptions, assert all 1000 nonces are distinct. Validates the critical invariant that the immutable crypto schema cannot produce nonce reuse.
- **D-07:** `/readyz` integration test verifies BOTH the write pool and the read pool are responsive (not just one ping) — a wedged write pool with healthy read pool must result in 503.

### Locked Decisions (pre-decided, not re-discussed)

The following are locked and downstream agents must not revisit them without explicit user discussion:

- **D-08:** SQLite driver: `modernc.org/sqlite` (pure Go, CGO disabled). NOT `mattn/go-sqlite3`.
- **D-09:** golang-migrate import: `github.com/golang-migrate/migrate/v4/database/sqlite` (NOT `sqlite3`). The suffix difference is the entire CGO impact.
- **D-10:** Crypto storage: AES-256-GCM with 12-byte `crypto/rand` nonce. Stored as single BLOB column: `nonce (12 bytes) || ciphertext`. Never split into separate columns (schema is immutable after data is written).
- **D-11:** SQLite WAL mode + dual `*sql.DB` pools: write pool `MaxOpenConns(1)`, read pool `MaxOpenConns(4)`. Both opened before migrations run.
- **D-12:** Context key: unexported struct type in the `auth` package. Prevents cross-package key collision.
- **D-13:** Docker: `golang:1.26-alpine` builder → `FROM scratch` final. `CGO_ENABLED=0`, `-ldflags="-w -s"`, `ca-certificates.crt` copied from builder. Final image target: under 25 MB.
- **D-14:** OAuth CSRF state: stored in SQLite `pending_auth` table. Consumed via `DELETE ... RETURNING` (atomic single-use). 10-minute TTL.
- **D-15:** golangci-lint v2.11, mirroring karaclean config exactly (standard + gocyclo/godot/misspell/noctx, errcheck type assertions).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project Requirements

- `.planning/REQUIREMENTS.md` — Full v1 requirements (FOUND-01..10, SERV-01..08 are Phase 1 scope); acceptance criteria for each requirement
- `.planning/PROJECT.md` — Core value, constraints, key decisions table, tech stack, out-of-scope list
- `.planning/ROADMAP.md` §Phase 1 — 5 plans with detailed task breakdown and success criteria

### Research

- `.planning/research/SUMMARY.md` — Architecture, stack, pitfalls (top 5 ranked by severity), phase implications; HIGH confidence overall
- `.planning/research/STACK.md` — Dependency analysis (4 external deps only; what NOT to use)
- `.planning/research/ARCHITECTURE.md` — Package dependency graph, HTTP mux layout, key open questions
- `.planning/research/PITFALLS.md` — Security pitfalls with severity ranking

### External Reference (karaclean)

- `github.com/lm/karaclean` — Reference implementation for: Go 1.26 module layout (`cmd/<name>/main.go` + `internal/`), stdlib `log/slog`, Makefile, golangci-lint config, multi-stage Dockerfile, CI pipeline (`test` + `lint` + `docker` jobs, ghcr.io, `latest` + SHA tags). Mirror conventions exactly.

### External APIs

- Polar AccessLink OAuth2: `https://flow.polar.com/oauth2/authorization` (auth), `https://polarremote.com/v2/oauth2/token` (token exchange)
- mcp-go SDK: `github.com/mark3labs/mcp-go` — `NewStreamableHTTPServer`, `WithHTTPContextFunc`, `WithStateful(true)`, tool handler signature

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- None — greenfield project. Phase 1 creates the foundational assets that later phases reuse.

### Established Patterns (to establish in Phase 1)

- **Package dependency graph** (strict, unidirectional): `config → crypto → store → polar/auth → oauth → mcp → main`. No cycles; no `store` importing `mcp`, no `auth` importing `store`.
- **Context key pattern**: unexported struct type in `internal/auth` package. All tool handlers receive identity via `ctx.Value(authKeyType{})`.
- **Fail-closed pattern**: `server.go` `main()` calls config validation before any listener is started. Any validation failure: `slog.Error(msg)` + `os.Exit(1)`.
- **SQLite DSN**: WAL mode, busy_timeout, foreign keys all in the DSN string — not set post-open.

### Integration Points

- Auth middleware wraps ALL routes except `/healthz` and `/readyz`. It runs in two layers: (1) shared-secret check, (2) identity header extraction.
- `StreamableHTTPServer` is mounted as an `http.Handler` at `/mcp` inside the stdlib `ServeMux` — NOT started via its own `Start()` loop.

</code_context>

<specifics>
## Specific Ideas

- The startup security banner (FOUND-10) must name: the trusted identity header (`Remote-User` or configured value), the required secret header name, and the declared `AUTH_PROXY` value. Format: structured `slog.Info` fields, not a raw string.
- `docker-compose.yml` example ships with `AUTH_PROXY=unconfigured` so a naive `docker compose up` fails loudly — this is a deliberate UX contract for operators.
- The `KeyProvider` interface decouples key source from crypto operations. Phase 1 ships `env` (reads `ENCRYPTION_KEY` base64) and `file` (reads path from `ENCRYPTION_KEY_FILE`) implementations. Interface must be in `internal/crypto`, not `internal/config`.

</specifics>

<deferred>
## Deferred Ideas

- **KeyProvider key caching behavior** — Not discussed; planner can decide whether the env/file provider caches the key on first read or re-reads on every call. Either is correct for v1 given tokens don't expire and single-key-version scope. `file` impl MAY cache since key rotation is a v2 concern.
- **Startup error message format** — Not discussed; planner can decide between plain stderr + `os.Exit(1)` and structured `slog.Error` + `os.Exit(1)`. Either satisfies FOUND-02..04's "actionable error message" requirement. Suggest `slog.Error` for consistency with the startup banner.

None of the above require revisiting with the user — planner discretion is sufficient.

</deferred>

---

*Phase: 1-Project Foundation and Security Skeleton*
*Context gathered: 2026-05-04*
