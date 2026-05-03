# Project Research Summary

**Project:** polar-flow-mcp
**Domain:** Multi-user MCP server (Go) wrapping a third-party OAuth2 REST API
**Researched:** 2026-05-03
**Confidence:** HIGH

## Executive Summary

polar-flow-mcp is a networked Go MCP server that proxies Polar AccessLink API calls, allowing Claude to create, list, and delete training targets in Polar Flow on behalf of multiple users. The established pattern for this class of project is: stdlib-first Go (ServeMux, slog, crypto/aes, net/http), a single mature MCP SDK (mcp-go with StreamableHTTP transport), pure-Go SQLite (modernc.org/sqlite) for multi-user token storage, and a reverse-proxy auth contract that injects user identity via a trusted header. Exactly 4 external dependencies are needed — everything else is stdlib. The karaclean project is the direct reference implementation for Go conventions, Docker image layout, CI pipeline, and release infrastructure.

The critical design invariant is the security boundary: the reverse proxy injects `X-Remote-User`; a `PROXY_SHARED_SECRET` header prevents that header from being spoofed by direct requests. This boundary is fail-closed — the server refuses to start without a configured secret, and the secret is checked before any user-identity header is read. Per-user Polar bearer tokens are stored encrypted (AES-256-GCM, nonce prepended to ciphertext as a single BLOB) so an attacker who exfiltrates the database cannot use the tokens without the encryption key.

The main risks are all Phase 1 decisions that cannot be safely retrofitted: middleware ordering (secret check must be first), nonce/ciphertext storage layout (blob schema is immutable after data is written), CGO-free driver selection (`sqlite` not `sqlite3` in golang-migrate), and SQLite WAL + dual-pool concurrency setup. Get these right in Phase 1 and the rest of the build follows a well-documented, low-surprise path.

---

## Key Findings

### Stack

**4 external dependencies. Everything else is stdlib.**

- `github.com/mark3labs/mcp-go` — MCP server SDK; `NewStreamableHTTPServer` + `WithHTTPContextFunc` is the correct HTTP transport for multi-user identity injection. NOT SSE (deprecated), NOT stdio.
- `modernc.org/sqlite` — pure-Go SQLite, required by `CGO_ENABLED=0` constraint. The CGO variant (`mattn/go-sqlite3`) is a hard blocker for `FROM scratch`.
- `github.com/golang-migrate/migrate/v4` (sqlite driver, NOT sqlite3) — embedded SQL migrations at startup. The import path suffix is the only difference; the CGO impact is total.
- `golang.org/x/oauth2` — authorization code exchange with Polar; handles HTTP Basic auth (`AuthStyleInHeader`) for the token endpoint.
- `log/slog` (stdlib) — structured per-request logging; no external library needed.

**Do not use:** chi/gorilla/echo (6 routes, stdlib ServeMux sufficient in Go 1.22+), zerolog/zap/logrus, testify, oapi-codegen for Polar client (4 endpoints, hand-rolled is smaller), `golang-migrate/.../sqlite3` (CGO).

### Features

The Polar API surface for v1 is narrow: register user, create/list/delete training targets. The MCP tool interface accepts coaching language ("5×1km threshold") and maps it to Polar's `HEART_RATE_ZONE` intensity type internally.

**Must have (table stakes):**
- `create_training_target` — core value; warmup/repeat/cooldown phases with HR zone intensity (Z1–Z5), `intensity_label` → zone mapping built in
- `list_training_targets` — date-filtered (default: today → +30 days)
- `delete_training_target` — correct Claude's mistakes
- `get_user_info` — verify linked account; simplest tool, best first integration test
- Per-user OAuth link flow (`/oauth/login` + `/oauth/callback`) — prerequisite for all tools

**Polar HR zone mapping (used in bundled skill and tool handler):**
- Z1: recovery / easy / warm-up
- Z2: aerobic / base / long slow distance
- Z3: tempo / marathon pace
- Z4: threshold / lactate threshold
- Z5: VO2max / hard intervals / race effort

**Defer to v2+:** Pace and power intensity, multi-sport beyond RUNNING, Prometheus metrics, key rotation, favorite targets.

### Architecture

Layered Go module with strict package boundaries. Dependency graph:

```
config → crypto → store → polar / auth → oauth → mcp → main
```

The `polar` package has zero internal dependencies (takes bearer token as plain argument). Token decryption is encapsulated inside `store.GetToken()` so `mcp` tool handlers have no knowledge of encryption mechanics. OAuth state lives in SQLite (not in-memory map) for restart safety and atomic consume-on-first-use.

**HTTP mux layout:**
- `GET /healthz`, `GET /readyz` — no auth middleware
- `POST /mcp` — auth middleware → `StreamableHTTPServer`
- `GET /oauth/login`, `GET /oauth/callback` — auth middleware → handlers

**Key open question:** Verify whether `WithHTTPContextFunc` fires per-request or only at session establishment when `StreamableHTTPServer` is mounted as `http.Handler` (not started via `Start()`). If per-session only: extract identity in the auth middleware via `context.WithValue` instead. This is the safer fallback.

### Watch Out For (top 5, ranked by severity)

1. **Proxy header spoofing** — `subtle.ConstantTimeCompare` on `PROXY_SHARED_SECRET` is the FIRST middleware operation, before reading any identity header. Server refuses to start without a non-empty secret. Phase 1.

2. **AES-GCM nonce reuse** — `crypto/rand` only; store `nonce (12 bytes) || ciphertext` as one BLOB column, never separate columns. Test: 1000 encryptions produce 1000 distinct nonces. Phase 1 (schema is immutable after data is written).

3. **OAuth CSRF state** — 32-byte `crypto/rand` state in SQLite; consume-on-first-use via `DELETE ... RETURNING`; 10-minute TTL; 409 from Polar user registration is idempotent success. Phase 2.

4. **MCP context leakage** — unexported struct context key; every tool handler checks identity before proceeding; all SQL parameterized with `user_id`; `go test -race` with concurrent users. Phase 1 + Phase 3.

5. **SQLite SQLITE_BUSY errors** — WAL mode + `busy_timeout=5000` in DSN; write pool `MaxOpenConns(1)`; read pool `MaxOpenConns(4)`; set before migrations run. Phase 1.

---

## Phase Implications

### Suggested Phase Structure (4 phases)

**Phase 1 — Project Foundation and Security Skeleton**
All irreversible decisions must land here. Security middleware ordering, crypto schema, and SQLite initialization cannot be safely retrofitted.

Delivers: Compilable server with health endpoints, auth middleware (fail-closed), SQLite with WAL/dual-pool + 3-table schema, AES-256-GCM crypto + KeyProvider interface, Docker image mirroring karaclean, CI pipeline (test + lint + docker)

Critical: `sqlite` driver (not `sqlite3`), secret check before identity read, dual `*sql.DB` pools with WAL, `ca-certificates.crt` in scratch image

**Phase 2 — OAuth Link Flow**
All MCP tools require a linked Polar account. OAuth exercises every foundational layer end-to-end and validates the integration before tool handlers exist. `get_user_info` is included here as the simplest tool.

Delivers: `/oauth/login` → Polar authorization → `/oauth/callback` → encrypted token stored; `get_user_info` MCP tool

Critical: SQLite state storage (not in-memory), 409 treated as success, state deleted on use, TTL ≤ 10 minutes

**Phase 3 — Core MCP Tools**
With OAuth working, all remaining tools are leaf nodes sharing the identity-from-context pattern from Phase 1.

Delivers: `create_training_target` (all phases, HR zone + coaching-language mapping), `list_training_targets`, `delete_training_target`; bundled `skill/polar-coach/SKILL.md`

Critical: No mutable struct-level handler state, all SQL scoped by `user_id`, `WithStateful(true)` session identity behavior verified

**Phase 4 — Documentation and FOSS Release**
Docs and release polish once core functionality is proven.

Delivers: MkDocs Material site (Diátaxis structure), GitHub Pages workflow, release workflow (git-cliff + ghcr.io), FOSS hygiene files (LICENSE, README, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, .github/)

### Phase Ordering Rationale

- Phase 1 must be first: security and crypto schema are irreversible
- Phase 2 before Phase 3: OAuth is a hard prerequisite; it also serves as the integration smoke test for all foundational packages
- Phase 3 is one phase: all tools share the same pattern; splitting them creates artificial seams
- Phase 4 last: no value in polishing docs before core functionality is validated

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All choices verified via Context7 (mcp-go), pkg.go.dev (golang-migrate sqlite), karaclean reference |
| Features | MEDIUM-HIGH | Polar API shapes from public docs; MCP tool schema is a recommendation, not live-verified |
| Architecture | HIGH | mcp-go `WithHTTPContextFunc`/`WithStateful` verified via Context7; SQLite WAL dual-pool from multiple sources |
| Pitfalls | HIGH | CVEs cited for proxy spoofing; AES-GCM analysis from cryptographic sources; oauth2-proxy race from issue tracker |

**Overall confidence:** HIGH

### Gaps to Address During Planning

1. **`WithHTTPContextFunc` per-request vs per-session when mounted as handler:** Resolve in Phase 1 planning. Fallback documented and safe.
2. **Polar API live validation:** Validate training target JSON shapes during Phase 2 before Phase 3 builds on them.
3. **`WithStateful(true)` session identity binding:** Verify in Phase 3 via integration test whether `WithHTTPContextFunc` fires per-request on stateful sessions.

---

*Research completed: 2026-05-03*
*Ready for requirements and roadmap: yes*
