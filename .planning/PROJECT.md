# polar-flow-mcp

## What This Is

A multi-user MCP (Model Context Protocol) server written in Go that wraps the Polar AccessLink API, enabling users to create and manage Polar Flow training targets directly from Claude conversations. Deployed behind a user-provided reverse-proxy authentication layer (Authelia, Authentik, oauth2-proxy, etc.) and designed for self-hosting in homelab and small-team environments. Released as FOSS under MIT, hosted on GitHub with documentation on GitHub Pages.

## Core Value

A user can say "create a 5×1km threshold session for Thursday" in Claude and have it appear in Polar Flow — zero context-switching, zero manual UI navigation.

## Requirements

### Validated

- [x] Multi-user SQLite store with schema migrations — Validated in Phase 1: WAL dual-pool, 3-table schema, golang-migrate embedded migrations
- [x] Polar access tokens encrypted at rest (AES-256-GCM, KeyProvider interface) — Validated in Phase 1: nonce-prepend BLOB layout, EnvKeyProvider + FileKeyProvider
- [x] Fail-closed auth design (refuse to start unless AUTH_PROXY ≠ "unconfigured") — Validated in Phase 1: config.Load() exits 1 on missing/unconfigured AUTH_PROXY
- [x] Proxy shared secret enforcement (PROXY_SHARED_SECRET + configurable header) — Validated in Phase 1: subtle.ConstantTimeCompare before identity header read
- [x] /healthz and /readyz endpoints — Validated in Phase 1: wired in main.go with httptest integration tests
- [x] CI pipeline mirroring karaclean exactly (test + lint + docker jobs, ghcr.io) — Validated in Phase 1: three-job workflow, ghcr.io push with latest + sha tags

### Active

- [ ] Multi-user SQLite store with schema migrations
- [ ] Per-user Polar OAuth2 flow (login + callback endpoints)
- [ ] Polar access tokens encrypted at rest (AES-256-GCM, KeyProvider interface)
- [ ] MCP tool: `create_training_target` (warmup/repeats/cooldown, HR zone/pace/power intensity)
- [ ] MCP tool: `list_training_targets` (upcoming scheduled targets)
- [ ] MCP tool: `delete_training_target`
- [ ] MCP tool: `get_user_info` (which Polar account is linked)
- [ ] Fail-closed auth design (refuse to start unless AUTH_PROXY ≠ "unconfigured")
- [ ] Proxy shared secret enforcement (PROXY_SHARED_SECRET + configurable header)
- [ ] Bind to 127.0.0.1 by default; BIND_ADDRESS for anything else
- [ ] /healthz and /readyz endpoints
- [ ] MkDocs Material documentation site (Diátaxis structure, GitHub Pages)
- [ ] Bundled Claude skill at skill/polar-coach/SKILL.md
- [ ] Full FOSS hygiene (LICENSE, README, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, .github/)
- [ ] CI pipeline mirroring karaclean exactly (test + lint + docker jobs, ghcr.io)

### Out of Scope

- Built-in user authentication — proxy contract is the only auth boundary; no login forms, JWT, sessions
- Polar data ingestion / analytics — read-only historical data endpoints; this project only writes training targets
- Mobile app or web UI — Claude conversation is the only UX surface
- Real-time push from Polar (webhooks) — polling or on-demand only
- Multi-key rotation in v1 — key_version column present for future rotation, but v1 only handles version 1
- Vault / cloud KMS KeyProvider implementations — interface designed for it, not built in v1

## Context

**Reference project:** `karaclean` (`github.com/lm/karaclean`) — Go 1.26, `cmd/<name>/main.go` + `internal/` packages, stdlib `log`, multi-stage Dockerfile (`golang:1.26-alpine` → `scratch`), golangci-lint v2.11 (standard + gocyclo/godot/misspell/noctx), CI: `test` + `lint` → `docker` (ghcr.io, `latest` + SHA tags). This project must mirror those conventions exactly.

**Polar AccessLink API:**
- OAuth2: auth at `https://flow.polar.com/oauth2/authorization`, token at `https://polarremote.com/v2/oauth2/token`
- Scope: `accesslink.read_all`; tokens do not expire unless revoked
- User registration required after OAuth: `POST /v3/users` with the bearer token
- Training targets: `POST /v3/users/{user-id}/training-targets`, `GET /v3/users/{user-id}/training-targets`, `DELETE /v3/users/{user-id}/training-targets/{target-id}`
- Phase structure: warmup → repeats (with recovery) → cooldown; intensity by HR zone (Z1–Z5), pace, or power

**MCP-go SDK (`github.com/mark3labs/mcp-go`):**
- `server.NewStreamableHTTPServer` with `WithHTTPContextFunc` injects proxy identity header into tool call context
- Single `/mcp` endpoint; session tracking via `WithStateful(true)`
- Tool handler signature: `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`

**SQLite driver:** `modernc.org/sqlite` (pure Go, CGO disabled)

**Schema migrations:** `golang-migrate` (or equivalent — decision pending research)

**Documentation:** MkDocs Material deployed to GitHub Pages via Actions; Diátaxis structure (tutorial / how-to / reference / explanation)

## Constraints

- **Language:** Go 1.26 — mirroring karaclean exactly
- **CGO:** Disabled — `modernc.org/sqlite` for pure-Go SQLite
- **Image size:** Final Docker image under 25 MB (target 15–20 MB); `FROM scratch` or `distroless-static`
- **Auth model:** No built-in auth — proxy contract only; `AUTH_PROXY` env var must be set to non-`unconfigured` value or server refuses to start
- **Encryption key:** Must be 32 bytes (AES-256); server refuses to start without it
- **CI:** Must match karaclean's pipeline exactly — same stages, tools, registry, tagging scheme
- **License:** MIT
- **Deployment target:** Self-hosted (homelab / small team); `BIND_ADDRESS` defaults to `127.0.0.1`

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| MCP transport: StreamableHTTP | `WithHTTPContextFunc` lets us inject proxy identity header into every tool call context naturally | Confirmed Phase 1 — auth middleware injects identity via `context.WithValue`; `WithHTTPContextFunc` copies it to mcp-go context |
| Auth model: proxy contract only | No built-in auth means no auth bugs; fail-closed design prevents silent misconfiguration | Confirmed Phase 1 — `subtle.ConstantTimeCompare` before identity header; server refuses start without AUTH_PROXY |
| Token encryption: AES-256-GCM per-row nonce | Standard at-rest protection; KeyProvider interface keeps key source swappable | Confirmed Phase 1 — nonce-prepend BLOB layout, EnvKeyProvider + FileKeyProvider, 1000-nonce uniqueness test |
| SQLite + modernc.org/sqlite | Pure Go, CGO disabled, zero deployment dependencies, appropriate for homelab n=1..small-team scale | Confirmed Phase 1 — WAL dual-pool (write MaxOpenConns=1, read MaxOpenConns=4), embedded migrations |
| `FROM scratch` final image | Matches karaclean; minimizes attack surface and image size | Confirmed Phase 1 — 11.46 MB final image, ca-certificates.crt copied for Polar API HTTPS |
| golang-migrate for schema migrations | Pure-Go sqlite driver suffix `sqlite` (not `sqlite3`) required for CGO-disabled build | Confirmed Phase 1 — `golang-migrate/migrate/v4/database/sqlite` wired and tested |
| MkDocs Material for docs | GitHub Pages deployment, Diátaxis structure, good for FOSS projects | — Pending Phase 4 |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-05-05 — Phase 1 complete (foundation + security skeleton)*
