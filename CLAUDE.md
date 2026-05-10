# polar-flow-mcp

Multi-user MCP server in Go wrapping the Polar AccessLink API. Users link their Polar account via OAuth and then create/manage training targets directly from Claude conversations. Deployed behind a reverse-proxy auth layer (Authelia, Authentik, oauth2-proxy, etc.).

## GSD Workflow

This project uses the GSD (Get Shit Done) workflow for planning and execution.

**Current state:** Planning complete — ready to build.

**Next step:** `/gsd-discuss-phase 1` (or `/gsd-plan-phase 1` to skip discussion)

**Phases:**
1. Project Foundation and Security Skeleton — Go scaffold, fail-closed auth, SQLite + crypto, Docker, CI
2. OAuth Link Flow + UserInfo — Polar OAuth2, encrypted token storage, get_user_info tool
3. Core MCP Tools + Bundled Skill — create/list/delete training targets, polar-coach skill
4. Documentation, FOSS Hygiene, Release — MkDocs site, release workflow, FOSS files

## Pre-Commit Checklist

1. **Lint**: Run `~/go/bin/golangci-lint run ./...` before committing. Fix all lint errors.
2. **Tests**: Run `CGO_ENABLED=0 go test -tags=polartest -race -count=1 ./...` before committing. Fix all failures.
3. **Documentation**: Update docs when features are added or modified. Do not defer.

## Project Conventions (mirror karaclean exactly)

- Go 1.26, module `github.com/lm/polar-flow-mcp`
- Layout: `cmd/polar-flow-mcp/main.go` + `internal/{config,auth,store,crypto,polar,oauth,mcp}/`
- Logging: `log/slog` (stdlib; Go 1.26)
- golangci-lint v2.11 (standard + gocyclo/godot/misspell/noctx, errcheck type assertions)
- Docker: `golang:1.26-alpine` builder → `FROM scratch` final; `CGO_ENABLED=0`, `-ldflags="-w -s"`, copy `ca-certificates.crt`
- CI: `test` + `lint` → `docker` (ghcr.io, `latest` + SHA tags) — same structure as karaclean

## Critical Decisions (do not revisit without explicit discussion)

| Decision | Why |
|----------|-----|
| `modernc.org/sqlite` (NOT `mattn/go-sqlite3`) | CGO disabled; `FROM scratch` requires pure Go |
| `golang-migrate/migrate/v4/database/sqlite` (NOT `sqlite3`) | Same reason; import path suffix is the only diff, CGO impact is total |
| AES-256-GCM nonce (12 bytes) \|\| ciphertext stored as single BLOB | Prevents nonce/ciphertext misalignment; schema is immutable after data is written |
| `subtle.ConstantTimeCompare` for proxy secret; runs BEFORE identity header is read | Security ordering cannot be inverted; prevents timing oracle on secret |
| OAuth CSRF state in SQLite (NOT in-memory map) | Restart-safe; atomic consume-on-first-use via `DELETE ... RETURNING` |
| SQLite WAL mode + dual `*sql.DB` pools (write: MaxOpenConns=1, read: MaxOpenConns=4) | Prevents SQLITE_BUSY under concurrent HTTP requests |
| Unexported struct type as context key for user identity | Prevents cross-package context key collision |

## Open Questions (resolve in Phase 1 planning)

- Does `WithHTTPContextFunc` fire per-request or per-session when `StreamableHTTPServer` is mounted as `http.Handler`? Safe fallback: extract identity in auth middleware via `context.WithValue`.
- Confirm Polar training target JSON shapes against a live account during Phase 2 before Phase 3 builds on them.

## Tech Stack

- Go 1.26 (CGO disabled)
- `github.com/mark3labs/mcp-go` — MCP server (StreamableHTTP transport)
- `modernc.org/sqlite` — pure-Go SQLite driver
- `github.com/golang-migrate/migrate/v4` (sqlite driver) — schema migrations
- `golang.org/x/oauth2` — Polar OAuth2 token exchange
- `log/slog`, `crypto/aes`, `crypto/cipher`, `crypto/rand`, `net/http` — stdlib only for everything else
