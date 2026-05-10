# Phase 2: OAuth Link Flow + UserInfo - Research

**Researched:** 2026-05-05
**Domain:** Polar AccessLink OAuth2, golang.org/x/oauth2, modernc.org/sqlite DELETE...RETURNING, mcp-go tool registration
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Add `POLAR_CLIENT_ID`, `POLAR_CLIENT_SECRET`, and `POLAR_REDIRECT_URL` to the `Config` struct. All three are required at startup — fail-closed in `config.Load()` with actionable error messages.
- **D-02:** `POLAR_REDIRECT_URL` is set by the operator (not derived from the request).
- **D-03:** `GET /oauth/login` reads identity from proxy header, generates 32-byte `crypto/rand` state, stores in `pending_auth` with 10-min expiry, redirects to Polar auth URL with `scope=accesslink.read_all`.
- **D-04:** `GET /oauth/callback` validates state via `DELETE ... RETURNING` (atomic consume-on-first-use). Checks: present, not expired, identity matches. Exchanges code via `POST https://polarremote.com/v2/oauth2/token` (HTTP Basic with client credentials). Registers user with `POST https://www.polaraccesslink.com/v3/users`; HTTP 409 = idempotent success.
- **D-05:** After linking, encrypts token with `internal/crypto` (AES-256-GCM) and upserts into `polar_tokens` with `key_version=1`. Re-linking silently overwrites.
- **D-06:** Callback returns self-contained inline HTML for both success (200) and error (400/500) — no external deps, no JS, inline CSS only.
- **D-07:** `get_user_info` returns local DB data only — no live Polar API call. Returns: `linked` (bool), `polar_user_id` (int64, omitted if not linked), `identity` (proxy identity string). Clear "no account linked" + hint to visit `/oauth/login` if not linked.
- **D-08:** `UpsertUser(ctx, identity, polarUserID)` — insert or update users table.
- **D-09:** `UpsertToken(ctx, identity, encryptedBlob []byte, keyVersion int)` — insert or update polar_tokens.
- **D-10:** `GetPolarUserID(ctx, identity) (int64, bool, error)` — for get_user_info.
- **D-11:** `CreateOAuthState(ctx, state, identity string, expiresAt time.Time) error` — pending_auth insert.
- **D-12:** `ConsumeOAuthState(ctx, state string) (identity string, err error)` — DELETE...RETURNING; returns ErrNotFound or ErrExpired on failure.

### Claude's Discretion

- Error message copy for HTML error pages (wording is implementation detail).
- HTML/CSS styling for the callback page (minimal, self-contained).
- Whether to use `golang.org/x/oauth2` convenience types or raw `net/http` for token exchange (either is fine given tokens don't refresh).

### Deferred Ideas (OUT OF SCOPE)

None.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| OAUTH-01 | `GET /oauth/login`: read identity, generate 32-byte CSRF state, store in `pending_auth` (10-min expiry), redirect to Polar auth URL | D-03, D-11; Polar auth URL + scope confirmed |
| OAUTH-02 | `GET /oauth/callback`: validate state (present, not expired, identity match), delete state row (single-use) | D-04, D-12; DELETE...RETURNING pattern confirmed for modernc.org/sqlite |
| OAUTH-03 | Callback exchanges code for Polar access token via `POST https://polarremote.com/v2/oauth2/token` with HTTP Basic auth | Token exchange format verified; raw net/http recommended over oauth2 library |
| OAUTH-04 | Callback registers user with Polar via `POST https://www.polaraccesslink.com/v3/users`; HTTP 409 = idempotent | User registration response format verified; `polar-user-id` field confirmed |
| OAUTH-05 | Callback encrypts token + upserts into `polar_tokens` with `key_version=1`; success = 200 HTML | crypto.Cipher.Encrypt API confirmed; upsert pattern via ON CONFLICT |
| MCP-01 | `get_user_info` tool: returns `linked`, `polar_user_id`, `identity` from DB; clear message if not linked | mcp-go AddTool + mcp.NewToolResultText pattern confirmed |
</phase_requirements>

---

## Summary

Phase 2 implements three tightly coupled sub-systems: (1) the Polar OAuth2 link flow — a pair of HTTP handlers on routes already registered as stubs in `main.go`; (2) five new store methods on `*Store` for CSRF state management and user/token persistence; and (3) the `get_user_info` MCP tool. All three build on the Phase 1 foundation without requiring any new third-party dependencies — `golang.org/x/oauth2` is listed in CLAUDE.md's tech stack but is not yet in `go.mod`, and after analysis it is simpler to do the token exchange with raw `net/http` + HTTP Basic auth rather than through the library's `oauth2.Config.Exchange` path (which adds complexity for an endpoint that requires non-standard auth and whose tokens never refresh).

The Polar AccessLink token endpoint (`https://polarremote.com/v2/oauth2/token`) uses HTTP Basic auth with `client_id:client_secret` as credentials, form-encoded body, and returns a JSON object with `access_token` and `x_user_id`. The user registration endpoint (`POST /v3/users`) uses a JSON body of `{"member-id": "<access_token>"}` — the access token itself is the member identifier — and returns a 200 with a full user object including `polar-user-id` (an integer) on first registration, or 409 if already registered (idempotent success). `DELETE ... RETURNING` works in modernc.org/sqlite and is the correct atomic consume-on-first-use pattern for the CSRF state table.

**Primary recommendation:** Use raw `net/http` for the Polar token exchange (simpler, no library overhead, no token-refresh concern). Split new store methods across `store_oauth.go`, `store_users.go`, and `store_tokens.go` (same package, `*Store` receiver). Register `get_user_info` with `s.AddTool(tool, handler)` in `mcp.RegisterTools()` following the established mcp-go builder pattern.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| CSRF state generation + redirect | API / Backend (`internal/oauth`) | — | Server-side only; browser never sees state value directly |
| CSRF state validation + consume | Database / Storage (`internal/store`) | API / Backend | Atomic DELETE...RETURNING must happen in store layer |
| Polar token exchange | API / Backend (`internal/polar`) | — | HTTP call to external service; client secret must stay server-side |
| User registration with Polar | API / Backend (`internal/polar`) | — | Requires bearer token; server-side only |
| Token encryption + persistence | Database / Storage (`internal/store` + `internal/crypto`) | — | Encrypt before write; never expose plaintext |
| `get_user_info` tool handler | API / Backend (`internal/mcp`) | Database / Storage | Reads identity from ctx; queries store; no Polar API call |
| OAuth route wiring | API / Backend (`cmd/.../main.go`) | — | Already stubbed; Phase 2 fills `internal/oauth` handlers |

---

## Standard Stack

### Core (already in go.mod — no new deps required)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `net/http` (stdlib) | Go 1.26 | Polar token exchange + user registration HTTP calls | Simpler than oauth2 lib for non-refreshing tokens; no extra dep |
| `crypto/rand` (stdlib) | Go 1.26 | 32-byte CSRF state generation | Already used in crypto package; standard |
| `encoding/json` (stdlib) | Go 1.26 | Decode Polar API responses | No external dep needed |
| `encoding/base64` (stdlib) | Go 1.26 | HTTP Basic auth credential encoding | Standard; no extra dep |
| `modernc.org/sqlite` | v1.50.0 | DELETE...RETURNING for atomic state consume | Already present; verified CGO-free |
| `github.com/mark3labs/mcp-go` | v0.50.0 | `s.AddTool()` + `mcp.NewToolResultText` | Already present |
| `internal/crypto` (local) | — | AES-256-GCM token encryption | Phase 1 deliverable; `Encrypt(plaintext []byte)` API confirmed |
| `internal/store` (local) | — | New methods on `*Store` receiver | Dual-pool pattern established |

### golang.org/x/oauth2 — Not Needed for Phase 2

`golang.org/x/oauth2` was in the original CLAUDE.md tech stack listing. After research, it is NOT needed for Phase 2 and should NOT be added to `go.mod`:

- Its `Exchange()` method uses `oauth2.Config` and manages token refresh, which is irrelevant (Polar tokens don't expire unless revoked).
- The Polar token endpoint requires HTTP Basic auth (`AuthStyleInHeader` in oauth2 terms), which works but adds boilerplate configuration compared to a 15-line `net/http.PostForm` with `Authorization: Basic ...`.
- The `oauth2.Config.AuthCodeURL()` convenience for building the redirect URL is replicated in 3 lines of `url.Values` + `url.URL`.
- Verdict: add to `go.mod` only if Phase 3 needs it for token transport; skip in Phase 2. [VERIFIED: pkg.go.dev/golang.org/x/oauth2]

**Installation:** No new `go get` required for Phase 2 — all dependencies already present.

---

## Architecture Patterns

### System Architecture Diagram

```
Browser (behind proxy)
    │
    │ GET /oauth/login  (proxy injects identity header)
    ▼
auth.Middleware ──► oauth.LoginHandler
                        │ 1. auth.UserIDFromContext(ctx) → identity
                        │ 2. crypto/rand → 32-byte state hex
                        │ 3. store.CreateOAuthState(state, identity, now+10min)
                        │ 4. build redirect URL: flow.polar.com/oauth2/authorization
                        │    ?response_type=code&client_id=...&redirect_uri=...
                        │    &scope=accesslink.read_all&state=<hex>
                        └──► http.Redirect(302) → Polar
                                                       │
                                                       │ user grants access
                                                       ▼
                                              Polar → GET /oauth/callback?code=...&state=...
                                                       │
auth.Middleware ──► oauth.CallbackHandler
                        │ 1. r.URL.Query().Get("state")
                        │ 2. store.ConsumeOAuthState(state) → identity / ErrNotFound / ErrExpired
                        │ 3. verify returned identity == current request identity
                        │ 4. polar.ExchangeCode(code) → accessToken, polarUserID (x_user_id)
                        │    POST https://polarremote.com/v2/oauth2/token
                        │    Authorization: Basic base64(clientID:clientSecret)
                        │    body: grant_type=authorization_code&code=...&redirect_uri=...
                        │ 5. polar.RegisterUser(accessToken) → polarUserID (200 or 409)
                        │    POST https://www.polaraccesslink.com/v3/users
                        │    Authorization: Bearer <accessToken>
                        │    body: {"member-id": "<accessToken>"}
                        │ 6. store.UpsertUser(identity, polarUserID)
                        │ 7. crypto.Encrypt([]byte(accessToken)) → blob
                        │ 8. store.UpsertToken(identity, blob, keyVersion=1)
                        └──► htmlSuccess(w) 200 or htmlError(w, status, msg)

MCP tool call: get_user_info
    │
    ▼
mcp.RegisterTools → s.AddTool("get_user_info", handler)
    handler:
        │ 1. auth.UserIDFromContext(ctx) → identity
        │ 2. store.GetPolarUserID(identity) → (id, found, err)
        └──► mcp.NewToolResultText(JSON: {linked, polar_user_id, identity})
```

### Recommended File Layout for Phase 2

```
internal/
├── config/
│   └── config.go          ← add PolarClientID, PolarClientSecret, PolarRedirectURL fields + validation
├── store/
│   ├── store.go            ← unchanged (dual-pool, migrations)
│   ├── store_oauth.go      ← CreateOAuthState, ConsumeOAuthState (+ sentinel errors)
│   ├── store_users.go      ← UpsertUser, GetPolarUserID
│   └── store_tokens.go     ← UpsertToken
├── polar/
│   └── client.go           ← add ExchangeCode, RegisterUser methods (raw net/http)
├── oauth/
│   └── oauth.go            ← LoginHandler, CallbackHandler (replace stubs)
└── mcp/
    └── mcp.go              ← RegisterTools adds get_user_info via s.AddTool
```

### Pattern 1: Config Extension — Fail-Closed Required Fields

Follow exactly the same validation structure in `loadAuthFields()`:

```go
// Source: internal/config/config.go (existing pattern)
func loadPolarFields(cfg *Config) error {
    clientID := os.Getenv("POLAR_CLIENT_ID")
    if clientID == "" {
        return errors.New(
            "POLAR_CLIENT_ID must be set to your Polar developer client ID; " +
                "see https://github.com/lm/polar-flow-mcp/docs/deployment/polar-oauth-setup",
        )
    }
    cfg.PolarClientID = clientID

    clientSecret := os.Getenv("POLAR_CLIENT_SECRET")
    if clientSecret == "" {
        return errors.New("POLAR_CLIENT_SECRET must be set; see deployment docs")
    }
    cfg.PolarClientSecret = clientSecret

    redirectURL := os.Getenv("POLAR_REDIRECT_URL")
    if redirectURL == "" {
        return errors.New(
            "POLAR_REDIRECT_URL must be set to the callback URL registered in the Polar developer portal " +
                "(e.g., https://polar.example.com/oauth/callback)",
        )
    }
    cfg.PolarRedirectURL = redirectURL

    return nil
}
```

Call `loadPolarFields(cfg)` from `Load()` after `loadAuthFields`. [ASSUMED — exact wording of function split; the pattern itself is VERIFIED from existing code]

### Pattern 2: CSRF State Generation and Storage

```go
// Source: CLAUDE.md decision + crypto/rand stdlib docs (VERIFIED)
// In oauth.LoginHandler:
stateBytes := make([]byte, 32)
if _, err := io.ReadFull(rand.Reader, stateBytes); err != nil {
    // 500 — CSPRNG failure is unrecoverable
    htmlError(w, http.StatusInternalServerError, "failed to generate state")
    return
}
state := hex.EncodeToString(stateBytes) // 64-char hex string
expiresAt := time.Now().UTC().Add(10 * time.Minute)
if err := st.CreateOAuthState(r.Context(), state, identity, expiresAt); err != nil {
    htmlError(w, http.StatusInternalServerError, "failed to store state")
    return
}
```

### Pattern 3: DELETE...RETURNING in modernc.org/sqlite

`DELETE ... RETURNING` is supported in SQLite 3.35+ (2021-03-12). `modernc.org/sqlite` v1.50.0 bundles SQLite 3.49.x, so RETURNING is available. [VERIFIED: modernc.org/sqlite changelog; SQLite 3.35 release notes]

```go
// Source: SQLite docs + pattern (VERIFIED: https://www.sqlite.org/lang_returning.html)
// In store_oauth.go:
func (s *Store) ConsumeOAuthState(ctx context.Context, state string) (string, error) {
    var identity string
    var expiresAt time.Time
    err := s.writeDB.QueryRowContext(ctx,
        `DELETE FROM pending_auth WHERE state = ? RETURNING identity, expires_at`,
        state,
    ).Scan(&identity, &expiresAt)
    if errors.Is(err, sql.ErrNoRows) {
        return "", ErrNotFound
    }
    if err != nil {
        return "", fmt.Errorf("store: consume oauth state: %w", err)
    }
    if time.Now().UTC().After(expiresAt) {
        return "", ErrExpired
    }
    return identity, nil
}
```

Two sentinel errors must be defined in the store package (exportable, comparable with `errors.Is`):

```go
var (
    ErrNotFound = errors.New("store: record not found")
    ErrExpired  = errors.New("store: record expired")
)
```

[ASSUMED — sentinel error names; the pattern is VERIFIED]

### Pattern 4: Upsert in SQLite (users and polar_tokens)

```sql
-- users table: upsert by identity (UNIQUE constraint present in schema)
INSERT INTO users (identity, polar_user_id, created_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT(identity) DO UPDATE SET polar_user_id = excluded.polar_user_id;

-- polar_tokens table: upsert by user_id FK (UNIQUE constraint on user_id)
INSERT INTO polar_tokens (user_id, encrypted_token, key_version, updated_at)
VALUES ((SELECT id FROM users WHERE identity = ?), ?, ?, datetime('now'))
ON CONFLICT(user_id) DO UPDATE SET
    encrypted_token = excluded.encrypted_token,
    key_version     = excluded.key_version,
    updated_at      = datetime('now');
```

`ON CONFLICT DO UPDATE` (UPSERT) is supported in SQLite 3.24+ (2018). [VERIFIED: https://www.sqlite.org/lang_UPSERT.html]

Use `writeDB` for both inserts. Use `readDB` for `GetPolarUserID` (`SELECT`).

### Pattern 5: Polar Token Exchange — Raw net/http

```go
// Source: Polar AccessLink API docs (VERIFIED: https://www.polar.com/accesslink-api/#oauth-2-0)
// In internal/polar/client.go:

type TokenResponse struct {
    AccessToken string `json:"access_token"`
    TokenType   string `json:"token_type"`
    XUserID     int64  `json:"x_user_id"`
}

func ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURL string) (*TokenResponse, error) {
    data := url.Values{
        "grant_type":   {"authorization_code"},
        "code":         {code},
        "redirect_uri": {redirectURL},
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        "https://polarremote.com/v2/oauth2/token",
        strings.NewReader(data.Encode()),
    )
    if err != nil {
        return nil, fmt.Errorf("polar: build token request: %w", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("Accept", "application/json;charset=UTF-8")
    req.SetBasicAuth(clientID, clientSecret)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("polar: token exchange: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return nil, fmt.Errorf("polar: token exchange: status %d: %s", resp.StatusCode, body)
    }
    var tr TokenResponse
    if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
        return nil, fmt.Errorf("polar: decode token response: %w", err)
    }
    return &tr, nil
}
```

**Important:** Use `http.NewRequestWithContext` (not `http.NewRequest`) to satisfy the `noctx` golangci-lint rule. The project's linter config includes `noctx`. [VERIFIED: CLAUDE.md lint rules]

**Note on `polar.Client`:** The existing `NewClient(bearerToken)` stub is designed for bearer-token-authenticated calls (Phase 3 training targets). `ExchangeCode` does not need a `*Client` instance — it is a package-level function taking credentials as parameters, or a method on a config struct. This is a planner decision, but a top-level function is simpler. [ASSUMED — exact API shape; the HTTP call format is VERIFIED]

### Pattern 6: Polar User Registration

```go
// Source: Polar AccessLink API docs (VERIFIED: https://www.polar.com/accesslink-api/#tag/Users/operation/registerUser)

type UserRegistrationResponse struct {
    PolarUserID int64  `json:"polar-user-id"`
    MemberID    string `json:"member-id"`
    // other fields omitted — only polar-user-id is needed
}

func RegisterUser(ctx context.Context, accessToken string) (int64, error) {
    body, _ := json.Marshal(map[string]string{"member-id": accessToken})
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        "https://www.polaraccesslink.com/v3/users",
        bytes.NewReader(body),
    )
    if err != nil {
        return 0, fmt.Errorf("polar: build register request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json")
    req.Header.Set("Authorization", "Bearer "+accessToken)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return 0, fmt.Errorf("polar: register user: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusConflict { // 409 = already registered (D-04)
        // User already registered — idempotent success.
        // polar_user_id must come from the token exchange x_user_id field.
        return 0, nil // signal: use x_user_id from token response
    }
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return 0, fmt.Errorf("polar: register user: status %d: %s", resp.StatusCode, body)
    }
    var reg UserRegistrationResponse
    if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
        return 0, fmt.Errorf("polar: decode register response: %w", err)
    }
    return reg.PolarUserID, nil
}
```

**409 handling detail:** When 409 is returned, the `polar-user-id` is NOT in the response body. The planner must arrange to use `x_user_id` from the token exchange response as the authoritative user ID in all cases — both first registration and re-link. [VERIFIED: Polar API docs — 409 = "User already registered to partner"]

### Pattern 7: mcp-go Tool Registration

```go
// Source: github.com/mark3labs/mcp-go README + docs (VERIFIED via Context7)
// In internal/mcp/mcp.go:

func RegisterTools(s *server.MCPServer, st *store.Store) {
    getUserInfoTool := mcp.NewTool("get_user_info",
        mcp.WithDescription("Returns the Polar account linked to your identity. "+
            "If no account is linked, returns instructions to link one."),
    )
    s.AddTool(getUserInfoTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        identity, ok := auth.UserIDFromContext(ctx)
        if !ok {
            return mcp.NewToolResultError("no identity in context — auth middleware not applied"), nil
        }
        polarID, found, err := st.GetPolarUserID(ctx, identity)
        if err != nil {
            return mcp.NewToolResultError("database error: " + err.Error()), nil
        }
        if !found {
            return mcp.NewToolResultText(
                "No Polar account is linked to your identity (" + identity + "). " +
                    "Visit /oauth/login to link your Polar account.",
            ), nil
        }
        return mcp.NewToolResultText(fmt.Sprintf(
            "Polar account linked.\nIdentity: %s\nPolar user ID: %d",
            identity, polarID,
        )), nil
    })
}
```

**Signature change:** `RegisterTools` currently takes only `*server.MCPServer`. Phase 2 adds `*store.Store` as a second parameter so the handler can query the DB. Update the call site in `main.go`. [ASSUMED — exact parameter addition; the mcp-go API is VERIFIED]

### Pattern 8: Inline HTML Response

```go
// Source: Go net/http stdlib docs (VERIFIED)
func htmlSuccess(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Polar Linked</title>
<style>body{font-family:sans-serif;text-align:center;padding:4rem}
.check{font-size:4rem;color:#2ecc71}</style></head>
<body><div class="check">&#10004;</div>
<h1>Polar account linked</h1>
<p>You may close this tab and return to Claude.</p>
</body></html>`)
}

func htmlError(w http.ResponseWriter, status int, msg string) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(status)
    fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Error</title>
<style>body{font-family:sans-serif;text-align:center;padding:4rem}
.x{font-size:4rem;color:#e74c3c}</style></head>
<body><div class="x">&#10008;</div>
<h1>Link failed</h1><p>%s</p>
</body></html>`, html.EscapeString(msg))
}
```

Use `html.EscapeString` from `html` stdlib package to escape any user-visible error text before embedding in HTML. [VERIFIED: Go stdlib docs]

### Pattern 9: Route Wiring in main.go (already done in Phase 1)

Phase 1 already registered `/oauth/login` and `/oauth/callback` as stubs (lines 93–98 of `main.go`). Phase 2 replaces the `oauth.LoginHandler` and `oauth.CallbackHandler` function bodies; no changes to `main.go` route registration are required beyond passing `cfg` and `st` to a new constructor (or passing them directly to the handler functions).

**Dependency injection options (planner chooses one):**
- Option A: `oauth.NewHandlers(cfg, st, cipher)` returns a struct with `Login` and `Callback` methods. Main.go calls `h := oauth.NewHandlers(...)` then `http.HandlerFunc(h.Login)`.
- Option B: Package-level `LoginHandler` / `CallbackHandler` become closures returned by `oauth.NewLoginHandler(cfg, st, cipher)`. Main.go replaces the stub references.

Both follow established Go patterns. Option A is cleaner for testability. [ASSUMED — which option the planner prefers; both are VERIFIED patterns in Go]

### Anti-Patterns to Avoid

- **Building the auth URL with string concatenation:** Use `url.Values` + `url.URL.String()` to avoid encoding bugs with special characters in state or redirect URL.
- **Using `http.NewRequest` without context:** The `noctx` linter rule will reject it. Always use `http.NewRequestWithContext(ctx, ...)`.
- **Using `http.DefaultClient` in production without a timeout:** The existing `polar.Client` sets a 30-second timeout. For `ExchangeCode` and `RegisterUser`, either use a package-level `*http.Client` with timeout set at init, or use `polar.Client`'s `httpClient` field.
- **In-memory CSRF state map:** Already decided against (CLAUDE.md). `pending_auth` in SQLite is the only valid location.
- **Using mattn/go-sqlite3 or `sqlite3` migrate driver:** CGO disabled; must use `modernc.org/sqlite` + `database/sqlite` migrate driver.
- **Caching the decrypted token:** The crypto package design decision is "decrypt on demand, never cache." Phase 2 doesn't decrypt tokens — just encrypts and stores.
- **Reading identity header directly in oauth handlers:** Must use `auth.UserIDFromContext(r.Context())` — the identity was injected by `auth.Middleware` which already validated the proxy secret. Direct header reads bypass the secret check ordering constraint.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| AES-256-GCM encryption | Custom encryption | `internal/crypto.Cipher.Encrypt` | Phase 1 deliverable; nonce uniqueness + BLOB layout already correct |
| HTTP Basic auth encoding | Manual base64 | `req.SetBasicAuth(id, secret)` | stdlib method handles encoding correctly |
| HTML escaping in error pages | Manual replacement | `html.EscapeString(msg)` | Avoids XSS if error messages contain user-controlled data |
| CSRF state | Custom session store | `pending_auth` SQLite table + `DELETE...RETURNING` | Restart-safe; atomic consume-on-first-use |
| Token refresh | Token refresh loop | Nothing — tokens don't expire | Polar tokens are long-lived; no refresh needed in v1 |
| URL building for auth redirect | String concat | `url.Values.Encode()` + `url.URL` | Handles special chars, percent-encoding |

**Key insight:** All cryptographic and storage primitives are already built. Phase 2 is integration, not new infrastructure.

---

## Common Pitfalls

### Pitfall 1: polar_user_id on 409 Registration Response

**What goes wrong:** Code tries to decode `polar-user-id` from the 409 response body. Polar returns an empty body (or a conflict message) on 409, not the user object.

**Why it happens:** The 409 response only says "already registered." It does not repeat the user object.

**How to avoid:** Always use `x_user_id` from the token exchange response (`TokenResponse.XUserID`) as the authoritative Polar user ID, regardless of whether registration returned 200 or 409.

**Warning signs:** `polar_user_id` column in DB is 0 or NULL after a re-link.

### Pitfall 2: Identity Mismatch Check After ConsumeOAuthState

**What goes wrong:** The state is consumed (row deleted) before the identity check. If identity doesn't match, the state has already been consumed and the user must re-authorize.

**Why it happens:** `DELETE...RETURNING` atomically deletes and returns. The identity is in the returned row.

**How to avoid:** This is correct behavior — state is single-use. The identity check uses the `identity` value returned by `ConsumeOAuthState`, not a separate query. If identity doesn't match, log a warning (possible CSRF attempt) and return 400. The consumed state cannot be replayed.

**Code pattern:**
```go
identity, err := st.ConsumeOAuthState(ctx, state)
if errors.Is(err, store.ErrNotFound) {
    htmlError(w, http.StatusBadRequest, "invalid or expired authorization state")
    return
}
if errors.Is(err, store.ErrExpired) {
    htmlError(w, http.StatusBadRequest, "authorization session expired — please try again")
    return
}
if err != nil {
    htmlError(w, http.StatusInternalServerError, "state validation error")
    return
}
// Identity from state must match current request's identity (CSRF check)
currentIdentity, _ := auth.UserIDFromContext(r.Context())
if identity != currentIdentity {
    slog.Warn("oauth callback identity mismatch — possible CSRF",
        "state_identity", identity, "request_identity", currentIdentity)
    htmlError(w, http.StatusBadRequest, "identity mismatch")
    return
}
```

[VERIFIED: security reasoning; SQLite RETURNING behavior]

### Pitfall 3: noctx Linter on http.NewRequest

**What goes wrong:** `http.NewRequest(...)` without context → `noctx` lint error → CI fails.

**Why it happens:** The `noctx` linter rule in golangci-lint v2.11 (project config) disallows context-less HTTP requests.

**How to avoid:** Every `http.NewRequest` call must be `http.NewRequestWithContext(ctx, method, url, body)`. The `ctx` comes from the HTTP handler's `r.Context()` in `CallbackHandler`.

**Warning signs:** `golangci-lint run ./...` reports `noctx: http.NewRequest` violation.

### Pitfall 4: Shared in-memory DSN for Store Tests

**What goes wrong:** Using `:memory:` DSN in tests creates two isolated databases (write pool ≠ read pool). Data written via `writeDB` is invisible to `readDB` queries.

**Why it happens:** Each `sql.Open(":memory:")` creates a new database. The dual-pool design requires the same physical database.

**How to avoid:** Use `fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())` — established in existing `store_test.go`. All new store tests must follow this pattern. [VERIFIED: existing store_test.go openForTest helper]

### Pitfall 5: UpsertToken Requires users Row First

**What goes wrong:** `INSERT INTO polar_tokens ... VALUES ((SELECT id FROM users WHERE identity = ?), ...)` fails or inserts NULL for `user_id` if no user row exists yet.

**Why it happens:** The `polar_tokens.user_id` column has a `NOT NULL` constraint and a FK to `users(id)`. The upsert order matters.

**How to avoid:** Always call `UpsertUser` before `UpsertToken` in the callback handler. This is the correct sequence regardless — user must exist before token. [VERIFIED: schema from 001_initial.up.sql]

### Pitfall 6: State Expiry Check After DELETE

**What goes wrong:** Checking expiry before deleting creates a TOCTOU window: two concurrent requests with the same state could both pass the expiry check before either deletes.

**Why it happens:** SELECT then DELETE is not atomic.

**How to avoid:** The `DELETE...RETURNING` approach deletes unconditionally and returns the row (including `expires_at`). Check expiry on the returned data. This is already the design in D-12 and ConsumeOAuthState pattern above. No additional protection needed. [VERIFIED: SQLite RETURNING semantics; CLAUDE.md decision]

### Pitfall 7: errcheck Lint on Type Assertions

**What goes wrong:** Bare type assertions like `v := ctx.Value(key).(string)` fail errcheck lint if the assertion can fail silently.

**Why it happens:** golangci-lint v2.11 with `errcheck` enabled flags unchecked type assertions.

**How to avoid:** Use the two-value form: `v, ok := ctx.Value(key).(string)`. Already established in `auth.UserIDFromContext`. Tool handlers must use the same pattern. [VERIFIED: existing auth.go + CLAUDE.md lint config]

---

## Polar AccessLink API — Verified Shapes

### Token Exchange

**Endpoint:** `POST https://polarremote.com/v2/oauth2/token`
[VERIFIED: https://www.polar.com/accesslink-api/#oauth-2-0]

**Request:**
```
Authorization: Basic base64(clientID + ":" + clientSecret)
Content-Type: application/x-www-form-urlencoded
Accept: application/json;charset=UTF-8

grant_type=authorization_code&code=<code>&redirect_uri=<redirectURL>
```

**Response (200):**
```json
{
  "access_token": "2YotnFZFEjr1zCsicMWpAA",
  "token_type": "bearer",
  "expires_in": 31535999,
  "x_user_id": 10579
}
```

Key fields: `access_token` (string, the bearer token to encrypt and store), `x_user_id` (int64, the Polar user identifier — use this as `polar_user_id` in all cases).

### User Registration

**Endpoint:** `POST https://www.polaraccesslink.com/v3/users`
[VERIFIED: https://www.polar.com/accesslink-api/#tag/Users/operation/registerUser]

**Request:**
```
Authorization: Bearer <access_token>
Content-Type: application/json
Accept: application/json

{"member-id": "<access_token>"}
```

Note: the `member-id` field is documented as "partner's custom user identifier." The official Python example uses the access token as the member ID. This is the canonical pattern. [CITED: https://github.com/polarofficial/accesslink-example-python]

**Response (200):**
```json
{
  "polar-user-id": 2278512,
  "member-id": "i09u9ujj",
  "registration-date": "2011-10-14T12:50:37.000Z",
  "first-name": "Eka",
  "last-name": "Toka",
  ...
}
```

**Response (409):** Conflict — "User already registered to partner or duplicated member-id." Body is not the user object. Use `x_user_id` from token exchange in this case.

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes (OAuth flow) | Proxy contract + CSRF state; no credential storage |
| V3 Session Management | yes (OAuth state token) | 32-byte CSPRNG state; single-use via DELETE...RETURNING; 10-min TTL |
| V4 Access Control | yes | `auth.Middleware` (proxy secret + identity) gates all OAuth routes |
| V5 Input Validation | yes | `code` and `state` are URL params — only used in DB lookup and forwarded to Polar; never reflected to user in raw form |
| V6 Cryptography | yes (token storage) | AES-256-GCM; CSPRNG nonce; `internal/crypto` — do not hand-roll |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| CSRF on callback | Spoofing | 32-byte CSPRNG state + identity-bound + DELETE...RETURNING atomic consume |
| State replay | Spoofing | Single-use: DELETE...RETURNING removes on first use |
| State fixation (attacker sets state) | Spoofing | State generated server-side via CSPRNG; user never sets it |
| Token leakage via HTML error page | Information Disclosure | Never reflect access_token or code in HTML response; log errors server-side only |
| Timing oracle on state lookup | Spoofing | DELETE...RETURNING path: presence/absence always produces the same code path; no timing difference on hit vs miss beyond DB latency (not exploitable) |
| Identity mismatch (cross-user linking) | Elevation of Privilege | ConsumeOAuthState returns identity bound at login time; callback verifies it matches current request identity |

**CSRF state security assessment:** 32 bytes from `crypto/rand` = 256 bits of entropy. A brute-force attack would require 2^128 expected attempts to find a valid state with 50% probability. Combined with the 10-minute TTL and single-use deletion, this is adequate. No additional `subtle.ConstantTimeCompare` is needed for the state lookup because: (a) the lookup is a DB primary key match (not a timing-sensitive string comparison) and (b) `DELETE...RETURNING` fails with `ErrNoRows` on miss regardless of the state value — there is no timing difference between a wrong state and a missing state. [VERIFIED: security reasoning]

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + `net/http/httptest` |
| Config file | none (Go tests are discovered automatically) |
| Quick run command | `CGO_ENABLED=0 go test -count=1 ./internal/store/... ./internal/polar/... ./internal/oauth/... ./internal/mcp/...` |
| Full suite command | `CGO_ENABLED=0 go test -race -count=1 ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| OAUTH-01 | `/oauth/login` redirects to Polar with state param | httptest | `go test ./internal/oauth/...` | ❌ Wave 0 |
| OAUTH-01 | State stored in `pending_auth` with correct expiry | unit (store) | `go test ./internal/store/...` | ❌ Wave 0 |
| OAUTH-02 | Valid state consumed, returns identity | unit (store) | `go test ./internal/store/...` | ❌ Wave 0 |
| OAUTH-02 | Missing state → ErrNotFound | unit (store) | `go test ./internal/store/...` | ❌ Wave 0 |
| OAUTH-02 | Expired state → ErrExpired (row deleted) | unit (store) | `go test ./internal/store/...` | ❌ Wave 0 |
| OAUTH-02 | Identity mismatch → 400 from callback | httptest | `go test ./internal/oauth/...` | ❌ Wave 0 |
| OAUTH-03 | Code exchange success → access_token extracted | unit (polar) | `go test ./internal/polar/...` | ❌ Wave 0 |
| OAUTH-03 | Code exchange error (non-200 from Polar) → error returned | unit (polar) | `go test ./internal/polar/...` | ❌ Wave 0 |
| OAUTH-04 | Registration success → polar_user_id extracted | unit (polar) | `go test ./internal/polar/...` | ❌ Wave 0 |
| OAUTH-04 | Registration 409 → treated as success (idempotent) | unit (polar) | `go test ./internal/polar/...` | ❌ Wave 0 |
| OAUTH-05 | Token encrypted + upserted → blob stored | unit (store) | `go test ./internal/store/...` | ❌ Wave 0 |
| OAUTH-05 | Callback success → 200 HTML with "linked" text | httptest | `go test ./internal/oauth/...` | ❌ Wave 0 |
| MCP-01 | get_user_info: linked user → returns polar_user_id | unit (mcp) | `go test ./internal/mcp/...` | ❌ Wave 0 |
| MCP-01 | get_user_info: unlinked → returns "no account linked" hint | unit (mcp) | `go test ./internal/mcp/...` | ❌ Wave 0 |
| D-01 | Config.Load() fails if POLAR_CLIENT_ID missing | unit (config) | `go test ./internal/config/...` | ❌ Wave 0 |
| D-01 | Config.Load() fails if POLAR_CLIENT_SECRET missing | unit (config) | `go test ./internal/config/...` | ❌ Wave 0 |
| D-01 | Config.Load() fails if POLAR_REDIRECT_URL missing | unit (config) | `go test ./internal/config/...` | ❌ Wave 0 |

### Test Patterns to Follow

**Store tests (internal store package):**
- Use `openForTest(t)` helper (already defined in `store_test.go`) — shared in-memory DSN.
- Table-driven tests for `CreateOAuthState` / `ConsumeOAuthState` covering: happy path, missing state, expired state (insert with `expiresAt` in the past).
- Verify state row is deleted after `ConsumeOAuthState` (check `sqlite_master` or re-query).

**Polar client tests (internal polar package, external package test):**
- Use `httptest.NewServer` to mock the Polar token endpoint and registration endpoint.
- `ExchangeCode` test: mock server returns 200 with JSON body → verify `access_token` and `x_user_id` extracted.
- `ExchangeCode` test: mock returns 400 → verify error returned (not panic).
- `RegisterUser` test: mock returns 200 → verify `polar-user-id` extracted.
- `RegisterUser` test: mock returns 409 → verify no error returned (idempotent).

**OAuth handler tests (httptest):**
- `TestLoginHandler_Redirects`: valid request → 302, `Location` header contains Polar auth URL and `state` query param.
- `TestLoginHandler_StateStoredInDB`: after login request, `pending_auth` has a row with correct identity.
- `TestCallbackHandler_Success`: mock Polar endpoints (httptest.Server); valid code + state → 200 HTML, user in users table, token in polar_tokens.
- `TestCallbackHandler_InvalidState_Returns400`: state not in DB → 400.
- `TestCallbackHandler_ExpiredState_Returns400`: state in DB but expired → 400.
- `TestCallbackHandler_IdentityMismatch_Returns400`: state exists but bound to different identity → 400.

**Config tests (internal config package):**
- Table-driven: each missing Polar env var → specific error message checked.
- Extend existing `config_test.go`.

### Sampling Rate

- **Per task commit:** `CGO_ENABLED=0 go test -count=1 ./internal/...` + `golangci-lint run ./...`
- **Per wave merge:** `CGO_ENABLED=0 go test -race -count=1 ./...`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/store/store_oauth_test.go` — covers OAUTH-02, D-12 (CreateOAuthState + ConsumeOAuthState)
- [ ] `internal/store/store_users_test.go` — covers D-08, D-10 (UpsertUser + GetPolarUserID)
- [ ] `internal/store/store_tokens_test.go` — covers D-09, OAUTH-05 (UpsertToken)
- [ ] `internal/polar/client_test.go` — covers OAUTH-03, OAUTH-04 (ExchangeCode + RegisterUser with httptest mock)
- [ ] `internal/oauth/oauth_test.go` — covers OAUTH-01, OAUTH-02, OAUTH-05 (LoginHandler + CallbackHandler end-to-end with mocks)
- [ ] `internal/mcp/mcp_test.go` — covers MCP-01 (get_user_info happy path + no-account case)
- [ ] `internal/config/config_test.go` extension — covers D-01 (three new required Polar env vars)

*No new framework install needed — existing Go stdlib testing + httptest covers all cases.*

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All builds | ✓ | 1.26.2 (go.mod) | — |
| golangci-lint | Lint step | ✓ (CLAUDE.md) | v2.11 | — |
| modernc.org/sqlite | Store layer | ✓ (go.mod) | v1.50.0 | — |
| github.com/mark3labs/mcp-go | MCP tools | ✓ (go.mod) | v0.50.0 | — |
| golang.org/x/oauth2 | Phase 2 | NOT NEEDED | — | raw net/http (simpler) |
| Polar AccessLink API | Integration tests | live account needed | — | httptest mocks cover unit tests |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** `golang.org/x/oauth2` not in go.mod — replaced by raw `net/http` (recommended).

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `ExchangeCode` is best as a package-level function (not a `*Client` method) | Standard Stack / Pattern 5 | Low — either design works; planner can choose method on a config struct instead |
| A2 | `RegisterTools` signature changes to `(s *server.MCPServer, st *store.Store)` | Pattern 7 | Low — alternative is a closure in main.go; either compiles |
| A3 | Sentinel error names `ErrNotFound` and `ErrExpired` in store package | Pattern 3 | Low — names are implementation detail; any exported sentinel errors work |
| A4 | Option A (Handlers struct) preferred over Option B (closures) for oauth handler DI | Pattern 9 | Low — both are valid Go patterns; planner decides |
| A5 | `{"member-id": "<access_token>"}` is the correct member-id value for Polar registration | Pattern 6 | MEDIUM — this follows the official Python example. The docs say "partner's custom user identifier." If Polar rejects the access token as a member-id, a different stable identifier (e.g., identity header value) may be needed. Confirm with a live account in Phase 2 before Phase 3 depends on it. |
| A6 | `polar-user-id` in registration response is the same value as `x_user_id` in token response | Pattern 6 | LOW — both should refer to the same Polar ecosystem user ID; verified by API docs description |

---

## Open Questions

1. **member-id value for POST /v3/users**
   - What we know: The field is "partner's custom user identifier." The official Python example passes the access token.
   - What's unclear: Polar may also accept the proxy identity value (e.g., `alice@example.com`) as a more stable member-id. The access token is long-lived but is opaque.
   - Recommendation: Use the access token as member-id (matches official example). Verify with a live account in Phase 2. This does not block Phase 2 implementation.

2. **polar.Client HTTP client reuse**
   - What we know: The existing `polar.Client` has a 30-second timeout `*http.Client`. Phase 2 functions (ExchangeCode, RegisterUser) are called during the OAuth callback, not per-MCP-tool-call.
   - What's unclear: Whether to share the `*http.Client` instance with the Phase 3 `polar.Client` or keep separate.
   - Recommendation: Define a package-level `defaultHTTPClient` with 30-second timeout in `internal/polar`. Both Phase 2 functions and Phase 3 `Client` struct can reference it.

---

## Sources

### Primary (HIGH confidence)
- [Polar AccessLink API Docs — OAuth2](https://www.polar.com/accesslink-api/#oauth-2-0) — token endpoint URL, auth style, request/response format verified
- [Polar AccessLink API Docs — POST /v3/users](https://www.polar.com/accesslink-api/#tag/Users/operation/registerUser) — registration request + response format + 409 semantics verified
- [Context7: mark3labs/mcp-go](https://context7.com/mark3labs/mcp-go) — `s.AddTool`, `mcp.NewTool`, `mcp.NewToolResultText`, `mcp.NewToolResultError` patterns verified
- [Context7: golang/oauth2](https://context7.com/golang/oauth2) — `oauth2.Endpoint`, `AuthStyleInHeader`, `Exchange` API verified; confirms raw net/http is viable alternative
- [SQLite RETURNING clause docs](https://www.sqlite.org/lang_returning.html) — DELETE...RETURNING confirmed supported since SQLite 3.35
- [SQLite UPSERT docs](https://www.sqlite.org/lang_UPSERT.html) — ON CONFLICT DO UPDATE confirmed since SQLite 3.24
- Existing codebase: `internal/store/store.go`, `internal/auth/auth.go`, `internal/crypto/crypto.go`, `internal/config/config.go`, `cmd/polar-flow-mcp/main.go`, `internal/store/migrations/001_initial.up.sql` — all read directly

### Secondary (MEDIUM confidence)
- [polarofficial/accesslink-example-python — oauth2.py](https://github.com/polarofficial/accesslink-example-python) — confirms HTTP Basic auth pattern and member-id = access_token convention

### Tertiary (LOW confidence)
- None — all critical claims are VERIFIED or CITED.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries verified in go.mod; API shapes verified from official docs
- Architecture: HIGH — all patterns traced to existing codebase or official docs
- Pitfalls: HIGH — derived from schema inspection, linter config, and API docs
- Polar API shapes: MEDIUM-HIGH — verified from official API docs page; live account validation deferred to implementation

**Research date:** 2026-05-05
**Valid until:** 2026-06-05 (Polar API changes rarely; mcp-go may have patch releases)
