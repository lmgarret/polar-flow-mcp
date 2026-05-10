# Phase 2: OAuth Link Flow + UserInfo - Pattern Map

**Mapped:** 2026-05-05
**Files analyzed:** 15 (8 production + 7 test)
**Analogs found:** 15 / 15

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `internal/config/config.go` | config | request-response | `internal/config/config.go` (self — extend) | exact |
| `internal/store/store_oauth.go` | store | CRUD | `internal/store/store.go` | role-match |
| `internal/store/store_users.go` | store | CRUD | `internal/store/store.go` | role-match |
| `internal/store/store_tokens.go` | store | CRUD | `internal/store/store.go` | role-match |
| `internal/polar/client.go` | service | request-response | `internal/polar/client.go` (self — extend) | exact |
| `internal/oauth/oauth.go` | controller | request-response | `internal/auth/auth.go` + `cmd/.../main.go` | role-match |
| `internal/mcp/mcp.go` | controller | request-response | `internal/mcp/mcp.go` (self — extend) | exact |
| `cmd/polar-flow-mcp/main.go` | config/wiring | request-response | `cmd/polar-flow-mcp/main.go` (self — extend) | exact |
| `internal/config/config_test.go` | test | — | `internal/config/config_test.go` (self — extend) | exact |
| `internal/store/store_oauth_test.go` | test | — | `internal/store/store_test.go` | exact |
| `internal/store/store_users_test.go` | test | — | `internal/store/store_test.go` | exact |
| `internal/store/store_tokens_test.go` | test | — | `internal/store/store_test.go` | exact |
| `internal/polar/client_test.go` | test | — | `internal/store/store_test.go` | role-match |
| `internal/oauth/oauth_test.go` | test | — | `internal/store/store_test.go` | role-match |
| `internal/mcp/mcp_test.go` | test | — | `internal/store/store_test.go` | role-match |

---

## Pattern Assignments

### `internal/config/config.go` (config — extend existing)

**Analog:** `internal/config/config.go` (self)

**Existing Config struct fields** (lines 21–51) — add three Polar fields after `DatabasePath`/`Port`:
```go
// PolarClientID is the OAuth2 client ID from the Polar developer portal.
PolarClientID string

// PolarClientSecret is the OAuth2 client secret from the Polar developer portal.
PolarClientSecret string

// PolarRedirectURL is the callback URL registered in the Polar developer portal.
// Set by the operator; not derived from the request.
PolarRedirectURL string
```

**Fail-closed loader pattern** (lines 77–98, `loadAuthFields`) — add a new `loadPolarFields` function following this exact structure:
```go
// loadAuthFields validates and sets AUTH_PROXY and PROXY_SHARED_SECRET.
func loadAuthFields(cfg *Config) error {
    authProxy := os.Getenv("AUTH_PROXY")
    if authProxy == "" || authProxy == "unconfigured" {
        return errors.New(
            "AUTH_PROXY must be set to the name of your reverse proxy " +
                "(e.g. authelia, authentik, oauth2-proxy); " +
                "see https://github.com/lm/polar-flow-mcp/docs/deployment for configuration",
        )
    }
    cfg.AuthProxy = authProxy

    secret := os.Getenv("PROXY_SHARED_SECRET")
    if secret == "" {
        return errors.New(
            "PROXY_SHARED_SECRET must be set to a non-empty secret shared with your reverse proxy; " +
                "see deployment docs",
        )
    }
    cfg.ProxySharedSecret = secret

    return nil
}
```

New `loadPolarFields` follows this pattern — one env-var read + empty-check + assign per field, returning the first error encountered. Call it from `Load()` immediately after `loadAuthFields`.

**Load() call-site** (lines 57–74) — add the call after `loadAuthFields`:
```go
func Load() (*Config, error) {
    cfg := &Config{}

    if err := loadAuthFields(cfg); err != nil {
        return nil, err
    }

    if err := loadPolarFields(cfg); err != nil {  // ADD THIS
        return nil, err
    }

    loadOptionalFields(cfg)
    // ... rest unchanged
}
```

**Schema note:** `polar_user_id` in the migration schema (`001_initial.up.sql` line 4) is `TEXT`, not `INTEGER`. The store methods must use `string` for this column, not `int64`. The RESEARCH.md patterns show `int64` — the actual schema takes precedence. Planner must reconcile.

---

### `internal/store/store_oauth.go` (store, CRUD — new file)

**Analog:** `internal/store/store.go`

**Package declaration and imports pattern** (store.go lines 1–15):
```go
// Package store provides SQLite-backed persistent storage with dual connection pools.
package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"
)
```

**Receiver pattern** (store.go lines 97–101) — all new methods use `*Store` receiver accessing private fields:
```go
// WriteDB returns the serialized write connection pool.
func (s *Store) WriteDB() *sql.DB { return s.writeDB }

// ReadDB returns the concurrent read connection pool.
func (s *Store) ReadDB() *sql.DB { return s.readDB }
```

New methods access `s.writeDB` (writes/deletes) and `s.readDB` (reads) directly — the same private fields.

**Sentinel errors** — define at package level in `store_oauth.go` (exported, comparable with `errors.Is`):
```go
var (
    ErrNotFound = errors.New("store: record not found")
    ErrExpired  = errors.New("store: record expired")
)
```

**CreateOAuthState** — uses `s.writeDB`, wraps error:
```go
func (s *Store) CreateOAuthState(ctx context.Context, state, identity string, expiresAt time.Time) error {
    _, err := s.writeDB.ExecContext(ctx,
        `INSERT INTO pending_auth (state, identity, expires_at) VALUES (?, ?, ?)`,
        state, identity, expiresAt.UTC(),
    )
    if err != nil {
        return fmt.Errorf("store: create oauth state: %w", err)
    }
    return nil
}
```

**ConsumeOAuthState** — `DELETE ... RETURNING` on `s.writeDB` (write pool, not read — this is a write op):
```go
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

**Error wrapping pattern** from store.go (lines 54–57):
```go
if err != nil {
    return nil, fmt.Errorf("store: open write db: %w", err)
}
```
All store method errors use `fmt.Errorf("store: <method name>: %w", err)`.

---

### `internal/store/store_users.go` (store, CRUD — new file)

**Analog:** `internal/store/store.go`

**Package + import pattern** — same as store_oauth.go. Imports: `context`, `database/sql`, `errors`, `fmt`.

**UpsertUser** — uses `s.writeDB`; `ON CONFLICT(identity) DO UPDATE` (schema: `users.identity UNIQUE NOT NULL`):
```go
func (s *Store) UpsertUser(ctx context.Context, identity string, polarUserID string) error {
    _, err := s.writeDB.ExecContext(ctx,
        `INSERT INTO users (identity, polar_user_id, created_at)
         VALUES (?, ?, datetime('now'))
         ON CONFLICT(identity) DO UPDATE SET polar_user_id = excluded.polar_user_id`,
        identity, polarUserID,
    )
    if err != nil {
        return fmt.Errorf("store: upsert user: %w", err)
    }
    return nil
}
```

**Note on polar_user_id type:** The schema (`001_initial.up.sql` line 4) declares `polar_user_id TEXT`. Use `string` in Go, not `int64`. Store the value from `TokenResponse.XUserID` after converting via `strconv.FormatInt`.

**GetPolarUserID** — uses `s.readDB`; `sql.ErrNoRows` maps to `(zero, false, nil)`:
```go
func (s *Store) GetPolarUserID(ctx context.Context, identity string) (string, bool, error) {
    var polarUserID string
    err := s.readDB.QueryRowContext(ctx,
        `SELECT polar_user_id FROM users WHERE identity = ?`,
        identity,
    ).Scan(&polarUserID)
    if errors.Is(err, sql.ErrNoRows) {
        return "", false, nil
    }
    if err != nil {
        return "", false, fmt.Errorf("store: get polar user id: %w", err)
    }
    if polarUserID == "" {
        return "", false, nil
    }
    return polarUserID, true, nil
}
```

---

### `internal/store/store_tokens.go` (store, CRUD — new file)

**Analog:** `internal/store/store.go`

**Package + import pattern** — same. Imports: `context`, `fmt`.

**UpsertToken** — uses `s.writeDB`; subquery resolves `user_id` FK; `ON CONFLICT(user_id) DO UPDATE`:
```go
func (s *Store) UpsertToken(ctx context.Context, identity string, encryptedBlob []byte, keyVersion int) error {
    _, err := s.writeDB.ExecContext(ctx,
        `INSERT INTO polar_tokens (user_id, encrypted_token, key_version, updated_at)
         VALUES ((SELECT id FROM users WHERE identity = ?), ?, ?, datetime('now'))
         ON CONFLICT(user_id) DO UPDATE SET
             encrypted_token = excluded.encrypted_token,
             key_version     = excluded.key_version,
             updated_at      = datetime('now')`,
        identity, encryptedBlob, keyVersion,
    )
    if err != nil {
        return fmt.Errorf("store: upsert token: %w", err)
    }
    return nil
}
```

**Ordering constraint:** Always call `UpsertUser` before `UpsertToken` in the OAuth callback handler — `polar_tokens.user_id` has `NOT NULL` FK to `users(id)`. The subquery returns NULL if no user row exists, and the `NOT NULL` constraint will reject it.

---

### `internal/polar/client.go` (service, request-response — extend existing)

**Analog:** `internal/polar/client.go` (self)

**Existing file** (lines 1–23) — keep `Client` struct and `NewClient` unchanged. Add below:

**Package-level HTTP client with timeout** — pattern from existing `NewClient` (line 19):
```go
// defaultHTTPClient is shared by package-level functions (ExchangeCode, RegisterUser).
// Matches the 30-second timeout set on *Client for tool-call HTTP requests.
//
//nolint:gochecknoglobals
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}
```

**Imports to add:**
```go
import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"
)
```

**TokenResponse struct:**
```go
// TokenResponse holds the fields returned by the Polar token endpoint.
type TokenResponse struct {
    AccessToken string `json:"access_token"`
    TokenType   string `json:"token_type"`
    XUserID     int64  `json:"x_user_id"`
}
```

**ExchangeCode** — package-level function; `http.NewRequestWithContext` required by `noctx` linter:
```go
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

    resp, err := defaultHTTPClient.Do(req)
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

**RegisterUser** — 409 = idempotent success; returns `(0, nil)` on 409 so caller uses `XUserID` from `TokenResponse`:
```go
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

    resp, err := defaultHTTPClient.Do(req)
    if err != nil {
        return 0, fmt.Errorf("polar: register user: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusConflict {
        // Already registered — idempotent success (D-04).
        // polar-user-id is NOT in the 409 body; caller uses XUserID from token exchange.
        return 0, nil
    }
    if resp.StatusCode != http.StatusOK {
        errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return 0, fmt.Errorf("polar: register user: status %d: %s", resp.StatusCode, errBody)
    }
    var reg struct {
        PolarUserID int64 `json:"polar-user-id"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
        return 0, fmt.Errorf("polar: decode register response: %w", err)
    }
    return reg.PolarUserID, nil
}
```

---

### `internal/oauth/oauth.go` (controller, request-response — replace stubs)

**Analog (primary):** `internal/auth/auth.go` — identity-from-context pattern
**Analog (secondary):** `cmd/polar-flow-mcp/main.go` — handler wiring pattern

**Identity extraction pattern** from `auth.go` (lines 25–28) — use two-value form (`errcheck` requirement):
```go
// From internal/auth/auth.go lines 25-28:
func UserIDFromContext(ctx context.Context) (string, bool) {
    v, ok := ctx.Value(userIDKey{}).(string)
    return v, ok
}
```
OAuth handlers must call `auth.UserIDFromContext(r.Context())` and check `ok`. Never read the identity header directly.

**Handler struct pattern (Option A — recommended for testability):**
```go
// Handlers holds the dependencies for the OAuth flow HTTP handlers.
type Handlers struct {
    cfg    *config.Config
    st     *store.Store
    cipher *crypto.Cipher
}

// NewHandlers creates an Handlers instance with all required dependencies.
func NewHandlers(cfg *config.Config, st *store.Store, cipher *crypto.Cipher) *Handlers {
    return &Handlers{cfg: cfg, st: st, cipher: cipher}
}
```

**slog.Warn pattern** from `auth.go` (lines 46–51):
```go
slog.Warn("proxy secret mismatch — probable spoofing attempt",
    "remote_addr", r.RemoteAddr,
    "method", r.Method,
    "path", r.URL.Path,
)
```
`oauth.CallbackHandler` uses the same structured logging for identity mismatch (CSRF warning).

**HTTP error response pattern** from `auth.go` (lines 53–54):
```go
http.Error(w, "forbidden", http.StatusForbidden)
```
OAuth handlers replace this with `htmlError(w, status, msg)` for inline HTML responses.

**Inline HTML helpers** — package-private functions in `oauth.go`; use `html.EscapeString` from stdlib `html` package on any user-visible error text:
```go
func htmlSuccess(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    // ... inline HTML with no external deps
}

func htmlError(w http.ResponseWriter, status int, msg string) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(status)
    // msg must be html.EscapeString(msg) before embedding
}
```

**CSRF state generation** — `crypto/rand` + `encoding/hex`; same import approach as `crypto.go` (line 8):
```go
import cryptorand "crypto/rand"  // alias to avoid collision with internal/crypto
```

**url.Values for redirect URL** — never string concatenation; from RESEARCH.md anti-patterns:
```go
params := url.Values{
    "response_type": {"code"},
    "client_id":     {h.cfg.PolarClientID},
    "redirect_uri":  {h.cfg.PolarRedirectURL},
    "scope":         {"accesslink.read_all"},
    "state":         {state},
}
redirectURL := "https://flow.polar.com/oauth2/authorization?" + params.Encode()
http.Redirect(w, r, redirectURL, http.StatusFound)
```

**ConsumeOAuthState error handling** — pattern from RESEARCH.md Pitfall 2; state is already consumed when identity check runs:
```go
identity, err := h.st.ConsumeOAuthState(r.Context(), state)
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
currentIdentity, _ := auth.UserIDFromContext(r.Context())
if identity != currentIdentity {
    slog.Warn("oauth callback identity mismatch — possible CSRF",
        "state_identity", identity, "request_identity", currentIdentity)
    htmlError(w, http.StatusBadRequest, "identity mismatch")
    return
}
```

**Imports for oauth.go:**
```go
import (
    "crypto/rand"
    "encoding/hex"
    "errors"
    "fmt"
    "html"
    "io"
    "log/slog"
    "net/http"
    "net/url"
    "time"

    "github.com/lm/polar-flow-mcp/internal/auth"
    "github.com/lm/polar-flow-mcp/internal/config"
    "github.com/lm/polar-flow-mcp/internal/crypto"
    "github.com/lm/polar-flow-mcp/internal/polar"
    "github.com/lm/polar-flow-mcp/internal/store"
)
```

---

### `internal/mcp/mcp.go` (controller, request-response — extend existing)

**Analog:** `internal/mcp/mcp.go` (self — replace stub body)

**Current stub** (lines 1–9):
```go
package mcp

import "github.com/mark3labs/mcp-go/server"

func RegisterTools(s *server.MCPServer) {
}
```

**Signature change** — add `*store.Store` parameter (planner updates call site in `main.go`):
```go
func RegisterTools(s *server.MCPServer, st *store.Store) {
```

**mcp-go tool registration pattern** — from RESEARCH.md Pattern 7 (verified against mcp-go docs):
```go
getUserInfoTool := mcp.NewTool("get_user_info",
    mcp.WithDescription("Returns the Polar account linked to your identity. "+
        "If no account is linked, returns instructions to link one."),
)
s.AddTool(getUserInfoTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // ...
})
```

**Two-value type assertion** (errcheck linter — from auth.go lines 25–27):
```go
identity, ok := auth.UserIDFromContext(ctx)
if !ok {
    return mcp.NewToolResultError("no identity in context — auth middleware not applied"), nil
}
```

**Imports for mcp.go:**
```go
import (
    "context"
    "fmt"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"

    "github.com/lm/polar-flow-mcp/internal/auth"
    "github.com/lm/polar-flow-mcp/internal/store"
)
```

---

### `cmd/polar-flow-mcp/main.go` (wiring — minimal change)

**Analog:** `cmd/polar-flow-mcp/main.go` (self)

**RegisterTools call-site** (line 60) — add `st` argument to match new signature:
```go
// Before:
mcp.RegisterTools(mcpServer)

// After:
mcp.RegisterTools(mcpServer, st)
```

**OAuth route wiring** (lines 93–98) — currently uses package-level functions as stubs. With `Handlers` struct (Option A), update to:
```go
oauthHandlers := oauth.NewHandlers(cfg, st, cipher)
mux.Handle("GET /oauth/login",
    auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader,
        http.HandlerFunc(oauthHandlers.Login)))
mux.Handle("GET /oauth/callback",
    auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader,
        http.HandlerFunc(oauthHandlers.Callback)))
```

`cipher` is a `*crypto.Cipher` constructed from `cfg.EncryptionKey` — add before the mux build:
```go
// Construct encryption cipher from validated key.
keyProvider := crypto.StaticKeyProvider(cfg.EncryptionKey) // or equivalent Phase 1 KeyProvider
cipher := crypto.NewCipher(keyProvider)
```

**Note:** Check how Phase 1 wires the `KeyProvider` to `crypto.NewCipher` — the exact `KeyProvider` implementation name is in the existing codebase but not yet in these files. The planner must verify against the actual Phase 1 deliverable.

---

## Shared Patterns

### Identity Extraction from Context
**Source:** `internal/auth/auth.go` lines 25–28
**Apply to:** `internal/oauth/oauth.go` (LoginHandler, CallbackHandler), `internal/mcp/mcp.go` (get_user_info handler)
```go
identity, ok := auth.UserIDFromContext(ctx)  // or r.Context() in HTTP handlers
if !ok {
    // handle missing identity — auth middleware wiring bug
}
```
Always two-value form. Never read identity header directly in handlers.

### Error Wrapping Convention
**Source:** `internal/store/store.go` lines 54–57, 62–64
**Apply to:** All new store methods, polar client functions
```go
return nil, fmt.Errorf("store: <method name>: %w", err)
return nil, fmt.Errorf("polar: <operation name>: %w", err)
```
Package prefix + operation name + `%w` for all errors. No bare `err` returns.

### slog Structured Logging
**Source:** `internal/auth/auth.go` lines 46–51
**Apply to:** `internal/oauth/oauth.go` (CSRF warning, unexpected errors)
```go
slog.Warn("oauth callback identity mismatch — possible CSRF",
    "state_identity", identity,
    "request_identity", currentIdentity,
)
```
Key-value pairs for all structured fields. No `fmt.Sprintf` in log messages.

### noctx Linter Compliance
**Source:** CLAUDE.md lint config + RESEARCH.md Pitfall 3
**Apply to:** `internal/polar/client.go` (ExchangeCode, RegisterUser)

Every outbound HTTP request must use `http.NewRequestWithContext(ctx, ...)` — never `http.NewRequest`. The `noctx` golangci-lint rule rejects context-less requests. `ctx` comes from the HTTP handler's `r.Context()` passed through to polar functions.

### errcheck Type Assertion Safety
**Source:** `internal/auth/auth.go` lines 26–27; CLAUDE.md lint config
**Apply to:** All handlers reading from `context.Value`
```go
// Correct — two-value form:
v, ok := ctx.Value(key).(string)

// Wrong — bare assertion rejected by errcheck:
v := ctx.Value(key).(string)
```

### WAL Dual-Pool Write/Read Routing
**Source:** `internal/store/store.go` lines 97–101
**Apply to:** All new store methods
- `s.writeDB` — INSERT, UPDATE, DELETE, DELETE...RETURNING
- `s.readDB` — SELECT only
- `DELETE...RETURNING` is a write operation — use `s.writeDB.QueryRowContext`

### Test In-Memory DSN
**Source:** `internal/store/store_test.go` lines 96–108 (`openForTest`)
**Apply to:** All new store test files (`store_oauth_test.go`, `store_users_test.go`, `store_tokens_test.go`)
```go
func openForTest(t *testing.T) *Store {
    t.Helper()
    dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
    s, err := Open(dsn)
    if err != nil {
        t.Fatalf("Open: %v", err)
    }
    t.Cleanup(func() {
        if err := s.Close(); err != nil {
            t.Errorf("Close: %v", err)
        }
    })
    return s
}
```
`openForTest` is already defined in `store_test.go` (same package). New test files in the same `store` package can call it directly — do NOT redefine it.

---

## Schema Ground Truth

From `internal/store/migrations/001_initial.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    identity      TEXT    UNIQUE NOT NULL,
    polar_user_id TEXT,                          -- TEXT, not INTEGER
    created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS polar_tokens (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    encrypted_token BLOB    NOT NULL,
    key_version     INTEGER NOT NULL DEFAULT 1,
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS pending_auth (
    state      TEXT     PRIMARY KEY,
    identity   TEXT     NOT NULL,
    expires_at DATETIME NOT NULL
);
```

**Critical:** `polar_user_id` is `TEXT` in the schema. All store methods and tests must use `string`, not `int64`. `GetPolarUserID` returns `(string, bool, error)`. `UpsertUser` takes `polarUserID string`. Convert `TokenResponse.XUserID` (int64) via `strconv.FormatInt(tr.XUserID, 10)` before storing.

---

## Test Pattern Assignments

### `internal/config/config_test.go` (extend existing)

**Analog:** `internal/config/config_test.go` (self — extend)

**clearConfigEnv pattern** (lines 13–26) — add three new Polar env vars to the clear list:
```go
for _, key := range []string{
    "AUTH_PROXY",
    "PROXY_SHARED_SECRET",
    // ... existing keys ...
    "POLAR_CLIENT_ID",      // ADD
    "POLAR_CLIENT_SECRET",  // ADD
    "POLAR_REDIRECT_URL",   // ADD
} {
    t.Setenv(key, "")
}
```

**Table-driven test pattern** (lines 30–75) — one test function per missing required field, checking `err != nil` and `strings.Contains(err.Error(), "POLAR_CLIENT_ID")` etc. Also need a success-path test with all three set (extend `TestLoadEncryptionKeyValidBase64Succeeds` or add a new one).

### `internal/store/store_oauth_test.go` (new file, same package `store`)

**Analog:** `internal/store/store_test.go`

- Internal package test (`package store`) — can call `openForTest(t)` directly.
- Table-driven: `TestCreateAndConsumeOAuthState` (happy path), `TestConsumeOAuthState_NotFound` (state not in DB → `ErrNotFound`), `TestConsumeOAuthState_Expired` (insert with past `expiresAt` → `ErrExpired`), `TestConsumeOAuthState_DeletesRow` (re-query after consume returns `ErrNotFound`).

### `internal/store/store_users_test.go` (new file, same package `store`)

**Analog:** `internal/store/store_test.go`

- `TestUpsertUser_Insert` — insert new identity, verify row in DB.
- `TestUpsertUser_UpdateOnConflict` — upsert same identity with new `polarUserID`, verify update.
- `TestGetPolarUserID_Found` — after `UpsertUser`, `GetPolarUserID` returns `(id, true, nil)`.
- `TestGetPolarUserID_NotFound` — unknown identity returns `("", false, nil)`.

### `internal/store/store_tokens_test.go` (new file, same package `store`)

**Analog:** `internal/store/store_test.go`

- `TestUpsertToken_Insert` — requires prior `UpsertUser`; verifies token blob stored.
- `TestUpsertToken_UpdateOnConflict` — upsert second token for same user, verify replacement.

### `internal/polar/client_test.go` (new file, external package `polar_test`)

**Analog:** `internal/store/store_test.go` (structure only; uses `net/http/httptest` instead of openForTest)

- External package (`package polar_test`) — tests exported API only.
- Use `httptest.NewServer` to mock Polar endpoints.
- `TestExchangeCode_Success` — mock returns 200 JSON with `access_token` + `x_user_id`.
- `TestExchangeCode_NonOKStatus` — mock returns 400 → verify error returned, no panic.
- `TestRegisterUser_Success` — mock returns 200 with `polar-user-id` → verify extracted.
- `TestRegisterUser_Conflict` — mock returns 409 → verify `(0, nil)` (idempotent).

### `internal/oauth/oauth_test.go` (new file, external package `oauth_test`)

**Analog:** `internal/store/store_test.go` (structure); uses `net/http/httptest.NewRecorder`

- External package (`package oauth_test`).
- `TestLoginHandler_Redirects` — inject identity into context, call handler, verify 302 + `Location` contains Polar auth URL + `state` param.
- `TestLoginHandler_StateStoredInDB` — after login, `pending_auth` has row for identity.
- `TestCallbackHandler_InvalidState_Returns400` — state not in DB → 400 HTML.
- `TestCallbackHandler_ExpiredState_Returns400` — expired state in DB → 400 HTML.
- `TestCallbackHandler_IdentityMismatch_Returns400` — state bound to different identity → 400 HTML.
- `TestCallbackHandler_Success` — mock polar endpoints via `httptest.NewServer`; valid flow → 200 HTML, user row in users, token in polar_tokens.

### `internal/mcp/mcp_test.go` (new file, external package `mcp_test`)

**Analog:** `internal/store/store_test.go` (structure only)

- External package (`package mcp_test`).
- `TestGetUserInfo_Linked` — inject identity + populated store → result contains `polar_user_id`.
- `TestGetUserInfo_NotLinked` — inject identity + empty store → result contains "no Polar account" hint.

---

## No Analog Found

All files have close analogs. No entries.

---

## Metadata

**Analog search scope:** `internal/config/`, `internal/store/`, `internal/auth/`, `internal/crypto/`, `internal/polar/`, `internal/oauth/`, `internal/mcp/`, `cmd/polar-flow-mcp/`
**Files scanned:** 10 source files + 1 migration SQL
**Pattern extraction date:** 2026-05-05
