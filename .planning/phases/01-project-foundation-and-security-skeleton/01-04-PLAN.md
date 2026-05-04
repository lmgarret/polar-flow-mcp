---
phase: 01-project-foundation-and-security-skeleton
plan: 04
type: execute
wave: 3
depends_on:
  - 01-PLAN-02-config-validation.md
  - 01-PLAN-03-sqlite-crypto.md
files_modified:
  - internal/auth/auth.go
  - internal/auth/auth_test.go
  - cmd/polar-flow-mcp/main.go
autonomous: true
requirements:
  - SERV-01
  - SERV-02
  - SERV-03
  - SERV-04
  - SERV-05

must_haves:
  truths:
    - "GET /healthz returns 200 always — no auth, no config check"
    - "GET /readyz returns 200 only when all checks pass (AUTH_PROXY set, secret set, DB reachable, key loaded)"
    - "GET /readyz returns 503 with named failing checks when any check fails"
    - "All routes except /healthz return 403 when PROXY_SHARED_SECRET header is missing"
    - "All routes except /healthz return 403 when PROXY_SHARED_SECRET header value is wrong"
    - "Authenticated routes with missing identity header return 403 with a warning log"
    - "MCP handler is mounted at /mcp and user identity is in context for every tool call"
    - "Startup WARN log emitted when BIND_ADDRESS is not loopback (SERV-05)"
  artifacts:
    - path: "internal/auth/auth.go"
      provides: "Middleware with subtle.ConstantTimeCompare secret check BEFORE identity header read"
      contains: "subtle.ConstantTimeCompare"
    - path: "internal/auth/auth_test.go"
      provides: "Integration tests via httptest covering all middleware behaviors"
      min_lines: 80
    - path: "cmd/polar-flow-mcp/main.go"
      provides: "Full HTTP server wiring: ServeMux, /healthz, /readyz, /mcp, auth middleware"
  key_links:
    - from: "internal/auth/auth.go"
      to: "config.ProxySecretHeader"
      via: "reads r.Header.Get(config.ProxySecretHeader) for the secret comparison"
      pattern: "ProxySecretHeader"
    - from: "internal/auth/auth.go"
      to: "auth.userIDKey"
      via: "context.WithValue(r.Context(), userIDKey{}, identity)"
      pattern: "userIDKey{}"
    - from: "cmd/polar-flow-mcp/main.go"
      to: "internal/store"
      via: "store.Open(cfg.DatabasePath) called before http.ListenAndServe"
      pattern: "store\\.Open"
---

<objective>
Implement the full auth middleware (secret check with `subtle.ConstantTimeCompare` BEFORE identity header extraction, per D-04), wire up the HTTP server in `main.go` with `/healthz`, `/readyz`, `/mcp` mount via `StreamableHTTPServer`, and OAuth routes. Write httptest integration tests for all middleware behaviors.

Purpose: SERV-01..05 are the security-critical HTTP layer. Middleware ordering is locked (D-04): secret check first, identity read second. This ordering must be enforced by code structure, not convention.

Output: `internal/auth/auth.go` with full Middleware + `UserIDFromContext`; `internal/auth/auth_test.go` with httptest coverage; `cmd/polar-flow-mcp/main.go` fully wired.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md
@.planning/research/ARCHITECTURE.md

@.planning/phases/01-project-foundation-and-security-skeleton/01-01-SUMMARY.md
@.planning/phases/01-project-foundation-and-security-skeleton/01-02-SUMMARY.md
@.planning/phases/01-project-foundation-and-security-skeleton/01-03-SUMMARY.md
</context>

<interfaces>
<!-- Types and functions from Plans 02 and 03 that this plan depends on -->

From internal/config/config.go (Plan 02):
```go
const ProxySecretHeader = "X-Proxy-Secret"

type Config struct {
    AuthProxy         string
    ProxySharedSecret string
    IdentityHeader    string  // default "Remote-User"
    BindAddress       string  // default "127.0.0.1"
    KeyProviderType   string
    EncryptionKey     []byte  // pre-validated 32 bytes
}
```

From internal/store/store.go (Plan 03):
```go
func Open(dsn string) (*Store, error)
func (s *Store) Ping(ctx context.Context) error
func (s *Store) Close() error
```

From internal/auth/auth.go (Plan 01 stub — to be replaced):
```go
type userIDKey struct{}
var UserIDKey = userIDKey{}
func UserIDFromContext(ctx context.Context) (string, bool)
func Middleware(secret, identityHeader string, next http.Handler) http.Handler
```

From internal/mcp/mcp.go (Plan 01 stub):
```go
func RegisterTools(s *server.MCPServer)
```
</interfaces>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Auth middleware with secret check before identity extraction</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/internal/auth/auth.go (current stub — read before overwriting)
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (D-01, D-03, D-04 — ordering is locked)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/PITFALLS.md (Pitfall 1: Proxy header spoofing, Pitfall 4: MCP context leakage)
    - /var/home/lm/git/polar-flow-mcp/internal/config/config.go (ProxySecretHeader constant)
  </read_first>
  <files>
    internal/auth/auth.go,
    internal/auth/auth_test.go
  </files>
  <behavior>
    - Test 1: Request with correct secret and valid identity header → 200 from downstream handler
    - Test 2: Request with missing secret header → 403, downstream NOT called
    - Test 3: Request with wrong secret value → 403, downstream NOT called; log entry at WARN level contains "secret mismatch" or "spoofing"
    - Test 4: Request with correct secret but empty identity header → 403 with warning log (D-03)
    - Test 5: Request with correct secret and valid identity → identity string available via UserIDFromContext(ctx)
    - Test 6: UserIDFromContext on context without identity key returns ("", false)
    - Test 7: Two concurrent requests with different identity headers produce different values from UserIDFromContext (no shared state)
  </behavior>
  <action>
Replace stub `internal/auth/auth.go` with the full middleware:

```go
package auth

import (
    "context"
    "crypto/subtle"
    "log/slog"
    "net/http"

    "github.com/lm/polar-flow-mcp/internal/config"
)

// userIDKey is the unexported context key type for user identity (per D-12).
// Unexported struct type prevents cross-package key collisions.
type userIDKey struct{}

// UserIDFromContext extracts the authenticated user identity from ctx.
// Returns ("", false) if not present — call sites MUST handle the missing case.
func UserIDFromContext(ctx context.Context) (string, bool) {
    v, ok := ctx.Value(userIDKey{}).(string)
    return v, ok
}

// Middleware returns an http.Handler that enforces the proxy shared-secret contract.
//
// Security ordering (D-04, LOCKED — must not be inverted):
//  1. Compare PROXY_SHARED_SECRET header using subtle.ConstantTimeCompare → 403 on mismatch
//  2. Read identity header → 403 + WARN log if missing or empty
//  3. Inject identity into context → call next handler
//
// The secret check MUST be the first operation; reading the identity header
// before the secret check would allow header spoofing if the secret is wrong.
func Middleware(secret, identityHeader string, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Step 1: Shared-secret check FIRST (D-04).
        // subtle.ConstantTimeCompare prevents timing-oracle on the secret value.
        provided := r.Header.Get(config.ProxySecretHeader)
        if subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
            if provided != "" {
                // Non-empty wrong secret: probable spoofing attempt.
                slog.Warn("proxy secret mismatch — probable spoofing attempt",
                    "remote_addr", r.RemoteAddr,
                    "method", r.Method,
                    "path", r.URL.Path,
                )
            }
            http.Error(w, "forbidden", http.StatusForbidden)
            return
        }

        // Step 2: Identity header extraction (AFTER secret check — D-04).
        identity := r.Header.Get(identityHeader)
        if identity == "" {
            slog.Warn("missing identity header — probable proxy misconfiguration",
                "identity_header", identityHeader,
                "remote_addr", r.RemoteAddr,
                "path", r.URL.Path,
            )
            http.Error(w, "forbidden", http.StatusForbidden)
            return
        }

        // Step 3: Inject identity into context.
        ctx := context.WithValue(r.Context(), userIDKey{}, identity)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

Write `internal/auth/auth_test.go` using `net/http/httptest`. For the secret mismatch log test, use a custom `slog.Handler` that captures log records, or simply verify the 403 response is returned (log content testing is optional — response code testing is mandatory).

Test for concurrent safety (Test 7): create two `httptest.ResponseRecorder`, call the middleware handler in two goroutines with different identity headers simultaneously, use a `sync.WaitGroup`, assert each goroutine's identity is distinct. Run with `-race`.
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && CGO_ENABLED=0 go test -race -count=1 ./internal/auth/...</automated>
  </verify>
  <acceptance_criteria>
    - `internal/auth/auth.go` contains `crypto/subtle` import
    - `internal/auth/auth.go` contains `subtle.ConstantTimeCompare`
    - `internal/auth/auth.go` contains `context.WithValue(r.Context(), userIDKey{}, identity)`
    - The `subtle.ConstantTimeCompare` line appears BEFORE any `r.Header.Get(identityHeader)` call in Middleware
    - `internal/auth/auth_test.go` contains at least 7 test cases
    - `CGO_ENABLED=0 go test -race -count=1 ./internal/auth/...` exits 0
    - `~/go/bin/golangci-lint run ./internal/auth/...` exits 0
  </acceptance_criteria>
  <done>Auth middleware implements secret-check-first ordering with subtle.ConstantTimeCompare; all 7 middleware tests pass including concurrent identity test; lint clean.</done>
</task>

<task type="auto">
  <name>Task 2: Wire HTTP server in main.go with /healthz, /readyz, /mcp, and OAuth stubs</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/cmd/polar-flow-mcp/main.go (current state after Plan 02 — read before overwriting)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/ARCHITECTURE.md (HTTP mux layout section, WithHTTPContextFunc pattern)
    - /var/home/lm/git/polar-flow-mcp/.planning/REQUIREMENTS.md (SERV-01: healthz always-200, SERV-02: readyz 503, SERV-04: MCP at /mcp)
    - /var/home/lm/git/polar-flow-mcp/internal/config/config.go (Config struct fields)
    - /var/home/lm/git/polar-flow-mcp/internal/store/store.go (Store.Ping signature)
  </read_first>
  <files>
    cmd/polar-flow-mcp/main.go
  </files>
  <action>
Replace the stub `main.go` with the fully wired HTTP server. The structure must be:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/mark3labs/mcp-go/server"
    "github.com/lm/polar-flow-mcp/internal/auth"
    "github.com/lm/polar-flow-mcp/internal/config"
    "github.com/lm/polar-flow-mcp/internal/mcp"
    "github.com/lm/polar-flow-mcp/internal/oauth"
    "github.com/lm/polar-flow-mcp/internal/store"
)

func main() {
    // 1. Load and validate config (fail-closed, exits 1 on error)
    cfg, err := config.Load()
    if err != nil {
        slog.Error("startup failed", "error", err)
        os.Exit(1)
    }

    // 2. Emit startup security banner
    config.LogStartupBanner(cfg)

    // 3. Warn on non-loopback bind address (SERV-05)
    if cfg.BindAddress != "127.0.0.1" && cfg.BindAddress != "::1" {
        slog.Warn("BIND_ADDRESS is not localhost — ensure PROXY_SHARED_SECRET is set and the server is not directly reachable without the reverse proxy",
            "bind_address", cfg.BindAddress)
    }

    // 4. Open SQLite store (WAL dual-pool + migrations)
    st, err := store.Open(cfg.DatabasePath)
    if err != nil {
        slog.Error("failed to open database", "error", err)
        os.Exit(1)
    }
    defer func() { _ = st.Close() }()

    // 5. Create MCP server with StreamableHTTP transport.
    // WithHTTPContextFunc fires per-request (verified in ARCHITECTURE.md).
    // Identity is injected by auth middleware before reaching MCP handler — this
    // function is defense-in-depth only; it reads from the already-enriched context.
    mcpServer := server.NewMCPServer(
        "polar-flow-mcp",
        "0.1.0",
        server.WithToolCapabilities(true),
    )
    mcp.RegisterTools(mcpServer)

    httpMCPServer := server.NewStreamableHTTPServer(mcpServer,
        server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
            // Identity was injected by auth.Middleware into r.Context() before this fires.
            // Re-extract from request context so MCP tool handlers receive it.
            if id, ok := auth.UserIDFromContext(r.Context()); ok {
                return context.WithValue(ctx, auth.UserIDKey, id)
            }
            return ctx
        }),
    )

    // 6. Build HTTP mux
    mux := http.NewServeMux()

    // /healthz — no auth, always 200 (SERV-01)
    mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = fmt.Fprint(w, "ok")
    })

    // /readyz — no auth, checks config + DB (SERV-02)
    mux.HandleFunc("GET /readyz", readyzHandler(cfg, st))

    // /mcp — auth middleware wraps StreamableHTTPServer (SERV-03, SERV-04)
    mux.Handle("/mcp", auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader, httpMCPServer))
    mux.Handle("/mcp/", auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader, httpMCPServer))

    // /oauth — auth middleware wraps stubs (Phase 2 fills these in)
    mux.Handle("GET /oauth/login",
        auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader,
            http.HandlerFunc(oauth.LoginHandler)))
    mux.Handle("GET /oauth/callback",
        auth.Middleware(cfg.ProxySharedSecret, cfg.IdentityHeader,
            http.HandlerFunc(oauth.CallbackHandler)))

    // 7. Start HTTP server with graceful shutdown
    srv := &http.Server{
        Addr:         cfg.BindAddress + ":8080",
        Handler:      mux,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 60 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    slog.Info("server listening", "addr", srv.Addr)

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("server error", "error", err)
            os.Exit(1)
        }
    }()

    <-quit
    slog.Info("shutting down")
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    _ = srv.Shutdown(ctx)
}
```

**readyzHandler** (defined as a helper in main.go, not inline):

```go
func readyzHandler(cfg *config.Config, st *store.Store) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        type check struct {
            Name   string `json:"name"`
            Status string `json:"status"`
            Error  string `json:"error,omitempty"`
        }
        checks := []check{}
        allOK := true

        // Check 1: AUTH_PROXY not unconfigured
        if cfg.AuthProxy == "unconfigured" || cfg.AuthProxy == "" {
            checks = append(checks, check{Name: "auth_proxy", Status: "fail", Error: "AUTH_PROXY not configured"})
            allOK = false
        } else {
            checks = append(checks, check{Name: "auth_proxy", Status: "ok"})
        }

        // Check 2: PROXY_SHARED_SECRET set
        if cfg.ProxySharedSecret == "" {
            checks = append(checks, check{Name: "proxy_secret", Status: "fail", Error: "PROXY_SHARED_SECRET not set"})
            allOK = false
        } else {
            checks = append(checks, check{Name: "proxy_secret", Status: "ok"})
        }

        // Check 3: DB reachable (both pools — D-07)
        if err := st.Ping(r.Context()); err != nil {
            checks = append(checks, check{Name: "database", Status: "fail", Error: err.Error()})
            allOK = false
        } else {
            checks = append(checks, check{Name: "database", Status: "ok"})
        }

        // Check 4: Encryption key loaded
        if len(cfg.EncryptionKey) != 32 {
            checks = append(checks, check{Name: "encryption_key", Status: "fail", Error: "encryption key not loaded or wrong length"})
            allOK = false
        } else {
            checks = append(checks, check{Name: "encryption_key", Status: "ok"})
        }

        w.Header().Set("Content-Type", "application/json")
        if allOK {
            w.WriteHeader(http.StatusOK)
        } else {
            w.WriteHeader(http.StatusServiceUnavailable)
        }
        _ = json.NewEncoder(w).Encode(map[string]any{"checks": checks})
    }
}
```

Config must also have a `DatabasePath string` field (add it to Plan 02's Config struct if not present; default to `"polar.db"`). If Plan 02's Config already has this field, use it directly.

Add `DATABASE_PATH` to config.Load() — reads `DATABASE_PATH` env var, defaults to `"polar.db"`.

Also add `export` of `UserIDKey` from `internal/auth/auth.go` so `main.go`'s `WithHTTPContextFunc` can reference it: `var UserIDKey = userIDKey{}` (already in Plan 01 stub — verify it exists).
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && CGO_ENABLED=0 go build ./... && echo "build OK"</automated>
  </verify>
  <acceptance_criteria>
    - `cmd/polar-flow-mcp/main.go` contains `server.NewStreamableHTTPServer`
    - `cmd/polar-flow-mcp/main.go` contains `"GET /healthz"` route handler
    - `cmd/polar-flow-mcp/main.go` contains `readyzHandler`
    - `cmd/polar-flow-mcp/main.go` contains `auth.Middleware` wrapping the `/mcp` route
    - `cmd/polar-flow-mcp/main.go` contains `subtle` check — NO, the subtle check is in auth.go; verify by grep: `grep "subtle" internal/auth/auth.go` returns a match
    - `cmd/polar-flow-mcp/main.go` contains `srv.Shutdown` (graceful shutdown)
    - `readyzHandler` checks all 4 conditions: auth_proxy, proxy_secret, database, encryption_key
    - `CGO_ENABLED=0 go build ./...` exits 0
    - `~/go/bin/golangci-lint run ./...` exits 0
  </acceptance_criteria>
  <done>Full HTTP server wired: /healthz always-200, /readyz with 4-check body, /mcp behind auth middleware, graceful shutdown; build and lint pass.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| external client→/healthz | Unauthenticated; returns only "ok" — zero information disclosure |
| external client→all other routes | Must present PROXY_SHARED_SECRET header; checked before any identity read |
| reverse proxy→server | Identity header trusted only after secret check passes |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-04-01 | Spoofing | proxy identity header | mitigate | `subtle.ConstantTimeCompare` on secret BEFORE identity header read; auth middleware enforces ordering |
| T-04-02 | Information Disclosure | /readyz response body | accept | Check names ("auth_proxy", "database") are non-sensitive configuration status; no secrets in body |
| T-04-03 | Elevation of Privilege | /mcp without auth | mitigate | `/mcp` and `/mcp/` both wrapped with `auth.Middleware`; only 2 routes bypass auth (/healthz, /readyz) |
| T-04-04 | Spoofing | timing oracle on secret | mitigate | `subtle.ConstantTimeCompare` ensures constant-time comparison regardless of secret length |
| T-04-05 | Denial of Service | slow client attack | mitigate | `ReadTimeout: 30s`, `WriteTimeout: 60s`, `IdleTimeout: 120s` set on http.Server |
| T-04-06 | Information Disclosure | /readyz accessible without auth | accept | /readyz is intentionally open for monitoring probes; response contains only config/DB status, no user data |
</threat_model>

<verification>
```bash
cd /var/home/lm/git/polar-flow-mcp

# Full build
CGO_ENABLED=0 go build ./...

# All tests
CGO_ENABLED=0 go test -race -count=1 ./...

# Lint
~/go/bin/golangci-lint run ./...

# Verify secret-check ordering: subtle appears before identity header read in auth.go
grep -n "subtle\|ConstantTimeCompare\|identityHeader\|r\.Header\.Get" internal/auth/auth.go

# Integration test: auth middleware
CGO_ENABLED=0 go test -race -count=1 -v ./internal/auth/...

# Verify /mcp is wrapped in auth middleware
grep -A2 '"/mcp"' cmd/polar-flow-mcp/main.go | grep "Middleware"

# Verify readyz has all 4 checks
grep -c '"auth_proxy"\|"proxy_secret"\|"database"\|"encryption_key"' cmd/polar-flow-mcp/main.go
```
</verification>

<success_criteria>
- `CGO_ENABLED=0 go build ./...` exits 0
- `CGO_ENABLED=0 go test -race -count=1 ./...` exits 0
- `~/go/bin/golangci-lint run ./...` exits 0
- Auth middleware tests: 403 on missing secret, 403 on wrong secret, 403 on missing identity, 200 on valid request
- `/healthz` handler has no auth middleware (verified by code inspection: registered on mux without Middleware wrapper)
- `/readyz` handler checks 4 named conditions: auth_proxy, proxy_secret, database, encryption_key
- `subtle.ConstantTimeCompare` appears in `internal/auth/auth.go` before any identity header read
</success_criteria>

<output>
After completion, create `.planning/phases/01-project-foundation-and-security-skeleton/01-04-SUMMARY.md` with:
- Auth middleware function signature and parameters
- UserIDFromContext signature and UserIDKey export name
- HTTP mux route table (path, auth-wrapped y/n, handler)
- /readyz check names and their meaning
- Confirmation auth middleware test results
- Note about WithHTTPContextFunc behavior (per-request confirmed via ARCHITECTURE.md)
</output>
