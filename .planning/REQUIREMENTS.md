# Requirements: polar-flow-mcp

**Defined:** 2026-05-03
**Core Value:** A user can say "create a 5×1km threshold session for Thursday" in Claude and have it appear in Polar Flow — zero context-switching, zero manual UI navigation.

---

## v1 Requirements

### Foundation

- [x] **FOUND-01**: Developer can build the project with `CGO_ENABLED=0 go build ./...` producing a single static binary
- [x] **FOUND-02**: Server refuses to start if `AUTH_PROXY` env var equals `"unconfigured"` (the default), printing an actionable error message and pointing at the docs
- [x] **FOUND-03**: Server refuses to start if `PROXY_SHARED_SECRET` is empty or unset, printing an actionable error message
- [x] **FOUND-04**: Server refuses to start if no 32-byte encryption key is loadable (via `KEY_PROVIDER=env` from `ENCRYPTION_KEY` or `KEY_PROVIDER=file` from `ENCRYPTION_KEY_FILE`), printing how to generate one with `openssl rand -base64 32`
- [ ] **FOUND-05**: SQLite database opens with WAL mode, `busy_timeout=5000`, `foreign_keys=ON`, and dual read/write connection pools
- [ ] **FOUND-06**: Schema migrations run automatically at startup before the server accepts connections, using embedded SQL files and the pure-Go `golang-migrate` sqlite driver
- [ ] **FOUND-07**: Users table stores `identity` (proxy header value), `polar_user_id`, and timestamps; polar_tokens table stores `encrypted_token BLOB` (nonce||ciphertext), `key_version`, and `updated_at`; pending_auth table stores OAuth CSRF state with expiry
- [ ] **FOUND-08**: Polar access tokens are encrypted with AES-256-GCM (unique `crypto/rand` nonce per encryption) before being written to SQLite; decrypted on demand, never cached
- [ ] **FOUND-09**: `KeyProvider` interface has two implementations: `env` (reads base64 key from `ENCRYPTION_KEY`) and `file` (reads key from path in `ENCRYPTION_KEY_FILE`)
- [x] **FOUND-10**: Startup log clearly states trust assumptions: which identity header is trusted, which secret header is required, and the declared auth proxy name

### Server

- [ ] **SERV-01**: `GET /healthz` returns 200 always (no auth, no config check) for liveness probes
- [ ] **SERV-02**: `GET /readyz` returns 200 only if: `AUTH_PROXY` ≠ `"unconfigured"`, `PROXY_SHARED_SECRET` is set, DB is reachable, encryption key loaded; otherwise 503 with a body naming each failing check
- [ ] **SERV-03**: All routes except `/healthz` and `/readyz` require the `PROXY_SHARED_SECRET` header to match the configured secret; mismatch returns 403; header present but wrong value is logged as a probable spoofing attempt; comparison uses `subtle.ConstantTimeCompare`
- [ ] **SERV-04**: MCP server uses StreamableHTTP transport (`github.com/mark3labs/mcp-go`) mounted at `/mcp`; user identity is extracted from the configured identity header (default `Remote-User`) and injected into every tool call's `context.Context`
- [ ] **SERV-05**: `BIND_ADDRESS` defaults to `127.0.0.1`; a startup WARN log is emitted if `BIND_ADDRESS` is not a loopback address, reminding the operator that `PROXY_SHARED_SECRET` is the security boundary
- [ ] **SERV-06**: Multi-stage Dockerfile (`golang:1.26-alpine` builder → `FROM scratch` final) with `CGO_ENABLED=0`, `-ldflags="-w -s"`, and `ca-certificates.crt` copied from builder; final image under 25 MB
- [ ] **SERV-07**: CI pipeline has three jobs mirroring karaclean exactly: `test` (`go test -race -count=1 ./...` with `CGO_ENABLED=0`), `lint` (`golangci-lint-action@v9`, `version: v2.11`), `docker` (needs both; builds and pushes to `ghcr.io/${{ github.repository }}` with `latest` + SHA tags)
- [ ] **SERV-08**: `docker-compose.yml` example ships with `AUTH_PROXY=unconfigured` so a naive `docker compose up` fails loudly with the startup error message

### OAuth and User Linking

- [ ] **OAUTH-01**: `GET /oauth/login` reads the user identity from the proxy identity header, generates a 32-byte `crypto/rand` state token bound to that identity, stores it in the `pending_auth` SQLite table with a 10-minute expiry, and redirects to Polar's authorization URL with `scope=accesslink.read_all`
- [ ] **OAUTH-02**: `GET /oauth/callback` validates the state parameter (present, found in DB, not expired, bound identity matches current request's identity header), then deletes the state row (single-use)
- [ ] **OAUTH-03**: After state validation, callback exchanges the authorization code for a Polar access token via `POST https://polarremote.com/v2/oauth2/token` (HTTP Basic auth with client credentials)
- [ ] **OAUTH-04**: Callback registers the user with Polar via `POST https://www.polaraccesslink.com/v3/users`; HTTP 409 is treated as idempotent success (already registered)
- [ ] **OAUTH-05**: Callback encrypts the Polar access token and upserts it in the `polar_tokens` table with `key_version=1`; responds 200 with a human-readable "Polar account linked" message

### MCP Tools

- [ ] **MCP-01**: `get_user_info` tool returns which Polar account is linked for the calling user (identity from proxy header, polar_user_id from store); returns a clear message if no account is linked
- [ ] **MCP-02**: `create_training_target` tool accepts: session name, sport (default `RUNNING`), scheduled date (ISO 8601), scheduled time (default `18:00`), and a phases array; each phase specifies type (warmup/repeat/cooldown), repeat count, duration or distance goal, and intensity by HR zone number (1–5) or intensity label (`easy`/`aerobic`/`tempo`/`threshold`/`vo2max`); repeat phases may include a recovery sub-phase
- [ ] **MCP-03**: `create_training_target` maps `intensity_label` values to Polar HR zones: easy→Z1-2, aerobic→Z2, tempo→Z3, threshold→Z4, vo2max→Z5; constructs the Polar API JSON and calls `POST /v3/users/{polar-user-id}/training-targets`
- [ ] **MCP-04**: `list_training_targets` tool accepts optional `from_date` and `to_date` parameters (defaults: today to +30 days); calls `GET /v3/users/{polar-user-id}/training-targets` and returns a human-readable list
- [ ] **MCP-05**: `delete_training_target` tool accepts `target_id`; calls `DELETE /v3/users/{polar-user-id}/training-targets/{id}`; returns confirmation or a clear error if not found
- [ ] **MCP-06**: All MCP tool handlers extract user identity from context, look up the Polar token from store (decrypt on demand), and return a clear error message (not a server panic) if the user has no linked account
- [ ] **MCP-07**: All MCP tool handler code uses an unexported struct type as the context key for user identity (prevents collision with any other package's context keys)

### Documentation

- [ ] **DOC-01**: `docs/` tree follows Diátaxis structure as specified: `index.md`, `getting-started.md`, `deployment/` (docker-compose, auth-proxies, polar-oauth-setup), `usage.md`, `reference/` (env vars, MCP tool schemas, HTTP endpoints, DB schema), `security.md`, `contributing.md`
- [ ] **DOC-02**: `docs/security.md` documents: threat model, what encryption-at-rest does and does not protect against, why the server refuses to start by default, what happens if the proxy is misconfigured, the header contract (identity header + shared secret header), and the key rotation plan
- [ ] **DOC-03**: `docs/deployment/auth-proxies.md` documents the header contract for Authelia, Authentik, oauth2-proxy, Pomerium, Cloudflare Access, and makes clear any auth proxy that injects both headers works
- [ ] **DOC-04**: Documentation uses `![placeholder](images/<descriptive-name>.png)` at logical screenshot points (Polar app registration, auth proxy login flow, Claude using tools)
- [ ] **DOC-05**: MkDocs Material site deploys to GitHub Pages via a `.github/workflows/docs.yml` workflow triggered on pushes to `main`

### Bundled Skill

- [ ] **SKILL-01**: `skill/polar-coach/SKILL.md` includes: trigger description for Polar/training/workout/interval/running intents, all 4 MCP tool names with purpose descriptions, Polar HR zone table (Z1–Z5) with training language mapping, warmup/cooldown defaults, Polar's 18:00 default scheduled time
- [ ] **SKILL-02**: Skill includes worked examples: "5×1km threshold with 2min recovery" → tool call shape; "12-week marathon plan" → questions to ask first, then iterative session creation
- [ ] **SKILL-03**: Skill includes guidance on when NOT to call tools (analyzing past sessions vs creating new ones) and safe degradation: check MCP tool availability before use; if server not connected, tell user and point at deployment docs
- [ ] **SKILL-04**: Skill documents both installation paths: Claude Desktop/Code skills directory and Claude.ai project upload

### FOSS Hygiene

- [ ] **FOSS-01**: `LICENSE` (MIT), `README.md` (badges: CI, license, latest release, image size, Go report card; screenshot placeholder; 5-line quickstart; links to docs), `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant), `SECURITY.md` (vulnerability reporting process)
- [ ] **FOSS-02**: `.github/` directory contains: `workflows/ci.yml`, `workflows/release.yml` (triggered on `v*` tags; builds multi-arch image, creates GitHub Release), `workflows/docs.yml`, `ISSUE_TEMPLATE/` (bug report, feature request), `PULL_REQUEST_TEMPLATE.md`
- [ ] **FOSS-03**: Release workflow uses conventional commits + `git-cliff` to generate `CHANGELOG.md` entries automatically on tag push; Docker image tagged with semver tag and `latest`

---

## v2 Requirements

### Intensity

- **INT-01**: `create_training_target` supports pace-based intensity (`min/km` or `min/mile`) for running targets
- **INT-02**: `create_training_target` supports power-based intensity (watts) for cycling targets

### Sports

- **SPORT-01**: `sport` parameter supports additional Polar sport types beyond `RUNNING` (cycling, swimming, strength, other)

### Operations

- **OPS-01**: Prometheus metrics endpoint (`/metrics`) for homelab observability
- **OPS-02**: Key rotation: support for decrypting tokens encrypted with `key_version=1` and re-encrypting with `key_version=2`; migration tooling

### Convenience

- **CONV-01**: `list_training_targets` can return saved favorite targets from Polar
- **CONV-02**: Token re-link flow UX (currently tokens do not expire, so re-link is manual)

---

## Out of Scope

| Feature | Reason |
|---------|--------|
| Built-in user authentication (login forms, JWT, sessions) | Proxy contract is the only auth boundary; no auth bugs if no auth code |
| Reading past training sessions / analytics | Project writes targets; reading historical data is a different API surface and use case |
| Polar account creation | Users must have an existing Polar account |
| Webhooks / real-time push from Polar | No server-push support in AccessLink; polling not in scope |
| Web UI for managing targets | Claude conversation is the UX surface |
| Vault / cloud KMS KeyProvider | Interface supports it; not built in v1 |
| Multi-key rotation in v1 | `key_version` column present; implementation deferred to v2 |
| Mobile app | Not applicable |

---

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| FOUND-01 | Phase 1 | Pending |
| FOUND-02 | Phase 1 | Done (01-02) |
| FOUND-03 | Phase 1 | Done (01-02) |
| FOUND-04 | Phase 1 | Done (01-02) |
| FOUND-05 | Phase 1 | Pending |
| FOUND-06 | Phase 1 | Pending |
| FOUND-07 | Phase 1 | Pending |
| FOUND-08 | Phase 1 | Pending |
| FOUND-09 | Phase 1 | Pending |
| FOUND-10 | Phase 1 | Done (01-02) |
| SERV-01 | Phase 1 | Pending |
| SERV-02 | Phase 1 | Pending |
| SERV-03 | Phase 1 | Pending |
| SERV-04 | Phase 1 | Pending |
| SERV-05 | Phase 1 | Pending |
| SERV-06 | Phase 1 | Pending |
| SERV-07 | Phase 1 | Pending |
| SERV-08 | Phase 1 | Pending |
| OAUTH-01 | Phase 2 | Pending |
| OAUTH-02 | Phase 2 | Pending |
| OAUTH-03 | Phase 2 | Pending |
| OAUTH-04 | Phase 2 | Pending |
| OAUTH-05 | Phase 2 | Pending |
| MCP-01 | Phase 2 | Pending |
| MCP-02 | Phase 3 | Pending |
| MCP-03 | Phase 3 | Pending |
| MCP-04 | Phase 3 | Pending |
| MCP-05 | Phase 3 | Pending |
| MCP-06 | Phase 3 | Pending |
| MCP-07 | Phase 3 | Pending |
| SKILL-01 | Phase 3 | Pending |
| SKILL-02 | Phase 3 | Pending |
| SKILL-03 | Phase 3 | Pending |
| SKILL-04 | Phase 3 | Pending |
| DOC-01 | Phase 4 | Pending |
| DOC-02 | Phase 4 | Pending |
| DOC-03 | Phase 4 | Pending |
| DOC-04 | Phase 4 | Pending |
| DOC-05 | Phase 4 | Pending |
| FOSS-01 | Phase 4 | Pending |
| FOSS-02 | Phase 4 | Pending |
| FOSS-03 | Phase 4 | Pending |

**Coverage:**
- v1 requirements: 43 total (40 functional + SKILL-01..04 resolved to Phase 3)
- Mapped to phases: 43
- Unmapped: 0 ✓

---
*Requirements defined: 2026-05-03*
*Last updated: 2026-05-03 after roadmap creation*
