# Stack Recommendations: polar-flow-mcp

**Domain:** Multi-user MCP server in Go wrapping Polar AccessLink API
**Researched:** 2026-05-03
**Overall confidence:** HIGH — decisions grounded in karaclean conventions, mcp-go docs, and golang-migrate driver compatibility.

---

## Go Version

**Recommendation:** Go 1.26 (mirror karaclean exactly)
**Confidence:** HIGH

`go.mod` declares `go 1.26.1`. Use the same. The `go-version-file: go.mod` directive in CI (`actions/setup-go@v6`) means both projects automatically stay in sync when the toolchain is bumped.

---

## Module Name

**Recommendation:** `github.com/lm/polar-flow-mcp`
**Why:** Mirrors karaclean's `github.com/lm/karaclean` convention. Update to actual GitHub org/repo at time of publishing.

---

## MCP SDK

**Recommendation:** `github.com/mark3labs/mcp-go` (latest)
**Confidence:** HIGH

- The only mature, actively maintained Go MCP SDK as of 2026.
- `server.NewStreamableHTTPServer` is the correct transport for an HTTP-accessible server.
- `WithHTTPContextFunc` enables per-request header injection into tool handler context — the exact mechanism this project needs for multi-user identity.
- `WithStateful(true)` for session tracking across a conversation.
- **NOT SSE transport** — SSE (`server.NewSSEServer`) is deprecated in favor of StreamableHTTP per the MCP spec evolution.
- **NOT stdio** — stdio is for CLI-embedded tools; this is a networked server.

---

## HTTP Router

**Recommendation:** `net/http` stdlib `ServeMux` (Go 1.22+)
**Confidence:** HIGH
**Do NOT use:** chi, gorilla/mux, echo, fiber

Go 1.22's enhanced `ServeMux` supports method-qualified patterns (`GET /healthz`) and path parameters. This project has ~6 routes with no path-parameter complexity beyond a single optional segment in the OAuth callback. `chi` adds value at 20+ routes with complex middleware trees; it is not worth the dependency here. The `mcp-go` `StreamableHTTPServer` is mounted as an `http.Handler` — it integrates with stdlib mux cleanly.

---

## SQLite Driver

**Recommendation:** `modernc.org/sqlite` (pure Go)
**Confidence:** HIGH
**Do NOT use:** `mattn/go-sqlite3` (requires CGO)

`CGO_ENABLED=0` is a hard constraint (mirrors karaclean, required for `FROM scratch`). `modernc.org/sqlite` is a transpiled pure-Go SQLite — no CGO, no C compiler, no shared libraries. Fully compatible with `database/sql`. BLOB columns scan as `[]byte` idiomatically. WAL mode and all pragmas work via DSN parameters.

DSN format: `file:polar.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)`

---

## Schema Migrations

**Recommendation:** `github.com/golang-migrate/migrate/v4`
**Import:** `github.com/golang-migrate/migrate/v4/database/sqlite` (pure Go — NOT `sqlite3`)
**Confidence:** HIGH

Critical: use the `sqlite` package (modernc.org/sqlite backend), NOT `sqlite3` (mattn/go-sqlite3 backend). The import path suffix is the only difference; the CGO impact is total.

Migration files embedded via `//go:embed migrations/*.sql` and loaded with `iofs.New(migrationsFS, "migrations")`. Migrations run at startup before the HTTP server begins accepting connections. Each migration file should contain a single logical change. Do NOT include `BEGIN`/`COMMIT` in migration files — the driver wraps each file in an implicit transaction.

**Alternative considered:** `github.com/pressly/goose/v3` — also supports modernc.org/sqlite, but golang-migrate has broader community adoption and cleaner embed integration. Either is viable; golang-migrate is recommended for consistency with the ecosystem.

---

## OAuth2 Client

**Recommendation:** `golang.org/x/oauth2`
**Confidence:** HIGH
**Do NOT use:** hand-rolled token exchange

`golang.org/x/oauth2` handles the authorization code exchange cleanly: `oauth2.Config{ClientID, ClientSecret, Endpoint, RedirectURL, Scopes}` + `conf.Exchange(ctx, code)`. The Polar endpoint values:
- `AuthURL`: `https://flow.polar.com/oauth2/authorization`
- `TokenURL`: `https://polarremote.com/v2/oauth2/token`

Token exchange requires HTTP Basic auth (client_id:client_secret). `golang.org/x/oauth2` handles this when the endpoint has `AuthStyle: oauth2.AuthStyleInHeader`.

Polar tokens do not expire, so token refresh is not needed in v1. The `oauth2.Token.AccessToken` string is what gets encrypted and stored.

---

## Logging

**Recommendation:** `log/slog` (stdlib, Go 1.21+)
**Confidence:** MEDIUM — diverges from karaclean (which uses stdlib `log`)
**Rationale for divergence:** Go 1.26 ships `log/slog` as stable standard library. This project has structured logging needs (user identity in log lines, Polar API call outcomes, startup security banner). `slog.With("user", identity)` is the idiomatic way to attach per-request context. The startup security banner (`Trusting header 'Remote-User' from requests bearing 'X-Proxy-Auth'...`) benefits from structured fields. Karaclean is a cron tool with simple output; this is an HTTP server with concurrent request handling.

If you prefer strict convention parity with karaclean, use stdlib `log` — it is sufficient and there is no functional difference at homelab scale. The recommendation is `slog` for forward compatibility and structured output; the decision is yours.

**Do NOT use:** zerolog, zap, logrus — external logging libraries are not worth pulling in when stdlib `slog` covers the use case.

---

## Encryption

**Recommendation:** `crypto/aes` + `crypto/cipher` + `crypto/rand` (stdlib only)
**Confidence:** HIGH

AES-256-GCM is available in stdlib. No external crypto library needed. Pattern:
- Key: 32 bytes from `KeyProvider`
- Nonce: 12 bytes from `crypto/rand` per encryption call
- Storage: `nonce (12 bytes) || ciphertext` as a single BLOB column
- Decryption: slice `blob[:12]` for nonce, `blob[12:]` for ciphertext

**Do NOT use:** `golang.org/x/crypto` for this — AES-GCM is in stdlib. `x/crypto` is needed only for Argon2/scrypt/ChaCha20 etc.

---

## HTTP Client for Polar API

**Recommendation:** stdlib `net/http` with a custom `http.Client`
**Confidence:** HIGH

No generated client from the Polar OpenAPI spec in v1 — the surface is small (4 endpoints: register user, create/list/delete training targets). Hand-roll typed request/response structs. The Polar API uses standard REST+JSON; no special transport is needed. Set a reasonable timeout (30s) on the `http.Client`.

**Do NOT use:** oapi-codegen for the Polar client in v1 — the generated code surface is larger than the hand-rolled equivalent for 4 endpoints.

---

## Test Tooling

**Recommendation:** `go test -race ./...` (stdlib), no external test framework
**Confidence:** HIGH — mirrors karaclean exactly

`testify` is not in karaclean; do not add it here. Stdlib `testing` package with table-driven tests. Use `httptest.NewServer` and `httptest.NewRecorder` for HTTP handler tests. Use in-memory SQLite (`:memory:` DSN) for store tests.

Add `CGO_ENABLED=0` to the `go test` invocation in CI to catch any CGO leakage at test time rather than at Docker build time.

---

## Documentation

**Recommendation:** MkDocs Material + GitHub Pages via Actions
**Confidence:** HIGH (user-specified)

- `mkdocs.yml` at repo root
- Theme: `material` with recommended plugins: `search`, `navigation.sections`, `content.code.copy`
- Deploy: `mkdocs gh-deploy` via GitHub Actions workflow triggered on `push: main` or tag

---

## FOSS / Release Infrastructure

**Recommendation:**
- **Changelog:** `git-cliff` or GitHub's auto-generated release notes (conventional commits → automatic CHANGELOG)
- **Semantic versioning:** GitHub release tags (`v0.1.0`, `v0.2.0`)
- **Release workflow:** `.github/workflows/release.yml` — triggered on `push: tags: v*`; builds multi-arch Docker image, creates GitHub Release with CHANGELOG entry
- **Lint action:** `golangci/golangci-lint-action@v9` with `version: v2.11` (exact match with karaclean)
- **CI registry:** `ghcr.io/${{ github.repository }}` (mirrors karaclean)
- **CI tags:** `latest` (on `push: main`) + `sha-<commit>` (always) — same tagging scheme as karaclean

Karaclean does not have a release workflow. This project adds one. `git-cliff` can generate CHANGELOG from conventional commits automatically at release time.

---

## Dependency Summary

| Purpose | Library | Version strategy |
|---|---|---|
| MCP server | `github.com/mark3labs/mcp-go` | latest at init |
| SQLite driver | `modernc.org/sqlite` | latest at init |
| Schema migrations | `github.com/golang-migrate/migrate/v4` | latest at init |
| OAuth2 client | `golang.org/x/oauth2` | latest at init |
| Logging | `log/slog` | stdlib (Go 1.26) |
| Encryption | `crypto/aes`, `crypto/cipher`, `crypto/rand` | stdlib |
| HTTP client | `net/http` | stdlib |
| HTTP router | `net/http` ServeMux | stdlib |
| Test | `testing`, `net/http/httptest` | stdlib |

**External dependency count: 4** (mcp-go, sqlite, migrate, oauth2). This is minimal and intentional.

---

## What NOT to Use

| Library | Reason |
|---|---|
| `mattn/go-sqlite3` | CGO required; breaks `FROM scratch` |
| `github.com/golang-migrate/migrate/v4/database/sqlite3` | CGO-backed; import path trap |
| `chi`, `gorilla/mux`, `echo` | Overkill for 6 routes; stdlib ServeMux is sufficient in Go 1.22+ |
| `zerolog`, `zap`, `logrus` | Not worth the dependency; stdlib `slog` covers the need |
| oapi-codegen for Polar client | Generated surface > hand-rolled for 4 endpoints |
| `golang.org/x/crypto` for AES-GCM | Stdlib has it; x/crypto is for other algorithms |
| `testify` | Not in karaclean; not needed |
