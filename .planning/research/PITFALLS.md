# Domain Pitfalls

**Domain:** Multi-user MCP server with proxy-auth, OAuth2, encrypted token storage, SQLite, Go/scratch Docker
**Researched:** 2026-05-03
**Confidence:** HIGH — all critical pitfalls verified against official documentation or CVEs

---

## Critical Pitfalls

Mistakes that cause a rewrite, a security breach, or data corruption.

---

### Pitfall 1: Proxy Auth Header Spoofing — the shared secret is the only real boundary

**What goes wrong:**
The project trusts a header (e.g., `X-Forwarded-User`) to identify who is making the tool call. If the server is reachable from anywhere other than the reverse proxy, any client can forge that header and impersonate any user. The reverse proxy (Authelia, Authentik, oauth2-proxy) passes identity in a header, but it does not and cannot strip that header from requests that bypass it. Authelia's own documentation states: "your proxy must ensure only Authelia is setting these headers, and any other headers are never forwarded to the backend" — the responsibility for stripping is on the proxy config, not the identity provider, and not the application.

A real CVE shows this pattern failing in production: oauth2-proxy CVE-2026-40575 allowed attackers to spoof `X-Forwarded-Uri` to bypass path-based authentication rules when `--reverse-proxy` was enabled, because the proxy trusted client-supplied header values.

The project's mitigation is a `PROXY_SHARED_SECRET` header — a value only the reverse proxy knows, sent with every request. This is the right design. A request carrying the correct shared secret cannot have come from an arbitrary external client. But the implementation must be fail-closed: if the secret header is absent or wrong, the server must reject the request unconditionally before touching any identity header.

**Warning signs:**
- The server starts and serves requests when `PROXY_SHARED_SECRET` is an empty string or the zero value.
- The shared-secret check runs after any logging, tracing, or header-reading code that touches the identity header.
- The `BIND_ADDRESS` default is overridden to `0.0.0.0` without the admin understanding the implication.
- Integration tests pass requests without the shared-secret header and still get 200s.

**Prevention strategy:**
1. Validate the shared-secret header as the very first operation in the HTTP middleware chain — before routing, before reading `X-Forwarded-User`, before logging user identity.
2. Use `subtle.ConstantTimeCompare` to compare the header value to avoid timing-oracle attacks on the secret itself.
3. Refuse to start if `PROXY_SHARED_SECRET` is empty or if `AUTH_PROXY` is `"unconfigured"` — enforced at boot, not just at request time.
4. Bind to `127.0.0.1` by default. Document explicitly that `BIND_ADDRESS=0.0.0.0` requires the operator to ensure network-level isolation.
5. The `/healthz` endpoint may reasonably skip the shared-secret check (probes run from the same host), but `/readyz` and all MCP/OAuth routes must enforce it.

**Phase:** Phase 1 (server skeleton, middleware chain). Cannot be retrofitted safely after tool handlers exist.

---

### Pitfall 2: AES-256-GCM Nonce Reuse — catastrophic, silent, hard to detect after the fact

**What goes wrong:**
GCM mode is an authenticated cipher that is completely broken if the same (key, nonce) pair is ever used twice. The consequences are not graceful degradation — they are full cryptographic collapse:
- Two ciphertexts produced with the same key and nonce can be XOR'd together to cancel out the keystream, revealing `P1 XOR P2`. If either plaintext is known (and OAuth tokens have predictable structure), both are recoverable.
- The GHASH authentication key leaks from two same-nonce ciphertexts, allowing an attacker to forge arbitrary authenticated ciphertexts that pass integrity checks. This means the attacker can substitute their own Polar token for any user.

For per-row random 96-bit nonces, the birthday bound at 2^-32 collision probability is approximately 2^32 (~4.3 billion) encryptions. This project encrypts at most one token per user (tokens do not expire), so random nonces are safe at any practical scale. The real risk is not birthday collision — it is coding mistakes:

- Generating the nonce with `math/rand` instead of `crypto/rand`. `math/rand` is not cryptographically random; it is seeded deterministically and predictable.
- Reusing the nonce buffer across calls in a shared struct (e.g., a package-level `[12]byte` variable written to once and reused).
- Storing the nonce outside the ciphertext blob (separate column) and then accidentally reading the wrong row's nonce due to a query bug.

**Warning signs:**
- Any use of `math/rand` or `rand.Read` (deprecated in Go 1.20+) anywhere near encryption code.
- Nonce stored in a separate database column instead of prepended to the ciphertext blob.
- A `KeyProvider` or `Encrypter` type that holds mutable nonce state at struct level.
- Unit tests for the encrypter that do not assert nonces are unique across multiple calls.

**Prevention strategy:**
1. Always generate nonces via `io.ReadFull(crypto_rand.Reader, nonce[:])` — the canonical Go pattern. Never use `math/rand`.
2. Prepend the nonce to the ciphertext and store them as a single blob (e.g., `nonce || ciphertext`). This eliminates any possibility of misaligning nonce and ciphertext in the database.
3. Schema: one `encrypted_token BLOB NOT NULL` column. The application splits off the first 12 bytes as the nonce on read.
4. Write a unit test that calls `Encrypt` 1000 times and asserts all nonces are distinct.
5. The `key_version` column is already planned — use it to validate the encryption key version on read before decrypting, so a future key-rotation bug surfaces as a clear error rather than a decryption panic.

**Phase:** Phase 1 (database schema, encryption layer). The blob layout is a schema decision that cannot change after data is written without a migration.

---

### Pitfall 3: OAuth CSRF — state parameter implementation errors

**What goes wrong:**
The state parameter exists specifically to bind a callback to the user who initiated the flow and to prevent CSRF. Several failure modes exist:

**Failure mode A — static or predictable state.**
A state value that is reused across users, derived from a predictable value (timestamp, user ID alone), or too short allows an attacker to initiate a flow, observe the state in the redirect URL, and inject a crafted callback for a different user.

**Failure mode B — concurrent users, one state store.**
If two users begin OAuth at the same time and state is stored in a single global variable (or an in-memory map keyed only by state value without user binding), a race condition can cause user A's callback to be validated against user B's state. oauth2-proxy hit this exact issue in production (issue #611: "multitab authentication race condition failure").

**Failure mode C — no expiry, enabling replay.**
Without a short TTL on stored state (5–10 minutes is standard), a state value remains valid indefinitely. An attacker who obtains an old state (e.g., from browser history or a log) can replay it.

**Failure mode D — unknown state returns a helpful error.**
If the callback responds to unknown state with a redirect or a message that reveals what state values are valid, it aids attackers. Unknown state must result in a generic 400 with no detail.

**Failure mode E — Polar registration idempotency.**
The Polar AccessLink API requires `POST /v3/users` after the OAuth token exchange to register the user. If this call is not idempotent-safe, a retry of the callback route (e.g., a network blip causing the browser to resubmit) can fail with a 409. The 409 on user registration is explicitly expected by the Polar API and must be treated as success, not an error, so long as the user row already exists locally.

**Warning signs:**
- State is stored in a Go `map[string]string` without a mutex (data race under `go test -race`).
- State values are not deleted after successful validation (replay possible).
- State does not include or verify the initiating user's identity header value.
- The callback handler returns the state value or any internal error detail in the HTTP response.
- The TTL for pending state entries is absent or longer than 10 minutes.

**Prevention strategy:**
1. State = `crypto/rand`-generated 32-byte hex string. Store in SQLite with columns: `state TEXT PRIMARY KEY`, `user_id TEXT NOT NULL`, `created_at INTEGER NOT NULL`.
2. On callback: look up state in DB, verify `user_id` matches the current `X-Forwarded-User` header, verify `created_at > now - 10 minutes`, then delete the row immediately.
3. If state is missing from the request, unknown in the DB, expired, or user_id mismatched: respond `400 Bad Request` with a generic message. No redirect, no detail.
4. Use SQLite for state storage, not an in-memory map. This eliminates the concurrency race and makes state survive a server restart during a flow.
5. Treat Polar's `POST /v3/users` 409 as idempotent success.

**Phase:** Phase 2 (OAuth flow). The SQLite state table is part of the initial schema migration.

---

### Pitfall 4: MCP Tool Context Leakage — using the wrong user's Polar token

**What goes wrong:**
Every MCP tool call arrives in a goroutine with its own `context.Context`. The `WithHTTPContextFunc` callback injects the proxy identity into that context. A tool handler that retrieves the current user's identity from anywhere other than the request context will silently use the wrong identity under concurrent load.

The classic failure modes:

**Failure mode A — package-level or struct-level "current user" variable.**
If a handler or service stores the current user in a field (`s.currentUser`) or a package-level variable, two simultaneous tool calls will race, and one handler will operate on the other user's token.

**Failure mode B — context key collision.**
If the identity context key is a plain `string` (e.g., `"user"`), any other middleware or library that also sets a context value with the key `"user"` will shadow it. The handler then reads the wrong value or a zero value, which either fails with a confusing error or, worse, falls through to a default that uses the first user in the database.

**Failure mode C — missing identity check.**
A tool handler that assumes context always has a valid user (because the middleware is supposed to enforce it) and panics or returns a generic error on zero value is better than one that silently proceeds with an empty user ID. But it's still wrong. Each handler must explicitly extract the identity, check it is non-empty, and return a clear `"authentication required"` error if absent.

**Failure mode D — database query without user scope.**
A query like `SELECT * FROM training_targets ORDER BY created_at DESC LIMIT 10` with no `WHERE user_id = ?` clause will return data from all users to whichever tool call runs it first.

**Warning signs:**
- Any mutable field on a handler struct that carries request-scoped state.
- Context key defined as a `string` or `int` type rather than an unexported struct type.
- SQL queries in tool handlers that do not include `WHERE user_id = ?` with a parameter bound from context.
- `go test -race` failures in tool handler tests that run two users concurrently.
- A helper function like `getCurrentUser()` with no `ctx` parameter.

**Prevention strategy:**
1. Define the identity context key as an unexported struct type in the `internal/auth` package: `type userIDKey struct{}`. This prevents collision with any other package's context keys.
2. Write a `UserIDFromContext(ctx) (string, bool)` accessor — the boolean forces every call site to handle the "not present" case explicitly.
3. Every tool handler's first action: extract user ID from context. Return `mcp.NewToolResultError("authentication required")` if absent. Never proceed past this check.
4. Every database function that touches user data takes `userID string` as an explicit parameter, not a context extraction inside the DB layer.
5. Run `go test -race` as part of CI. Write a test that invokes two tool calls for different users concurrently and asserts each gets its own data.

**Phase:** Phase 1 (middleware, context extraction). Phase 3 (tool handlers) must use the accessor established in Phase 1.

---

## Moderate Pitfalls

---

### Pitfall 5: SQLite Concurrency — SQLITE_BUSY errors under load

**What goes wrong:**
SQLite allows only one writer at a time. In default journal mode, a write also blocks all readers. In a concurrent HTTP server handling multiple tool calls simultaneously, naive use of a single `*sql.DB` with default settings causes frequent `SQLITE_BUSY` (database locked) errors because Go's `database/sql` pool may open multiple connections that race for the write lock.

Two common mistakes:

**Mistake A — no WAL mode.**
Without `PRAGMA journal_mode=WAL`, readers block writers and writers block readers. Any concurrent read during a write returns SQLITE_BUSY.

**Mistake B — no busy_timeout.**
`PRAGMA busy_timeout` defaults to 0, meaning SQLite returns SQLITE_BUSY immediately on lock contention rather than retrying. In a server that processes multiple requests per second, even a 100ms busy_timeout prevents the vast majority of contention errors.

**Mistake C — multiple `*sql.DB` instances with default settings.**
Two `*sql.DB` objects both configured for reads and writes will contend on the write lock. The recommended pattern for WAL mode is a single write connection (`db.SetMaxOpenConns(1)`) and a separate read pool.

**Warning signs:**
- `database is locked` errors in logs during load tests.
- `go test -race` passing but integration tests failing under parallel HTTP requests.
- The DSN does not include `?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` (or equivalent).

**Prevention strategy:**
1. Open the SQLite file with WAL mode and busy_timeout set via DSN parameters: `file:polar.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)`.
2. Use a single `*sql.DB` for writes with `SetMaxOpenConns(1)` to serialize all write transactions.
3. Use a second `*sql.DB` opened read-only (or the same file with a separate pool) for reads; set `SetMaxOpenConns(4)` or `min(runtime.NumCPU(), 4)`.
4. All write operations use `BEGIN IMMEDIATE` transactions to acquire the write lock upfront and avoid the upgrade-from-deferred deadlock.
5. The modernc.org/sqlite driver supports WAL pragma via DSN — verify this in the driver's documentation before assuming standard SQLite DSN parameters apply.

**Phase:** Phase 1 (database initialization). The WAL pragma must be set before any schema migrations run.

---

### Pitfall 6: golang-migrate Driver Mismatch — sqlite3 (CGO) vs sqlite (pure Go)

**What goes wrong:**
golang-migrate has two SQLite driver packages:
- `github.com/golang-migrate/migrate/v4/database/sqlite3` — uses `mattn/go-sqlite3` (CGO required)
- `github.com/golang-migrate/migrate/v4/database/sqlite` — uses `modernc.org/sqlite` (pure Go)

The project requires CGO disabled. Importing the `sqlite3` package will cause a build failure or CGO linkage at container build time. The error is not obvious: the import compiles fine on a dev machine with CGO enabled, and the bug only surfaces during `CGO_ENABLED=0 go build` in the Docker build stage.

A partial migration failure in the `sqlite` driver is safe by default: the driver wraps each migration file in an implicit transaction. If a migration file fails mid-run, the transaction is rolled back and the `schema_migrations` version table is not advanced. However, if a migration file contains a DDL statement that SQLite cannot roll back (e.g., certain `DROP TABLE` variants with foreign key constraints in edge cases), the database can be left in a partial state.

**Warning signs:**
- `import _ "github.com/golang-migrate/migrate/v4/database/sqlite3"` anywhere in the codebase.
- CI `docker` job succeeds but the `test` job (which may run with CGO enabled locally) hides the mismatch.
- Migration files contain explicit `BEGIN` / `COMMIT` statements (conflicts with driver's implicit transaction wrapping).
- `x-no-tx-wrap=true` in the DSN without a corresponding explicit transaction wrapping every migration.

**Prevention strategy:**
1. Import only `github.com/golang-migrate/migrate/v4/database/sqlite` (no trailing `3`).
2. Add `CGO_ENABLED=0` to the `go test` command in CI to catch CGO leakage at test time, not just at Docker build time.
3. Never put explicit `BEGIN` / `COMMIT` in migration files unless `x-no-tx-wrap=true` is set.
4. Keep each migration file to a single logical change (one table create, one column add). Smaller files mean smaller rollback scope if something goes wrong.
5. Test migrations against a fresh database in CI, not just the current dev database.

**Phase:** Phase 1 (schema migrations setup). The driver import is a one-time decision that affects the entire migration infrastructure.

---

### Pitfall 7: scratch Docker Image — missing TLS certificates for Polar API calls

**What goes wrong:**
The karaclean Dockerfile copies only `ca-certificates.crt` and the binary to `FROM scratch`. This is correct for a service that makes outbound HTTPS calls. For polar-flow-mcp, the server makes outbound HTTPS requests to:
- `https://flow.polar.com/oauth2/authorization` (redirect only — no direct HTTP call)
- `https://polarremote.com/v2/oauth2/token` (token exchange — direct HTTPS POST)
- `https://www.polaraccesslink.com/v3/users/*` (all API calls — direct HTTPS)

Without `ca-certificates.crt` in the final image, all TLS handshakes will fail with `x509: certificate signed by unknown authority`. karaclean already handles this correctly. The risk is a deviation from the pattern during development — e.g., someone adds a distroless base "temporarily" during debugging and forgets to re-check the final certificate copy.

Timezone data is not needed: the project stores timestamps as UTC integers in SQLite and does not format times for display. The `time/tzdata` embed is unnecessary here, but it also causes no harm if added. The Polar API returns RFC3339 timestamps that Go parses without timezone database.

One additional gap: `FROM scratch` has no `/etc/passwd` or `/etc/group`, so the process runs as root (UID 0) by default. While this is acceptable for a container with no other processes, it violates the principle of least privilege. karaclean does not currently address this — the project should not introduce a regression here, but it is worth noting for a future hardening pass.

**Warning signs:**
- Any outbound HTTPS call during integration tests fails with `x509: certificate signed by unknown authority`.
- The `COPY --from=builder /etc/ssl/certs/ca-certificates.crt` line is absent or commented out in the Dockerfile.
- The Dockerfile uses a base other than `scratch` (e.g., `alpine` or `distroless`) without justification.

**Prevention strategy:**
1. Mirror karaclean's Dockerfile exactly: `COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/` before the binary copy.
2. Add an integration test that performs a real (or mocked) HTTPS GET and fails if the certificate chain cannot be verified.
3. Do not add timezone data — it is not needed, and adding unnecessary files contradicts the scratch philosophy.
4. CI `docker` job should run the container and hit `/healthz` — if TLS is broken, any startup check that calls Polar will surface it immediately.

**Phase:** Phase 1 (Docker infrastructure). Mirror karaclean. Do not diverge.

---

### Pitfall 8: BIND_ADDRESS=0.0.0.0 — network exposure without visible warning

**What goes wrong:**
The default `BIND_ADDRESS=127.0.0.1` is correct. But self-hosting operators routinely override this to `0.0.0.0` when they cannot get a localhost-only connection working in Docker. Once the server binds to all interfaces, several attack surfaces open:

1. **Direct header spoofing.** Any client on the LAN or any process in the Docker network can reach the server directly and inject `X-Forwarded-User`. The shared-secret header is the mitigation, but operators who do not understand the security model may not set it.
2. **Browser drive-by attack.** Major browsers (as of 2025) treat `http://0.0.0.0:<port>` as equivalent to localhost. A malicious webpage visited by the operator can make JavaScript requests directly to the MCP server, bypassing same-origin policy.
3. **LAN exposure.** In a homelab, the server is now reachable from every device on the network segment, including less-trusted devices.

The `/readyz` endpoint is where a bind-address check should live: if `BIND_ADDRESS` is not `127.0.0.1` and `PROXY_SHARED_SECRET` is not set, `/readyz` should return 503 to prevent the server from being registered as healthy behind the proxy.

**Warning signs:**
- Server starts successfully with `BIND_ADDRESS=0.0.0.0` and `PROXY_SHARED_SECRET` unset (or default/empty).
- The documentation does not explicitly warn that `BIND_ADDRESS=0.0.0.0` requires `PROXY_SHARED_SECRET` to be set.
- The `/readyz` endpoint returns 200 regardless of configuration validity.

**Prevention strategy:**
1. At startup, if `BIND_ADDRESS` is not `127.0.0.1` (or `::1`), emit a prominent `WARN` log: "BIND_ADDRESS is not localhost — ensure PROXY_SHARED_SECRET is set and the server is not reachable without the reverse proxy."
2. `/readyz` checks: is `AUTH_PROXY` not `"unconfigured"`? Is `PROXY_SHARED_SECRET` non-empty? If either fails, return `503 Service Unavailable` with a body naming the failed check.
3. Document the security model explicitly in the deployment guide: the shared secret is the real security boundary; network isolation is defense-in-depth.
4. Do not attempt to automatically refuse to start on `0.0.0.0` — operators may have legitimate Docker networking reasons for it. Warn loudly; don't block.

**Phase:** Phase 1 (configuration, startup checks, health endpoints).

---

## Minor Pitfalls

---

### Pitfall 9: Polar User Registration — 409 treated as an error

**What goes wrong:**
After the OAuth token exchange, the Polar AccessLink API requires `POST /v3/users` to register the user for the client application. If the user has previously connected and been registered, this call returns HTTP 409 Conflict. The Polar documentation states this is expected and should be ignored. If the application treats 409 as an unrecoverable error, the OAuth callback will fail for any user who re-connects after revoking and re-granting access, or after a DB reset.

**Prevention strategy:**
Treat 409 from `POST /v3/users` as success. Log it at DEBUG level. Proceed to store the token.

**Phase:** Phase 2 (OAuth callback handler).

---

### Pitfall 10: Missing `_pragma=foreign_keys(ON)` — silent referential integrity violations

**What goes wrong:**
SQLite does not enforce foreign key constraints by default. The pragma must be enabled per connection. If `training_targets.user_id` references `users.id` without `PRAGMA foreign_keys=ON`, deleting a user row will not cascade to their training targets, leaving orphaned rows that will appear in future queries for new users who get the same user ID (unlikely but possible with string IDs from the proxy).

**Warning signs:**
- Schema has `FOREIGN KEY` declarations but no `PRAGMA foreign_keys=ON` in the DSN or connection setup.
- Deleting a user row does not delete their training targets.

**Prevention strategy:**
Add `_pragma=foreign_keys(ON)` to the SQLite DSN. Verify with a test that deletes a user and asserts their training targets are also deleted.

**Phase:** Phase 1 (database initialization).

---

### Pitfall 11: mcp-go `WithStateful(true)` — session identity binding

**What goes wrong:**
When `WithStateful(true)` is enabled, the mcp-go library maintains session state across requests on the same connection. The `WithHTTPContextFunc` callback runs on the initial session-establishing request. If the library caches context values at session creation time and reuses them for subsequent tool calls on the same session, a session established for user A that is then reused (e.g., via a stolen or reused session token) will operate as user A regardless of the current request's `X-Forwarded-User` header.

The inverse is also dangerous: if `WithHTTPContextFunc` runs on every request, but the tool handler uses session-level state instead of per-request context, the identity check is bypassed.

**Warning signs:**
- The mcp-go library does not call `WithHTTPContextFunc` on every request, only on session initialization.
- Integration tests with session reuse do not verify that changing `X-Forwarded-User` mid-session is rejected.

**Prevention strategy:**
Verify (via mcp-go source or documentation) whether `WithHTTPContextFunc` runs per-request or per-session. If per-session: validate the session's bound user ID against the current request's `X-Forwarded-User` header in every tool handler entry point, not just at session creation. If per-request: the context is already fresh — rely on the context extractor.

**Phase:** Phase 3 (tool handler implementation). Requires explicit testing with concurrent, multi-session scenarios.

---

## Phase-Specific Warning Summary

| Phase Topic | Most Likely Pitfall | Mitigation |
|-------------|---------------------|------------|
| Server skeleton, middleware chain | Shared-secret check runs after header reading (Pitfall 1) | Middleware order enforced at compile time by wrapping function calls |
| Database schema, encryption layer | Nonce stored in separate column (Pitfall 2) | Single blob column, schema defined before any data written |
| SQLite initialization | Default journal mode, no busy_timeout (Pitfall 5) | DSN pragmas set at DB open time |
| Migration driver selection | sqlite3 (CGO) imported instead of sqlite (pure Go) (Pitfall 6) | Import path audit in CI with CGO_ENABLED=0 |
| OAuth callback | 409 treated as fatal, state not deleted after use (Pitfalls 3, 9) | Explicit 409 handling, state deletion in same transaction as token store |
| Tool handler implementation | No user ID check, global state, unscoped SQL query (Pitfall 4) | Accessor function enforced, SQL always parameterized with user_id |
| Docker image | Missing ca-certificates.crt (Pitfall 7) | Mirror karaclean Dockerfile exactly |
| Configuration / health endpoints | BIND_ADDRESS=0.0.0.0 with no warning (Pitfall 8) | /readyz checks, startup WARN log |
| Session management | WithStateful(true) identity binding not verified (Pitfall 11) | Integration test with session reuse across users |

---

## Sources

- Authelia trusted header SSO documentation: https://www.authelia.com/integration/trusted-header-sso/introduction/
- Authelia forwarded headers: https://www.authelia.com/integration/proxies/forwarded-headers/
- oauth2-proxy CVE-2026-40575 (X-Forwarded-Uri spoofing): https://advisories.gitlab.com/golang/github.com/oauth2-proxy/oauth2-proxy/v7/CVE-2026-40575/
- oauth2-proxy GHSA-7x63-xv5r-3p2x: https://github.com/oauth2-proxy/oauth2-proxy/security/advisories/GHSA-7x63-xv5r-3p2x
- AES-GCM nonce reuse analysis: https://frereit.de/aes_gcm/
- GCM nonce reuse key recovery: https://www.elttam.com/blog/key-recovery-attacks-on-gcm
- Random 96-bit nonce birthday collision (kopia issue): https://github.com/kopia/kopia/issues/5169
- Go crypto/rand GCM proposal: https://github.com/golang/go/issues/69981
- oauth2-proxy multitab race condition: https://github.com/oauth2-proxy/oauth2-proxy/issues/611
- Auth0 OAuth state parameter guide: https://auth0.com/docs/secure/attack-protection/state-parameters
- SQLite WAL concurrency: https://sqlite.org/wal.html
- SQLite SQLITE_BUSY analysis: https://tenthousandmeters.com/blog/sqlite-concurrent-writes-and-database-is-locked-errors/
- turso.tech Go SQLite concurrency: https://turso.tech/blog/something-you-probably-want-to-know-about-if-youre-using-sqlite-in-golang-72547ad625f1
- golang-migrate sqlite (pure Go) driver: https://github.com/golang-migrate/migrate/tree/master/database/sqlite
- golang-migrate implicit transaction issue: https://github.com/golang-migrate/migrate/issues/346
- scratch image TLS certificates: https://medium.com/rewriting-my-notification-service-in-go/handling-tls-in-a-scratch-container-image-39fc0c40bd1f
- scratch image pitfalls: https://labs.iximiuz.com/tutorials/pitfalls-of-from-scratch-images
- MCP 0.0.0.0 drive-by attack: https://www.docker.com/blog/mpc-horror-stories-cve-2025-49596-local-host-breach/
- MCP server security defaults: https://cardinalops.com/blog/mcp-defaults-hidden-dangers-of-remote-deployment/
- Go context key collisions: https://rednafi.com/go/avoid-context-key-collisions/
- mcp-go WithHTTPContextFunc documentation: https://context7.com/mark3labs/mcp-go/llms.txt
