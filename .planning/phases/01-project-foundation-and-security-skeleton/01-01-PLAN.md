---
phase: 01-project-foundation-and-security-skeleton
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - go.mod
  - go.sum
  - cmd/polar-flow-mcp/main.go
  - internal/config/config.go
  - internal/auth/auth.go
  - internal/store/store.go
  - internal/crypto/crypto.go
  - internal/polar/client.go
  - internal/oauth/oauth.go
  - internal/mcp/mcp.go
  - Makefile
  - .golangci.yml
autonomous: true
requirements:
  - FOUND-01

must_haves:
  truths:
    - "CGO_ENABLED=0 go build ./... succeeds with no errors"
    - "All 8 internal packages compile with correct import paths"
    - "golangci-lint runs and produces no errors on the scaffold code"
    - "go test ./... exits 0 on the empty scaffold"
  artifacts:
    - path: "go.mod"
      provides: "Module declaration with all 4 external dependencies"
      contains: "module github.com/lm/polar-flow-mcp"
    - path: "cmd/polar-flow-mcp/main.go"
      provides: "Wiring entrypoint (no business logic)"
      min_lines: 10
    - path: "internal/config/config.go"
      provides: "Config struct stub"
    - path: "internal/auth/auth.go"
      provides: "Auth package stub with unexported context key type"
    - path: "internal/store/store.go"
      provides: "Store package stub"
    - path: "internal/crypto/crypto.go"
      provides: "Crypto package stub with KeyProvider interface"
    - path: "internal/polar/client.go"
      provides: "Polar client stub"
    - path: "internal/oauth/oauth.go"
      provides: "OAuth handler stubs"
    - path: "internal/mcp/mcp.go"
      provides: "MCP tool registration stub"
    - path: "Makefile"
      provides: "build/test/lint targets"
    - path: ".golangci.yml"
      provides: "golangci-lint v2.11 config mirroring karaclean"
  key_links:
    - from: "cmd/polar-flow-mcp/main.go"
      to: "internal/config"
      via: "import github.com/lm/polar-flow-mcp/internal/config"
      pattern: "polar-flow-mcp/internal/config"
    - from: "internal/store/store.go"
      to: "internal/crypto"
      via: "import github.com/lm/polar-flow-mcp/internal/crypto"
      pattern: "polar-flow-mcp/internal/crypto"
---

<objective>
Initialize the Go module, establish all 8 internal package stubs with correct import paths, and wire up the Makefile and golangci-lint config mirroring karaclean.

Purpose: Establish the project skeleton so every subsequent plan can add real implementation without structural rework. This plan creates compilable stubs only — no logic yet.

Output: Compilable Go module with correct dependency declarations, package layout matching `cmd/polar-flow-mcp/main.go` + `internal/{config,auth,store,crypto,polar,oauth,mcp}/`, Makefile with `build`/`test`/`lint` targets, `.golangci.yml` mirroring karaclean.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: Initialize go.mod and create package stubs</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (decisions D-08, D-09, D-12, D-15)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/STACK.md (dependency versions and DO NOT USE list)
  </read_first>
  <files>
    go.mod,
    go.sum,
    cmd/polar-flow-mcp/main.go,
    internal/config/config.go,
    internal/auth/auth.go,
    internal/store/store.go,
    internal/crypto/crypto.go,
    internal/polar/client.go,
    internal/oauth/oauth.go,
    internal/mcp/mcp.go
  </files>
  <action>
Run `go mod init github.com/lm/polar-flow-mcp` then add the 4 external dependencies at latest versions:

```
go get github.com/mark3labs/mcp-go@latest
go get modernc.org/sqlite@latest
go get github.com/golang-migrate/migrate/v4@latest
go get golang.org/x/oauth2@latest
```

Also run `go get github.com/golang-migrate/migrate/v4/database/sqlite` (the pure-Go driver — NOT sqlite3).

Create the directory tree and minimal stub files:

**cmd/polar-flow-mcp/main.go** — package main with a `main()` that prints "polar-flow-mcp starting" and exits; imports only standard library. This is intentionally minimal — Plan 02 adds config loading.

```go
package main

import (
    "log/slog"
    "os"
)

func main() {
    slog.Info("polar-flow-mcp starting")
    os.Exit(0)
}
```

**internal/config/config.go** — package config, empty `Config` struct, stub `Load()` function returning `(*Config, error)`:

```go
package config

// Config holds all validated server configuration loaded from environment variables.
type Config struct{}

// Load reads environment variables, validates required fields, and returns a populated Config.
// Returns an error (and calls os.Exit(1)) if any required field is missing or invalid.
func Load() (*Config, error) {
    return &Config{}, nil
}
```

**internal/auth/auth.go** — package auth with the unexported context key type (D-12) and a stub Middleware function:

```go
package auth

import "net/http"

// userIDKey is the unexported context key type for user identity.
// Using an unexported struct type prevents cross-package key collisions.
type userIDKey struct{}

// UserIDKey is the singleton key instance used with context.WithValue.
var UserIDKey = userIDKey{}

// UserIDFromContext extracts the user identity string from the context.
// Returns ("", false) if the identity is not present, forcing call sites to handle the missing case.
func UserIDFromContext(ctx interface{ Value(any) any }) (string, bool) {
    v, ok := ctx.Value(userIDKey{}).(string)
    return v, ok
}

// Middleware returns an http.Handler that enforces proxy authentication.
// Stub: passes through all requests. Real logic added in Plan 04.
func Middleware(secret, identityHeader string, next http.Handler) http.Handler {
    return next
}
```

Note: `UserIDFromContext` must accept `context.Context`. Replace the `interface{ Value(any) any }` with `context.Context` and add `"context"` import.

**internal/crypto/crypto.go** — package crypto with `KeyProvider` interface and stubbed `Cipher` type:

```go
package crypto

// KeyProvider is the interface for loading the AES-256-GCM encryption key.
// Phase 1 ships two implementations: env (reads ENCRYPTION_KEY) and file (reads ENCRYPTION_KEY_FILE).
type KeyProvider interface {
    // Key returns the 32-byte AES-256 key. Implementations may cache or re-read on each call.
    Key() ([]byte, error)
}

// Cipher wraps a KeyProvider and provides AES-256-GCM encrypt/decrypt.
type Cipher struct {
    provider KeyProvider
}

// NewCipher creates a new Cipher backed by the given KeyProvider.
func NewCipher(p KeyProvider) *Cipher {
    return &Cipher{provider: p}
}

// Encrypt encrypts plaintext with AES-256-GCM. Returns nonce(12)||ciphertext as a single []byte.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
    return nil, nil // stub
}

// Decrypt decrypts a blob of the form nonce(12)||ciphertext produced by Encrypt.
func (c *Cipher) Decrypt(blob []byte) ([]byte, error) {
    return nil, nil // stub
}
```

**internal/store/store.go** — package store with stub `Store` type:

```go
package store

// Store holds the SQLite read and write connection pools.
type Store struct{}

// Open opens the SQLite database, sets WAL mode and dual pools, and runs migrations.
// Stub: real implementation in Plan 03.
func Open(dsn string) (*Store, error) {
    return &Store{}, nil
}

// Close closes both connection pools.
func (s *Store) Close() error {
    return nil
}
```

**internal/polar/client.go** — package polar with stub `Client` type:

```go
package polar

import "net/http"

// Client is a typed HTTP client for the Polar AccessLink v3 API.
// Created fresh per tool invocation; never cached at server or session level.
type Client struct {
    httpClient *http.Client
    bearerToken string
}

// NewClient creates a new Polar API client for the given bearer token.
func NewClient(bearerToken string) *Client {
    return &Client{
        httpClient:  &http.Client{Timeout: 30 * 1e9}, // 30s in nanoseconds
        bearerToken: bearerToken,
    }
}
```

**internal/oauth/oauth.go** — package oauth with stub handler functions:

```go
package oauth

import "net/http"

// LoginHandler initiates the Polar OAuth2 authorization flow.
// Stub: real implementation in Phase 2.
func LoginHandler(w http.ResponseWriter, r *http.Request) {
    http.Error(w, "not implemented", http.StatusNotImplemented)
}

// CallbackHandler handles the Polar OAuth2 callback.
// Stub: real implementation in Phase 2.
func CallbackHandler(w http.ResponseWriter, r *http.Request) {
    http.Error(w, "not implemented", http.StatusNotImplemented)
}
```

**internal/mcp/mcp.go** — package mcp with stub RegisterTools:

```go
package mcp

import "github.com/mark3labs/mcp-go/server"

// RegisterTools registers all MCP tools with the given server.
// Stub: real implementations added in Phase 2 (get_user_info) and Phase 3 (training target tools).
func RegisterTools(s *server.MCPServer) {
}
```

After creating all files, run:
```
CGO_ENABLED=0 go build ./...
```
Fix any import or syntax errors until the build is clean.
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && CGO_ENABLED=0 go build ./...</automated>
  </verify>
  <acceptance_criteria>
    - `go.mod` first line is `module github.com/lm/polar-flow-mcp`
    - `go.mod` contains `modernc.org/sqlite` (not mattn/go-sqlite3)
    - `go.mod` contains `github.com/golang-migrate/migrate/v4`
    - `go.mod` contains `github.com/mark3labs/mcp-go`
    - `go.mod` contains `golang.org/x/oauth2`
    - `CGO_ENABLED=0 go build ./...` exits 0 with no error output
    - All 8 files exist: cmd/polar-flow-mcp/main.go, internal/config/config.go, internal/auth/auth.go, internal/store/store.go, internal/crypto/crypto.go, internal/polar/client.go, internal/oauth/oauth.go, internal/mcp/mcp.go
    - `grep -r "mattn/go-sqlite3" .` returns empty (no CGO sqlite driver imported)
    - `internal/auth/auth.go` contains `type userIDKey struct{}`
    - `internal/crypto/crypto.go` contains `type KeyProvider interface`
  </acceptance_criteria>
  <done>go.mod declares correct module path and 4 external deps; all 8 packages compile cleanly with CGO_ENABLED=0; unexported context key type and KeyProvider interface stubs are in place.</done>
</task>

<task type="auto">
  <name>Task 2: Makefile and golangci-lint config</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (D-15: golangci-lint v2.11 mirroring karaclean exactly)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/STACK.md (golangci-lint v2.11, linters: gocyclo/godot/misspell/noctx/errcheck)
  </read_first>
  <files>
    Makefile,
    .golangci.yml
  </files>
  <action>
Create **Makefile** with the following targets. The `test` target must use `CGO_ENABLED=0` and `-race` to catch CGO leakage and data races at test time:

```makefile
.PHONY: build test lint clean

build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/polar-flow-mcp ./cmd/polar-flow-mcp

test:
	CGO_ENABLED=0 go test -race -count=1 ./...

lint:
	~/go/bin/golangci-lint run ./...

clean:
	rm -rf bin/
```

Create **.golangci.yml** mirroring karaclean v2.11 config exactly. Enable these linters beyond the default set: `gocyclo`, `godot`, `misspell`, `noctx`. Enable `errcheck` with type assertion checking. Use `v2` linters format:

```yaml
version: "2"

linters:
  enable:
    - gocyclo
    - godot
    - misspell
    - noctx
  settings:
    gocyclo:
      min-complexity: 15
    errcheck:
      check-type-assertions: true

issues:
  exclude-rules:
    - path: "_test\\.go"
      linters:
        - godot
```

After creating both files, run `~/go/bin/golangci-lint run ./...` and fix any lint errors in the stub files (common: missing doc comments on exported symbols for `godot`, missing error checks).
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && ~/go/bin/golangci-lint run ./...</automated>
  </verify>
  <acceptance_criteria>
    - `Makefile` exists and contains targets: `build`, `test`, `lint`, `clean`
    - `Makefile` `test` target contains `CGO_ENABLED=0` and `-race`
    - `.golangci.yml` contains `version: "2"`
    - `.golangci.yml` enables `gocyclo`, `godot`, `misspell`, `noctx`
    - `.golangci.yml` contains `check-type-assertions: true`
    - `~/go/bin/golangci-lint run ./...` exits 0 with no lint errors
    - `make build` exits 0 and produces `bin/polar-flow-mcp`
  </acceptance_criteria>
  <done>Makefile and .golangci.yml created; `make lint` and `make build` pass with zero errors.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| developer→CI | go.mod dependency declarations must exclude CGO-requiring packages |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-01-01 | Tampering | go.mod | mitigate | `go.sum` pins exact module hashes; `CGO_ENABLED=0 go build` fails if a CGO dependency slips in |
| T-01-02 | Information Disclosure | internal/auth/auth.go | mitigate | Context key is an unexported struct type — zero probability of cross-package key collision; enforced in this plan |
| T-01-03 | Elevation of Privilege | dependency supply chain | accept | Module proxy and sum database enforce hash pinning; acceptable residual risk at scaffold stage |
</threat_model>

<verification>
```bash
cd /var/home/lm/git/polar-flow-mcp
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test -race -count=1 ./...
~/go/bin/golangci-lint run ./...
grep "module github.com/lm/polar-flow-mcp" go.mod
grep "modernc.org/sqlite" go.mod
grep -r "mattn/go-sqlite3" . && echo "FAIL: CGO driver found" || echo "OK: no CGO driver"
grep "type userIDKey struct{}" internal/auth/auth.go
grep "type KeyProvider interface" internal/crypto/crypto.go
```
</verification>

<success_criteria>
- `CGO_ENABLED=0 go build ./...` exits 0
- `CGO_ENABLED=0 go test -race -count=1 ./...` exits 0
- `~/go/bin/golangci-lint run ./...` exits 0
- `go.mod` declares `module github.com/lm/polar-flow-mcp` and lists all 4 external deps
- No `mattn/go-sqlite3` import anywhere in the codebase
- `internal/auth` contains unexported `userIDKey` struct type
- `internal/crypto` contains `KeyProvider` interface definition
- `make build`, `make test`, `make lint` all exit 0
</success_criteria>

<output>
After completion, create `.planning/phases/01-project-foundation-and-security-skeleton/01-01-SUMMARY.md` with:
- Files created and their purpose
- External dependency versions pinned in go.mod
- Any lint suppressions added and why
- Confirmation that CGO_ENABLED=0 build passes
</output>
