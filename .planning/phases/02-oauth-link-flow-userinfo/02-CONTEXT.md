# Phase 2: OAuth Link Flow + UserInfo - Context

**Gathered:** 2026-05-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement the Polar OAuth2 link flow (login + callback HTTP endpoints) that lets a proxy-authenticated user connect their Polar account, and the `get_user_info` MCP tool that reports which Polar account is linked. No web UI — the browser is only involved during the OAuth redirect round-trip.

Delivers: OAUTH-01 through OAUTH-05, MCP-01.

</domain>

<decisions>
## Implementation Decisions

### OAuth Configuration (Config package)
- **D-01:** Add `POLAR_CLIENT_ID`, `POLAR_CLIENT_SECRET`, and `POLAR_REDIRECT_URL` to the `Config` struct. All three are required at startup — fail-closed in `config.Load()` with actionable error messages (same pattern as `AUTH_PROXY`, `PROXY_SHARED_SECRET`, `ENCRYPTION_KEY`).
- **D-02:** `POLAR_REDIRECT_URL` is set by the operator to match exactly what they registered in the Polar developer portal (e.g., `https://polar.example.com/oauth/callback`). Not derived from the request — operator controls it explicitly.

### OAuth Endpoints
- **D-03:** `GET /oauth/login` reads identity from the proxy identity header (same `cfg.IdentityHeader` / context key pattern as auth middleware), generates a 32-byte `crypto/rand` state token, stores it in `pending_auth` with 10-minute expiry, redirects to Polar's authorization URL. No changes to auth middleware — these endpoints sit behind the existing proxy secret check.
- **D-04:** `GET /oauth/callback` validates state via `DELETE ... RETURNING` (atomic consume-on-first-use — already decided in CLAUDE.md). Checks: present, not expired, identity matches. Exchanges code via `POST https://polarremote.com/v2/oauth2/token` (HTTP Basic with client credentials). Registers user with `POST https://www.polaraccesslink.com/v3/users`; HTTP 409 = idempotent success.
- **D-05:** After successful linking, encrypts token with `internal/crypto` (existing AES-256-GCM) and upserts into `polar_tokens` with `key_version=1`. Re-linking (user already exists) silently overwrites the token — no warning, no block. Upsert handles it.

### Callback Response Format
- **D-06:** Callback returns self-contained inline HTML for both success and error cases — no external dependencies, no JS, inline CSS only. Success: 200 + checkmark + "Polar account linked / You may close this tab". Error cases (state mismatch, expired, identity mismatch, code exchange fail, registration fail): 400 or 500 + clear error message in the same HTML shell. No plain text, no redirects.

### get_user_info MCP Tool
- **D-07:** Returns local DB data only — no live Polar API call. Response includes: `linked` (bool), `polar_user_id` (int64, omitted if not linked), `identity` (proxy identity string). If no account is linked, returns a clear "no Polar account linked" message with a hint to visit `/oauth/login`.

### Store Methods Needed (Phase 2 adds these to internal/store)
- **D-08:** `UpsertUser(ctx, identity, polarUserID)` — insert or update users table
- **D-09:** `UpsertToken(ctx, identity, encryptedBlob []byte, keyVersion int)` — insert or update polar_tokens
- **D-10:** `GetPolarUserID(ctx, identity) (int64, bool, error)` — for get_user_info
- **D-11:** `CreateOAuthState(ctx, state, identity string, expiresAt time.Time) error` — pending_auth insert
- **D-12:** `ConsumeOAuthState(ctx, state string) (identity string, err error)` — DELETE ... RETURNING; returns ErrNotFound or ErrExpired on failure

### Claude's Discretion
- Error message copy for HTML error pages (wording is implementation detail)
- HTML/CSS styling for the callback page (minimal, self-contained)
- Whether to use `golang.org/x/oauth2` convenience types or raw `net/http` for token exchange (either is fine given tokens don't refresh)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project constraints and prior decisions
- `.planning/PROJECT.md` — key decisions table (locked decisions from Phase 1), tech stack, constraints
- `.planning/REQUIREMENTS.md` — OAUTH-01..05 and MCP-01 (the full requirement text)
- `CLAUDE.md` — critical decisions table; `DELETE ... RETURNING` for OAuth state, CGO disabled, driver import paths

### Polar AccessLink API
- OAuth2 endpoints (from PROJECT.md Context section):
  - Auth URL: `https://flow.polar.com/oauth2/authorization` (scope: `accesslink.read_all`)
  - Token URL: `https://polarremote.com/v2/oauth2/token` (HTTP Basic auth)
  - User registration: `POST https://www.polaraccesslink.com/v3/users` (409 = idempotent)
- No external API spec file — shapes documented in PROJECT.md and REQUIREMENTS.md

### Existing code to read before planning
- `internal/config/config.go` — Config struct and Load() pattern to extend with Polar fields
- `internal/crypto/crypto.go` — Encrypt/Decrypt API (nonce||ciphertext BLOB)
- `internal/store/store.go` — Store struct, dual pool, existing method signatures
- `internal/auth/auth.go` — identity header extraction pattern (context key type)
- `internal/oauth/oauth.go` — stubs to fill in
- `internal/polar/client.go` — Client stub to extend with registration + token exchange methods
- `internal/mcp/mcp.go` — RegisterTools stub to extend with get_user_info

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/crypto`: `Encrypt(key, plaintext []byte) ([]byte, error)` and `Decrypt` — ready to use for token storage; nonce||ciphertext layout already established
- `internal/store.Store`: `WriteDB()` / `ReadDB()` accessors for adding new query methods; dual-pool pattern to follow
- `internal/config.Config`: `Load()` pattern — add `PolarClientID`, `PolarClientSecret`, `PolarRedirectURL` fields using the same env-var + validation structure
- `internal/polar.Client`: `NewClient(bearerToken)` stub — extend with `RegisterUser()` and `ExchangeCode()` methods

### Established Patterns
- Identity in context: unexported struct type as context key (established in auth middleware) — `get_user_info` handler reads identity the same way
- Fail-closed config: all required fields validated in `Load()`, server refuses to start — three new Polar fields follow this exactly
- `subtle.ConstantTimeCompare` is already used for the proxy secret; OAuth state comparison uses `crypto/rand` + constant-time safe string lookup (DELETE ... RETURNING avoids timing issues)
- WAL dual-pool: write queries use `s.writeDB`, read queries use `s.readDB`

### Integration Points
- `cmd/polar-flow-mcp/main.go` — register `/oauth/login` and `/oauth/callback` routes on the existing mux, after auth middleware
- `internal/mcp/mcp.go` `RegisterTools()` — add `get_user_info` tool here
- `internal/store/store.go` — new methods added to `*Store` receiver (same file or split into `store_users.go`, `store_tokens.go`, `store_oauth.go`)

</code_context>

<specifics>
## Specific Ideas

- Callback HTML success page: checkmark (✔), heading "Polar account linked", body "You may close this tab and return to Claude." — minimal inline CSS, no external deps.
- Error pages: same HTML shell, different heading/message, appropriate HTTP status code (400 for client errors, 500 for server/Polar API errors).
- `get_user_info` response when not linked: include a hint like "Visit /oauth/login to link your Polar account."
- `polar_user_id` returned as integer (matches Polar API's numeric user ID).

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 2-OAuth Link Flow + UserInfo*
*Context gathered: 2026-05-05*
