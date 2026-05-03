# Architecture Patterns: polar-flow-mcp

**Domain:** Multi-user MCP server wrapping a third-party OAuth2 REST API (Polar AccessLink)
**Researched:** 2026-05-03
**Overall confidence:** HIGH — all critical patterns verified via Context7 (mcp-go), official pkg.go.dev docs (golang-migrate/sqlite), and multiple authoritative web sources.

---

## Recommended Package Layout

```
polar-flow-mcp/
  cmd/
    polar-flow-mcp/
      main.go          ← wiring only; no business logic
  internal/
    config/            ← env var loading, startup validation, fail-closed checks
    auth/              ← proxy secret enforcement (middleware)
    store/             ← SQLite pool, migrations, CRUD for users + tokens
    crypto/            ← AES-256-GCM encrypt/decrypt, KeyProvider interface
    polar/             ← Polar AccessLink HTTP client
    oauth/             ← /oauth/login and /oauth/callback HTTP handlers + state store
    mcp/               ← MCP tool handlers (create/list/delete targets, user info)
  migrations/
    001_initial.up.sql
    001_initial.down.sql
    ...
```

### Dependency Graph (arrows = "imports")

```
main
  ├── config
  ├── auth          ← config
  ├── store         ← config, crypto
  ├── crypto        ← config
  ├── polar         ← (no internal deps; takes token as argument)
  ├── oauth         ← config, store, polar
  └── mcp           ← store, polar, crypto (token decryption happens in store)
```

Key invariant: `polar` has no internal dependencies — it takes a bearer token as a plain argument and makes HTTP calls. This keeps it independently testable and prevents import cycles. `crypto` similarly has no internal deps beyond `config` (for the key bytes). `store` owns the full encrypt/decrypt lifecycle for token persistence, calling `crypto` internally.

---

## Component Boundaries

| Package | Single Responsibility | Public Surface |
|---|---|---|
| `config` | Parse env vars, validate required fields at startup, expose typed `Config` struct | `Load() (*Config, error)` |
| `auth` | Verify `PROXY_SHARED_SECRET` on every inbound request; 401 on mismatch | `Middleware(secret string) http.Handler` |
| `store` | Open SQLite, run migrations, CRUD for `users` and `polar_tokens` tables | `Open()`, `UpsertUser()`, `GetToken()`, `SetToken()`, `PendingAuth*` methods |
| `crypto` | AES-256-GCM encrypt/decrypt; `KeyProvider` interface for future Vault/KMS | `Encrypt(plain []byte) ([]byte, error)`, `Decrypt(blob []byte) ([]byte, error)` |
| `polar` | Typed HTTP client for Polar AccessLink v3 | `NewClient(token string)`, `CreateTrainingTarget()`, `ListTrainingTargets()`, etc. |
| `oauth` | `/oauth/login` redirect, `/oauth/callback` token exchange, state lifecycle | `LoginHandler`, `CallbackHandler` |
| `mcp` | One function per MCP tool; reads identity from context, fetches token from store | `RegisterTools(s *server.MCPServer, deps Deps)` |

---

## How `WithHTTPContextFunc` Works for Multi-User Identity

**Verified: HIGH confidence (Context7 / mark3labs/mcp-go docs)**

`WithHTTPContextFunc` is called **per HTTP request** (not per session). The function receives the raw `*http.Request` and returns an enriched `context.Context`. That context flows into every tool handler invoked for that request.

The pattern for this project:

1. The reverse proxy (Authelia/Authentik/oauth2-proxy) injects a trusted header — for example `X-Remote-User` — containing the authenticated user's identity (email or username).
2. `WithHTTPContextFunc` reads that header and also reads the `PROXY_SHARED_SECRET` header, then stores both in context using unexported key types.
3. Tool handlers call a helper `IdentityFromContext(ctx)` which returns the identity string. They pass it to `store.GetToken(identity)` to retrieve the encrypted Polar token for that specific user.

Each HTTP request is handled in its own goroutine by Go's stdlib `net/http`. The `WithHTTPContextFunc` closure itself does no mutation of shared state — it only reads from the request and writes to the local context — so it is goroutine-safe by construction.

If `WithHTTPContextFunc` finds no identity header or an invalid secret, the tool handler should detect the empty/missing value and return `mcp.NewToolResultError("unauthenticated")`. The `auth` middleware (registered before the MCP handler in the HTTP mux) should reject these requests first, so the tool handler check is defense-in-depth.

**Session vs. request identity:** With `WithStateful(true)`, mcp-go tracks sessions by `Mcp-Session-Id` header across multiple HTTP requests for the same conversation. `WithHTTPContextFunc` is still called on every request, so the identity is re-extracted every time. This is the correct behavior: the proxy header is trusted, re-reading it every request means a user whose proxy session expires will fail on the next tool call rather than carrying stale identity. Do not cache identity in the mcp-go session object — that would bypass the proxy re-validation.

---

## Data Flow: Complete MCP Tool Call

```
Claude Desktop
  │  POST /mcp  { tool: "create_training_target", ... }
  │  Headers: X-Remote-User: alice@example.com
  │           X-Proxy-Secret: <shared-secret>
  ▼
net/http router (main.go)
  │
  ├─ auth.Middleware   ← compares X-Proxy-Secret to config.ProxySecret
  │   fail → 401 Unauthorized (request rejected here)
  │   pass ↓
  │
  ├─ StreamableHTTPServer.ServeHTTP
  │   │  WithHTTPContextFunc fires:
  │   │    ctx["identity"] = "alice@example.com"
  │   │    ctx["proxy_validated"] = true
  │   ▼
  │  mcp tool dispatch
  │   │
  │   └─ mcp.CreateTrainingTargetHandler(ctx, req)
  │       │
  │       ├─ identity = IdentityFromContext(ctx)   // "alice@example.com"
  │       │   empty → return ToolResultError
  │       │
  │       ├─ store.GetToken(ctx, identity)
  │       │   │  SELECT encrypted_token, nonce, key_version
  │       │   │  FROM polar_tokens WHERE user_identity = ?
  │       │   │
  │       │   └─ crypto.Decrypt(nonce + ciphertext) → bearer token []byte
  │       │       not found / decrypt error → return ToolResultError("not linked")
  │       │
  │       ├─ polar.NewClient(bearerToken)
  │       │
  │       └─ client.CreateTrainingTarget(ctx, polarUserID, params)
  │           │  POST https://www.polaraccesslink.com/v3/users/{id}/training-targets
  │           └─ parse response → return ToolResultText(JSON)
  │
  └─ StreamableHTTPServer writes SSE/JSON response to client
```

---

## Data Flow: OAuth Login + Callback

```
User (browser or Claude tool invocation)
  │  GET /oauth/login
  │  Headers: X-Remote-User: alice@example.com
  │           X-Proxy-Secret: <shared-secret>
  ▼
auth.Middleware  ← same secret check as above
  ▼
oauth.LoginHandler
  │
  ├─ identity = r.Header.Get(config.IdentityHeader)
  │
  ├─ state = generateOpaqueToken()   // crypto/rand, 32 bytes, hex-encoded
  │
  ├─ store.CreatePendingAuth(ctx, state, identity, time.Now().Add(10*time.Minute))
  │   INSERT INTO pending_auth (state, identity, expires_at) VALUES (?, ?, ?)
  │
  └─ http.Redirect → https://flow.polar.com/oauth2/authorization
         ?client_id=...&redirect_uri=...&state=<state>&scope=accesslink.read_all

Polar authorization server (user grants access)
  │
  │  GET /oauth/callback?code=<code>&state=<state>
  ▼
auth.Middleware  ← proxy secret still required on callback
  ▼
oauth.CallbackHandler
  │
  ├─ state = r.URL.Query().Get("state")
  │
  ├─ store.ConsumePendingAuth(ctx, state)
  │   SELECT identity FROM pending_auth WHERE state = ? AND expires_at > NOW()
  │   DELETE FROM pending_auth WHERE state = ?   (consume = single-use)
  │   state not found / expired → 400 Bad Request
  │
  ├─ POST https://polarremote.com/v2/oauth2/token   (code exchange)
  │   → { access_token, token_type, x_user_id }
  │
  ├─ POST https://www.polaraccesslink.com/v3/users  (register if new)
  │   bearer = access_token
  │   409 Conflict is OK (already registered)
  │
  ├─ crypto.Encrypt([]byte(access_token)) → nonce + ciphertext blob
  │
  ├─ store.UpsertToken(ctx, identity, polarUserID, encryptedBlob, keyVersion=1)
  │   INSERT OR REPLACE INTO polar_tokens ...
  │
  └─ respond 200 "Polar account linked successfully"
```

---

## SQLite Schema and Migration Approach

**Verified: HIGH confidence (golang-migrate pkg.go.dev + Context7)**

### golang-migrate with modernc.org/sqlite

- Import path: `github.com/golang-migrate/migrate/v4/database/sqlite`
- This package explicitly uses `modernc.org/sqlite` (not mattn/go-sqlite3). No CGO required.
- Driver name registered: `"sqlite"` (DSN scheme: `sqlite:///path/to/db.sqlite`)
- Blank import required in `store` package: `_ "github.com/golang-migrate/migrate/v4/database/sqlite"`
- Embed migration files with `//go:embed migrations/*.sql` in `store` package; use `iofs.New(migrationsFS, "migrations")` to create the source driver, then `migrate.NewWithSourceInstance(...)`.
- Migrations are wrapped in implicit transactions by default; do not add `BEGIN`/`COMMIT` in SQL files.

### Proposed Schema

```
users
  id           INTEGER PRIMARY KEY AUTOINCREMENT
  identity     TEXT UNIQUE NOT NULL    -- proxy header value (e.g. alice@example.com)
  polar_user_id TEXT                   -- Polar's user ID, populated after OAuth
  created_at   DATETIME NOT NULL

polar_tokens
  id           INTEGER PRIMARY KEY AUTOINCREMENT
  user_id      INTEGER NOT NULL REFERENCES users(id)
  encrypted_token BLOB NOT NULL        -- nonce (12 bytes) || ciphertext
  key_version  INTEGER NOT NULL DEFAULT 1
  updated_at   DATETIME NOT NULL

pending_auth
  state        TEXT PRIMARY KEY        -- opaque CSRF token
  identity     TEXT NOT NULL           -- which proxy user initiated login
  expires_at   DATETIME NOT NULL       -- typically NOW + 10 minutes
```

### BLOB Storage Pattern

Scanning BLOBs with modernc.org/sqlite via database/sql is idiomatic: scan directly into `[]byte`. The driver maps SQLite BLOB affinity to Go `[]byte` automatically. Store the nonce prepended to the ciphertext as a single column: `nonce (12 bytes) || ciphertext`. On read, slice `blob[:12]` for the nonce and `blob[12:]` for the ciphertext before decrypting. No known issues with this pattern.

---

## Concurrency and State Management

### SQLite Connection Pool

SQLite with WAL mode supports concurrent readers but serializes writers. Configure two `*sql.DB` pools:

- **Write pool**: `MaxOpenConns(1)` — serializes all mutations, eliminates `SQLITE_BUSY` errors
- **Read pool**: `MaxOpenConns(N)` — N = runtime.NumCPU() or a small fixed number

Enable these pragmas on connection open (via `?_pragma=...` in DSN or via `db.Exec`):
- `journal_mode=WAL`
- `busy_timeout=5000`
- `synchronous=NORMAL`
- `foreign_keys=ON`

This is a well-established pattern for Go HTTP servers backed by SQLite. The write serialization is safe here because write frequency is low (OAuth flows are infrequent; tool calls are read-heavy: decrypt token → GET Polar API).

### OAuth State Store: SQLite, Not In-Memory

Store `pending_auth` in SQLite, not in a `sync.Map`:

- **Restart safety**: a server restart during an OAuth flow does not orphan the user with a dangling state token
- **No race condition**: `ConsumePendingAuth` is a single SQL statement (`DELETE ... WHERE state = ? AND expires_at > NOW() RETURNING identity`). SQLite serializes this via the write pool, making it atomically consume-on-first-use with no mutex needed in application code.
- **Automatic expiry**: a periodic goroutine (or migration-time trigger) can sweep `DELETE FROM pending_auth WHERE expires_at < NOW()`. Alternatively, simply let expired rows sit; the expiry check in the query is sufficient.

In-memory state maps have two failure modes: server restart during OAuth flow loses state; and if the application ever scales to two instances, in-memory state is not shared. SQLite eliminates both.

### Tool Handler Concurrency

Tool handlers are invoked concurrently by Go's HTTP server (one goroutine per in-flight request). They must be stateless: no package-level variables mutated during a call. The only shared mutable state is the `*sql.DB` pools, which are goroutine-safe. The `polar.Client` struct should be created fresh per tool invocation (it holds a `http.Client` with the bearer token) — this avoids any per-user token state leaking between goroutines.

---

## Request Lifecycle: Where Each Concern Lives

| Concern | Package | Mechanism |
|---|---|---|
| Proxy secret validation | `auth` | `http.Handler` middleware, runs before MCP handler |
| Identity extraction | `mcp` (via `WithHTTPContextFunc`) | Per-request closure; writes to context |
| Token retrieval + decryption | `store` + `crypto` | Called inside tool handler from context identity |
| Polar API call | `polar` | Fresh client per invocation |
| OAuth state generation | `oauth` | `crypto/rand`, stored in SQLite |
| OAuth token exchange | `oauth` | HTTP POST to Polar token endpoint |
| Schema migrations | `store` | `golang-migrate` with embedded SQL, runs at startup before serving |
| Startup validation | `config` | `Load()` returns error if `AUTH_PROXY` or `ENCRYPTION_KEY` unconfigured |

---

## HTTP Mux Layout (main.go wiring)

The server exposes four route groups:

```
GET  /healthz              ← inline handler, no auth
GET  /readyz               ← inline handler, checks DB ping
POST /mcp  (all methods)   ← auth.Middleware → StreamableHTTPServer
GET  /oauth/login          ← auth.Middleware → oauth.LoginHandler
GET  /oauth/callback       ← auth.Middleware → oauth.CallbackHandler
```

`net/http` stdlib `ServeMux` is sufficient. Register health/readyz first without the auth middleware so monitoring agents do not need the proxy secret.

The `StreamableHTTPServer` from mcp-go is used as an `http.Handler` and mounted at `/mcp`. The `auth.Middleware` wraps it in the mux registration. `WithHTTPContextFunc` is set on the `StreamableHTTPServer` to extract identity after the middleware has already validated the secret.

Note: the Context7 docs note that `WithHTTPContextFunc` "only works for the `Start` method" when the server calls `Start()` itself. When mounted as `http.Handler` into an external mux, verify that the context func is still invoked — review the mcp-go `ServeHTTP` implementation before finalizing this wiring pattern. Alternative: perform header extraction in the `auth` middleware itself and store identity in `r.Context()` using `context.WithValue`, then the MCP handler just reads from the already-enriched context. This is the safer approach.

---

## Build Order

The dependency graph implies this build order:

1. `config` — no internal deps; everything else reads config
2. `crypto` — depends only on config (for key bytes)
3. `store` — depends on config and crypto; must be buildable standalone for migration testing
4. `polar` — depends on nothing internal; build and test against Polar API contracts
5. `auth` — depends only on config (secret string)
6. `oauth` — depends on config, store, polar
7. `mcp` — depends on store and polar; tool handlers are the last leaf
8. `cmd/polar-flow-mcp/main.go` — wires everything together

This order minimizes circular dependency risk. The only non-obvious boundary: `store` calls `crypto` (not the reverse), and `mcp` does not call `crypto` directly — token decryption is encapsulated in `store.GetToken()`. Keeping decryption inside `store` means `mcp` handlers have zero knowledge of encryption mechanics.

---

## Anti-Patterns to Avoid

### Storing decrypted tokens in context or session
Decrypted bearer tokens must never be stored in mcp-go session state or in context values that persist across requests. Decrypt on-demand per tool call. If a key is ever rotated, in-flight sessions carrying decrypted tokens become stale.

### Global identity map keyed by session ID
Do not build an application-level `map[sessionID]identity` protected by a mutex. The proxy re-sends the identity header on every request; reading it fresh every time via `WithHTTPContextFunc` (or the middleware alternative) is simpler and more correct.

### Single `*sql.DB` for both reads and writes
Without separate write pool (MaxOpenConns=1), concurrent HTTP handlers issuing writes will race for the SQLite write lock and produce `SQLITE_BUSY` errors. The dual-pool pattern is required.

### Storing OAuth state in-memory
See "OAuth State Store: SQLite, Not In-Memory" above. In-memory state is lost on restart and cannot span two instances.

### Calling `polar.NewClient` with a cached token at server startup
There is no global Polar client. Each tool handler creates a client from the decrypted per-user token at invocation time.

---

## Scalability Notes

This is a homelab / small-team target (n = 1..~20 users). The architecture does not need to scale beyond that. SQLite is appropriate; the dual-pool pattern handles the concurrency level comfortably. If scale requirements change significantly, the `store` interface boundary makes it feasible to swap to Postgres without touching `mcp` or `polar`.

---

## Sources

- mcp-go `WithHTTPContextFunc` and `WithStateful`: https://context7.com/mark3labs/mcp-go/llms.txt (HIGH confidence)
- mcp-go server package API: https://pkg.go.dev/github.com/mark3labs/mcp-go/server (HIGH confidence)
- golang-migrate sqlite driver (modernc.org/sqlite): https://pkg.go.dev/github.com/golang-migrate/migrate/v4/database/sqlite (HIGH confidence)
- golang-migrate iofs embed pattern: https://context7.com/golang-migrate/migrate/llms.txt (HIGH confidence)
- modernc.org/sqlite BLOB / database/sql: https://pkg.go.dev/modernc.org/sqlite (HIGH confidence)
- SQLite WAL dual-pool pattern: https://turriate.com/articles/making-sqlite-faster-in-go (MEDIUM confidence, multiple sources agree)
- OAuth state CSRF token best practices: https://auth0.com/docs/secure/tokens/token-best-practices (MEDIUM confidence)
