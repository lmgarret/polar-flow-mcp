---
phase: 01-project-foundation-and-security-skeleton
plan: 02
type: execute
wave: 2
depends_on:
  - 01-PLAN-01-go-scaffold.md
files_modified:
  - internal/config/config.go
  - internal/config/config_test.go
  - cmd/polar-flow-mcp/main.go
autonomous: true
requirements:
  - FOUND-02
  - FOUND-03
  - FOUND-04
  - FOUND-10

must_haves:
  truths:
    - "Server exits non-zero and prints actionable message when AUTH_PROXY=unconfigured"
    - "Server exits non-zero and prints actionable message when PROXY_SHARED_SECRET is empty"
    - "Server exits non-zero and prints actionable message when no encryption key is loadable"
    - "Startup log names the trusted identity header, required secret header, and declared auth proxy"
    - "Config unit tests pass covering all fail-closed cases"
  artifacts:
    - path: "internal/config/config.go"
      provides: "Full Config struct with Load() validation and fail-closed checks"
      exports: ["Config", "Load"]
      contains: "AUTH_PROXY"
    - path: "internal/config/config_test.go"
      provides: "Unit tests for all fail-closed validation paths"
      min_lines: 60
    - path: "cmd/polar-flow-mcp/main.go"
      provides: "Updated main() that calls config.Load() and exits on error"
  key_links:
    - from: "cmd/polar-flow-mcp/main.go"
      to: "internal/config"
      via: "config.Load() return value checked; os.Exit(1) on error"
      pattern: "config\\.Load\\(\\)"
---

<objective>
Implement the full `config` package with fail-closed validation for `AUTH_PROXY`, `PROXY_SHARED_SECRET`, and encryption key loading. Update `main.go` to call `config.Load()` at startup and exit 1 with a structured `slog.Error` message on any validation failure. Write unit tests for every fail-closed path.

Purpose: FOUND-02, FOUND-03, FOUND-04 require the server to refuse to start with actionable error messages. FOUND-10 requires a startup security banner. These are the fail-closed invariants that every downstream phase depends on.

Output: `internal/config/config.go` with typed `Config` struct and validated `Load()`, `internal/config/config_test.go` covering all failure paths, updated `cmd/polar-flow-mcp/main.go`.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md

<!-- Summary from Plan 01 needed to understand current stub state -->
@.planning/phases/01-project-foundation-and-security-skeleton/01-01-SUMMARY.md
</context>

<interfaces>
<!-- Types established in Plan 01 that this plan must build on -->

From internal/config/config.go (Plan 01 stub):
```go
package config

type Config struct{}

func Load() (*Config, error) {
    return &Config{}, nil
}
```
This plan replaces the stub with the full implementation.

From internal/crypto/crypto.go (Plan 01 stub — KeyProvider interface, read before referencing):
```go
type KeyProvider interface {
    Key() ([]byte, error)
}
```
Config must declare `KeyProvider` field of type `crypto.KeyProvider` OR config only stores the key bytes ([]byte) and returns them, with crypto plan (Plan 03) owning the KeyProvider implementations.

Decision (per D-09, specifics section): `KeyProvider` interface lives in `internal/crypto`, not `internal/config`. `config.Config` stores `KeyProviderType string` ("env" or "file") and `EncryptionKeyRaw string`/`EncryptionKeyFile string`. `crypto.NewKeyProvider(cfg)` constructs the implementation. Config validation checks the key is loadable by calling a validation helper — OR config validates the env var presence only (not decoding). Use approach: Config.Load() validates that either ENCRYPTION_KEY is a non-empty base64-decodable 32-byte value OR ENCRYPTION_KEY_FILE points to a readable file with 32 bytes. Either passes.
</interfaces>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Config struct, Load(), and fail-closed validation</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/internal/config/config.go (current stub — read before overwriting)
    - /var/home/lm/git/polar-flow-mcp/.planning/REQUIREMENTS.md (FOUND-02, FOUND-03, FOUND-04, FOUND-10 exact text)
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (D-02, D-04, decisions section)
  </read_first>
  <files>
    internal/config/config.go,
    internal/config/config_test.go
  </files>
  <behavior>
    - Test 1: Load() with AUTH_PROXY=unconfigured returns error containing "AUTH_PROXY" and a docs pointer string
    - Test 2: Load() with AUTH_PROXY="" (empty) returns error containing "AUTH_PROXY"
    - Test 3: Load() with AUTH_PROXY set but PROXY_SHARED_SECRET="" returns error containing "PROXY_SHARED_SECRET"
    - Test 4: Load() with valid AUTH_PROXY + PROXY_SHARED_SECRET but no ENCRYPTION_KEY and no ENCRYPTION_KEY_FILE returns error containing "ENCRYPTION_KEY" and the openssl generation hint
    - Test 5: Load() with ENCRYPTION_KEY set to a valid base64 32-byte value succeeds
    - Test 6: Load() with ENCRYPTION_KEY set to a valid base64 value that decodes to != 32 bytes returns error
    - Test 7: Load() with ENCRYPTION_KEY_FILE pointing to a temp file containing 32 bytes succeeds
    - Test 8: Load() BIND_ADDRESS defaults to "127.0.0.1" when not set
    - Test 9: Load() IDENTITY_HEADER defaults to "Remote-User" when not set (per D-02)
  </behavior>
  <action>
Replace the stub `internal/config/config.go` with the full implementation:

**Config struct** — all fields exported, with field names matching their env var source:

```go
type Config struct {
    // Auth proxy identification
    AuthProxy string // AUTH_PROXY env var; must not be "unconfigured"

    // Proxy shared secret (the real security boundary)
    ProxySharedSecret string // PROXY_SHARED_SECRET env var; must not be empty

    // Identity header injected by the reverse proxy
    IdentityHeader string // IDENTITY_HEADER env var; defaults to "Remote-User"

    // Bind address
    BindAddress string // BIND_ADDRESS env var; defaults to "127.0.0.1"

    // Encryption key configuration
    KeyProviderType    string // KEY_PROVIDER env var; "env" or "file" (default "env")
    EncryptionKeyB64   string // ENCRYPTION_KEY env var; base64-encoded 32-byte key
    EncryptionKeyFile  string // ENCRYPTION_KEY_FILE env var; path to key file
}
```

**Load() function** — reads from `os.Getenv`, validates, returns typed error or nil:

1. Read `AUTH_PROXY` from env. If empty or equals `"unconfigured"`: return error:
   `"AUTH_PROXY must be set to the name of your reverse proxy (e.g. authelia, authentik, oauth2-proxy); see https://github.com/lm/polar-flow-mcp/docs/deployment for configuration"`

2. Read `PROXY_SHARED_SECRET`. If empty: return error:
   `"PROXY_SHARED_SECRET must be set to a non-empty secret shared with your reverse proxy; see deployment docs"`

3. Read `IDENTITY_HEADER`. Default to `"Remote-User"` if not set.

4. Read `BIND_ADDRESS`. Default to `"127.0.0.1"` if not set.

5. Read `KEY_PROVIDER`. Default to `"env"`. Validate it is `"env"` or `"file"`.

6. For key validation:
   - If KEY_PROVIDER=env: read `ENCRYPTION_KEY`. If empty: error with `"ENCRYPTION_KEY must be set to a base64-encoded 32-byte key; generate one with: openssl rand -base64 32"`. Try to base64-decode it; if result is not 32 bytes: error `"ENCRYPTION_KEY decoded length must be 32 bytes (got N); regenerate with: openssl rand -base64 32"`.
   - If KEY_PROVIDER=file: read `ENCRYPTION_KEY_FILE`. If empty: error with `"ENCRYPTION_KEY_FILE must be set to the path of a file containing a 32-byte key"`. Try to read the file; if error or length != 32 bytes: return descriptive error.
   - Store the raw key bytes ([]byte) in the Config as `EncryptionKey []byte` (unexported if you prefer, but exported is fine for tests).

7. Return the populated `*Config, nil`.

**Startup banner function** — separate `LogStartupBanner(cfg *Config)` that calls:
```go
slog.Info("polar-flow-mcp starting",
    "auth_proxy", cfg.AuthProxy,
    "identity_header", cfg.IdentityHeader,
    "secret_header", "X-Proxy-Secret",  // the header name the client must send
    "bind_address", cfg.BindAddress,
)
```
The "secret header name" is a constant `ProxySecretHeader = "X-Proxy-Secret"` defined in the config package (or auth package — put it where it is least likely to cycle; config is fine since auth imports config).

Add `const ProxySecretHeader = "X-Proxy-Secret"` to config.go.

Write `internal/config/config_test.go` using stdlib `testing` (no testify). Use `t.Setenv()` to set environment variables in tests. Test every behavior listed above.
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && CGO_ENABLED=0 go test -race -count=1 ./internal/config/...</automated>
  </verify>
  <acceptance_criteria>
    - `internal/config/config.go` contains `type Config struct` with all fields
    - `internal/config/config.go` contains `const ProxySecretHeader = "X-Proxy-Secret"`
    - `internal/config/config.go` contains `func LogStartupBanner`
    - `internal/config/config_test.go` exists with at least 9 test functions
    - `grep -c "func Test" internal/config/config_test.go` returns 9 or more
    - `CGO_ENABLED=0 go test -race -count=1 ./internal/config/...` exits 0
    - Error message for AUTH_PROXY=unconfigured contains both "AUTH_PROXY" and "docs"
    - Error message for missing ENCRYPTION_KEY contains "openssl rand -base64 32"
    - `~/go/bin/golangci-lint run ./internal/config/...` exits 0
  </acceptance_criteria>
  <done>config.Load() validates all required fields and returns actionable errors; all 9 unit tests pass; lint clean.</done>
</task>

<task type="auto">
  <name>Task 2: Wire config.Load() into main.go with startup banner and exit-on-error</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/cmd/polar-flow-mcp/main.go (current stub — read before overwriting)
    - /var/home/lm/git/polar-flow-mcp/internal/config/config.go (Load() and LogStartupBanner signatures — just written)
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (fail-closed pattern: slog.Error + os.Exit(1))
  </read_first>
  <files>
    cmd/polar-flow-mcp/main.go
  </files>
  <action>
Replace the stub `main.go` with an implementation that:

1. Calls `config.Load()`.
2. On error: calls `slog.Error("startup failed", "error", err)` then `os.Exit(1)`.
3. On success: calls `config.LogStartupBanner(cfg)` with the loaded config.
4. If `cfg.BindAddress` is not `"127.0.0.1"` and not `"::1"`: emit `slog.Warn("BIND_ADDRESS is not localhost — ensure PROXY_SHARED_SECRET is set and the server is not directly reachable without the reverse proxy", "bind_address", cfg.BindAddress)` (per Pitfall 8 / SERV-05).
5. Stub `slog.Info("server ready", "bind_address", cfg.BindAddress)` — the actual HTTP server is wired in Plan 04.
6. For now: after the banner, just `select {}` to block (Plan 04 replaces this with the real HTTP server).

The `main()` function must import:
- `"log/slog"`
- `"os"`
- `"github.com/lm/polar-flow-mcp/internal/config"`

No other imports needed in this plan. Do NOT import `net/http` yet (Plan 04 does that).
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && CGO_ENABLED=0 go build ./cmd/polar-flow-mcp && echo "build OK"</automated>
  </verify>
  <acceptance_criteria>
    - `cmd/polar-flow-mcp/main.go` imports `github.com/lm/polar-flow-mcp/internal/config`
    - `cmd/polar-flow-mcp/main.go` contains `config.Load()`
    - `cmd/polar-flow-mcp/main.go` contains `slog.Error` followed by `os.Exit(1)` in the error path
    - `cmd/polar-flow-mcp/main.go` contains `config.LogStartupBanner`
    - `cmd/polar-flow-mcp/main.go` contains the BIND_ADDRESS non-loopback warning
    - `CGO_ENABLED=0 go build ./cmd/polar-flow-mcp` exits 0
    - `~/go/bin/golangci-lint run ./cmd/...` exits 0
  </acceptance_criteria>
  <done>main.go calls config.Load(), exits 1 with slog.Error on validation failure, emits startup banner on success, warns on non-loopback bind address.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| environment→process | Config is loaded from environment variables; malformed or missing values must result in non-start |
| operator→server | AUTH_PROXY default "unconfigured" forces operators to make a conscious decision before the server accepts any request |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-02-01 | Spoofing | AUTH_PROXY default | mitigate | Server refuses to start if AUTH_PROXY="unconfigured"; enforced in config.Load() before any network listener opens |
| T-02-02 | Spoofing | PROXY_SHARED_SECRET empty | mitigate | Server refuses to start if PROXY_SHARED_SECRET is empty; enforced in config.Load() |
| T-02-03 | Information Disclosure | ENCRYPTION_KEY in env | accept | Key in env is standard practice for containerized workloads; key-in-file alternative provided; Vault/KMS deferred to v2 |
| T-02-04 | Elevation of Privilege | BIND_ADDRESS=0.0.0.0 | mitigate | Startup WARN log emitted if bind address is not loopback; /readyz check in Plan 04 enforces healthy-only-when-configured |
| T-02-05 | Denial of Service | missing encryption key | mitigate | Server exits 1 immediately with actionable error rather than starting in a degraded state that fails on first tool call |
</threat_model>

<verification>
```bash
cd /var/home/lm/git/polar-flow-mcp

# Unit tests
CGO_ENABLED=0 go test -race -count=1 ./internal/config/...

# Build
CGO_ENABLED=0 go build ./...

# Lint
~/go/bin/golangci-lint run ./...

# Verify fail-closed: server must exit 1 with AUTH_PROXY=unconfigured
AUTH_PROXY=unconfigured ./bin/polar-flow-mcp; echo "exit code: $?"

# Verify startup banner fields
AUTH_PROXY=authelia PROXY_SHARED_SECRET=secret ENCRYPTION_KEY=$(openssl rand -base64 32) ./bin/polar-flow-mcp 2>&1 | grep "auth_proxy"
```
</verification>

<success_criteria>
- `CGO_ENABLED=0 go test -race -count=1 ./internal/config/...` exits 0
- `CGO_ENABLED=0 go build ./...` exits 0
- `~/go/bin/golangci-lint run ./...` exits 0
- Running binary with `AUTH_PROXY=unconfigured` exits 1 and output contains "AUTH_PROXY"
- Running binary with `PROXY_SHARED_SECRET=` exits 1 and output contains "PROXY_SHARED_SECRET"
- Running binary with missing ENCRYPTION_KEY exits 1 and output contains "openssl rand -base64 32"
- Startup log with valid config contains fields: `auth_proxy`, `identity_header`, `bind_address`
</success_criteria>

<output>
After completion, create `.planning/phases/01-project-foundation-and-security-skeleton/01-02-SUMMARY.md` with:
- Full Config struct field list and their env var names
- ProxySecretHeader constant value
- Error message strings (exact text) for each fail-closed path
- Confirmation all 9+ config unit tests pass
</output>
