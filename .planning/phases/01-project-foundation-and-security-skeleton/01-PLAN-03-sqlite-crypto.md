---
phase: 01-project-foundation-and-security-skeleton
plan: 03
type: execute
wave: 2
depends_on:
  - 01-PLAN-01-go-scaffold.md
files_modified:
  - internal/crypto/crypto.go
  - internal/crypto/crypto_test.go
  - internal/crypto/env_provider.go
  - internal/crypto/file_provider.go
  - internal/store/store.go
  - internal/store/store_test.go
  - migrations/001_initial.up.sql
  - migrations/001_initial.down.sql
autonomous: true
requirements:
  - FOUND-05
  - FOUND-06
  - FOUND-07
  - FOUND-08
  - FOUND-09

must_haves:
  truths:
    - "AES-256-GCM encrypt/decrypt round-trips correctly for arbitrary plaintext"
    - "1000 sequential Encrypt() calls produce 1000 distinct nonces (no reuse)"
    - "Encrypted blob is always nonce(12 bytes) || ciphertext — never split into separate fields"
    - "SQLite opens with WAL mode, busy_timeout=5000, foreign_keys=ON"
    - "Dual connection pools: write MaxOpenConns(1), read MaxOpenConns(4)"
    - "Schema migrations run at store.Open() and create users, polar_tokens, pending_auth tables"
    - "KeyProvider env impl reads ENCRYPTION_KEY base64-decoded 32-byte key"
    - "KeyProvider file impl reads 32-byte key from path in ENCRYPTION_KEY_FILE"
  artifacts:
    - path: "internal/crypto/crypto.go"
      provides: "Cipher.Encrypt() and Cipher.Decrypt() using AES-256-GCM with crypto/rand nonces"
      exports: ["KeyProvider", "Cipher", "NewCipher"]
    - path: "internal/crypto/env_provider.go"
      provides: "EnvKeyProvider reading ENCRYPTION_KEY base64 value"
    - path: "internal/crypto/file_provider.go"
      provides: "FileKeyProvider reading key from ENCRYPTION_KEY_FILE path"
    - path: "internal/crypto/crypto_test.go"
      provides: "Unit tests including 1000-nonce uniqueness test (D-06)"
      min_lines: 80
    - path: "internal/store/store.go"
      provides: "Store.Open() with WAL dual-pool setup and golang-migrate migrations"
    - path: "internal/store/store_test.go"
      provides: "Integration test verifying WAL mode and both pools respond"
    - path: "migrations/001_initial.up.sql"
      provides: "users, polar_tokens, pending_auth table DDL"
      contains: "CREATE TABLE users"
    - path: "migrations/001_initial.down.sql"
      provides: "DROP TABLE statements for rollback"
  key_links:
    - from: "internal/store/store.go"
      to: "migrations/001_initial.up.sql"
      via: "//go:embed migrations/*.sql embedded filesystem"
      pattern: "go:embed migrations"
    - from: "internal/crypto/crypto.go"
      to: "AES-256-GCM nonce storage"
      via: "Encrypt() prepends 12-byte nonce to ciphertext in returned []byte"
      pattern: "nonce.*ciphertext\\|\\|nonce"
---

<objective>
Implement the `crypto` package (AES-256-GCM Cipher with both KeyProvider implementations) and the `store` package (SQLite dual-pool open with WAL pragmas, embedded migrations creating the 3-table schema). Write unit tests including the mandatory 1000-nonce uniqueness test (D-06) and the dual-pool readiness integration test (D-07).

Purpose: FOUND-05..09 are the irreversible data-layer decisions. The BLOB schema, nonce prepending convention, and WAL pool setup cannot be changed after data is written. These must be correct and tested before any OAuth or MCP work begins.

Output: `internal/crypto/` with full Cipher + two KeyProvider impls + tests; `internal/store/` with WAL dual-pool open + migrations + tests; `migrations/001_initial.up.sql` with 3-table schema.
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
</context>

<interfaces>
<!-- Contracts this plan must produce for downstream consumers (Plans 04, Phase 2) -->

From internal/config/config.go (Plan 02 output — read after Plan 02 completes):
```go
type Config struct {
    KeyProviderType   string  // "env" or "file"
    EncryptionKeyB64  string  // base64-encoded key (when KeyProviderType=="env")
    EncryptionKeyFile string  // file path (when KeyProviderType=="file")
    EncryptionKey     []byte  // pre-validated 32 bytes
}
```

This plan's crypto package MUST export:
```go
// KeyProvider interface (defined in Plan 01 stub — this plan provides implementations)
type KeyProvider interface {
    Key() ([]byte, error)
}

// Cipher is the AES-256-GCM encryptor/decryptor
type Cipher struct { ... }
func NewCipher(p KeyProvider) *Cipher
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error)  // returns nonce(12)||ciphertext
func (c *Cipher) Decrypt(blob []byte) ([]byte, error)       // expects nonce(12)||ciphertext input

// KeyProvider implementations
type EnvKeyProvider struct{ ... }
func NewEnvKeyProvider(b64key string) *EnvKeyProvider

type FileKeyProvider struct{ ... }
func NewFileKeyProvider(path string) *FileKeyProvider
```

This plan's store package MUST export:
```go
type Store struct {
    writeDB *sql.DB
    readDB  *sql.DB
}
func Open(dsn string) (*Store, error)
func (s *Store) Close() error
func (s *Store) WriteDB() *sql.DB  // for use by oauth and mcp packages
func (s *Store) ReadDB() *sql.DB
```
</interfaces>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: AES-256-GCM Cipher and KeyProvider implementations</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/internal/crypto/crypto.go (current stub — read before overwriting)
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (D-06: 1000-nonce uniqueness, D-10: nonce||ciphertext BLOB layout)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/PITFALLS.md (Pitfall 2: AES-GCM nonce reuse — entire section)
  </read_first>
  <files>
    internal/crypto/crypto.go,
    internal/crypto/env_provider.go,
    internal/crypto/file_provider.go,
    internal/crypto/crypto_test.go
  </files>
  <behavior>
    - Test 1: Encrypt([]byte("hello")) produces a blob of length > 12 with no error
    - Test 2: Decrypt(Encrypt(plaintext)) round-trips correctly for 100-byte plaintext
    - Test 3: Decrypt on a malformed blob (< 12 bytes) returns an error, not a panic
    - Test 4: Decrypt with wrong key returns error (GCM authentication failure)
    - Test 5 (D-06): Call Encrypt() 1000 times; collect all nonces (first 12 bytes of each blob); assert all 1000 are distinct — no map entry appears more than once
    - Test 6: EnvKeyProvider.Key() returns the decoded bytes for a valid base64 32-byte key
    - Test 7: EnvKeyProvider.Key() returns error if the stored key decodes to != 32 bytes
    - Test 8: FileKeyProvider.Key() returns 32 bytes from a tempfile containing exactly 32 random bytes
    - Test 9: FileKeyProvider.Key() returns error if the file does not exist
  </behavior>
  <action>
Replace stub `internal/crypto/crypto.go` with the full implementation:

```go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    cryptorand "crypto/rand"
    "errors"
    "fmt"
    "io"
)

const nonceSize = 12 // AES-256-GCM standard nonce size (bytes)

// KeyProvider is the interface for loading the AES-256-GCM encryption key.
type KeyProvider interface {
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

// Encrypt encrypts plaintext with AES-256-GCM.
// Returns a blob of the form: nonce (12 bytes) || GCM ciphertext.
// A unique crypto/rand nonce is generated for every call.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
    key, err := c.provider.Key()
    if err != nil {
        return nil, fmt.Errorf("crypto: load key: %w", err)
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, fmt.Errorf("crypto: create cipher block: %w", err)
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("crypto: create GCM: %w", err)
    }
    nonce := make([]byte, nonceSize)
    if _, err := io.ReadFull(cryptorand.Reader, nonce); err != nil {
        return nil, fmt.Errorf("crypto: generate nonce: %w", err)
    }
    ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
    // Return nonce || ciphertext as a single blob — never split.
    return append(nonce, ciphertext...), nil
}

// Decrypt decrypts a blob produced by Encrypt.
// Input must be at least nonceSize bytes; the first 12 bytes are the nonce.
func (c *Cipher) Decrypt(blob []byte) ([]byte, error) {
    if len(blob) < nonceSize {
        return nil, errors.New("crypto: blob too short to contain nonce")
    }
    key, err := c.provider.Key()
    if err != nil {
        return nil, fmt.Errorf("crypto: load key: %w", err)
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, fmt.Errorf("crypto: create cipher block: %w", err)
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("crypto: create GCM: %w", err)
    }
    nonce := blob[:nonceSize]
    ciphertext := blob[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, fmt.Errorf("crypto: decrypt: %w", err)
    }
    return plaintext, nil
}
```

Create `internal/crypto/env_provider.go`:

```go
package crypto

import (
    "encoding/base64"
    "fmt"
)

// EnvKeyProvider provides an AES-256 key from a base64-encoded string (typically from an env var).
type EnvKeyProvider struct {
    b64key string
}

// NewEnvKeyProvider creates a KeyProvider backed by the given base64-encoded key string.
func NewEnvKeyProvider(b64key string) *EnvKeyProvider {
    return &EnvKeyProvider{b64key: b64key}
}

// Key decodes the base64 key and validates it is exactly 32 bytes.
func (p *EnvKeyProvider) Key() ([]byte, error) {
    key, err := base64.StdEncoding.DecodeString(p.b64key)
    if err != nil {
        // Try URL-safe base64 as fallback (openssl rand -base64 uses standard, but be tolerant).
        key, err = base64.URLEncoding.DecodeString(p.b64key)
        if err != nil {
            return nil, fmt.Errorf("crypto: decode ENCRYPTION_KEY: not valid base64")
        }
    }
    if len(key) != 32 {
        return nil, fmt.Errorf("crypto: ENCRYPTION_KEY decoded to %d bytes, want 32", len(key))
    }
    return key, nil
}
```

Create `internal/crypto/file_provider.go`:

```go
package crypto

import (
    "fmt"
    "os"
)

// FileKeyProvider provides an AES-256 key from a file on disk.
type FileKeyProvider struct {
    path string
}

// NewFileKeyProvider creates a KeyProvider that reads a 32-byte key from the given file path.
func NewFileKeyProvider(path string) *FileKeyProvider {
    return &FileKeyProvider{path: path}
}

// Key reads the file and returns its contents as the key.
// The file must contain exactly 32 bytes.
func (p *FileKeyProvider) Key() ([]byte, error) {
    data, err := os.ReadFile(p.path)
    if err != nil {
        return nil, fmt.Errorf("crypto: read key file %q: %w", p.path, err)
    }
    if len(data) != 32 {
        return nil, fmt.Errorf("crypto: key file %q contains %d bytes, want 32", p.path, len(data))
    }
    return data, nil
}
```

Write `internal/crypto/crypto_test.go` using stdlib `testing` only. The 1000-nonce uniqueness test (D-06) must be present and must use `map[string]struct{}` keyed by `hex.EncodeToString(blob[:12])` to detect duplicates. Tests use a hardcoded 32-byte test key via `NewEnvKeyProvider(base64.StdEncoding.EncodeToString(make([]byte, 32)))`.
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && CGO_ENABLED=0 go test -race -count=1 ./internal/crypto/...</automated>
  </verify>
  <acceptance_criteria>
    - `internal/crypto/crypto.go` contains `const nonceSize = 12`
    - `internal/crypto/crypto.go` contains `append(nonce, ciphertext...)` (nonce prepended to ciphertext)
    - `internal/crypto/crypto.go` uses `cryptorand.Reader` (not math/rand)
    - `internal/crypto/env_provider.go` exists and contains `func (p *EnvKeyProvider) Key()`
    - `internal/crypto/file_provider.go` exists and contains `func (p *FileKeyProvider) Key()`
    - `internal/crypto/crypto_test.go` contains a test function whose body calls Encrypt() in a loop of at least 1000 iterations
    - `CGO_ENABLED=0 go test -race -count=1 ./internal/crypto/...` exits 0
    - `~/go/bin/golangci-lint run ./internal/crypto/...` exits 0
    - Grep: `grep "math/rand" internal/crypto/` returns empty (no math/rand usage)
  </acceptance_criteria>
  <done>Cipher, EnvKeyProvider, FileKeyProvider implemented and tested; 1000-nonce uniqueness test passes; no math/rand usage; lint clean.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: SQLite store with WAL dual-pool open and embedded migrations</name>
  <read_first>
    - /var/home/lm/git/polar-flow-mcp/internal/store/store.go (current stub — read before overwriting)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/ARCHITECTURE.md (SQLite Schema section, golang-migrate iofs embed pattern, BLOB storage pattern)
    - /var/home/lm/git/polar-flow-mcp/.planning/research/PITFALLS.md (Pitfall 5: SQLite BUSY, Pitfall 6: migrate driver mismatch, Pitfall 10: foreign keys)
    - /var/home/lm/git/polar-flow-mcp/.planning/phases/01-project-foundation-and-security-skeleton/01-CONTEXT.md (D-07: dual-pool readiness test, D-08: modernc driver, D-09: migrate sqlite not sqlite3, D-11: pool settings)
  </read_first>
  <files>
    internal/store/store.go,
    internal/store/store_test.go,
    migrations/001_initial.up.sql,
    migrations/001_initial.down.sql
  </files>
  <behavior>
    - Test 1: Open(":memory:") succeeds and returns a non-nil *Store
    - Test 2 (D-07): After Open(), both writeDB.PingContext() and readDB.PingContext() succeed — a wedged write pool with healthy read pool would return 503 in readyz
    - Test 3: After Open(), querying `PRAGMA journal_mode` on the writeDB returns "wal"
    - Test 4: After Open(), querying `PRAGMA foreign_keys` on the writeDB returns "1"
    - Test 5: After Open(), all three tables exist: `SELECT name FROM sqlite_master WHERE type='table' AND name='users'` returns a row
    - Test 6: After Open(), same query for 'polar_tokens' returns a row
    - Test 7: After Open(), same query for 'pending_auth' returns a row
    - Test 8: Store.Close() returns nil
  </behavior>
  <action>
Create `migrations/001_initial.up.sql` with the 3-table schema (per FOUND-07 and ARCHITECTURE.md):

```sql
CREATE TABLE IF NOT EXISTS users (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    identity     TEXT    UNIQUE NOT NULL,
    polar_user_id TEXT,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS polar_tokens (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
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

Create `migrations/001_initial.down.sql`:

```sql
DROP TABLE IF EXISTS pending_auth;
DROP TABLE IF EXISTS polar_tokens;
DROP TABLE IF EXISTS users;
```

Replace stub `internal/store/store.go` with the full implementation:

```go
package store

import (
    "context"
    "database/sql"
    "embed"
    "fmt"

    "github.com/golang-migrate/migrate/v4"
    "github.com/golang-migrate/migrate/v4/database/sqlite"
    "github.com/golang-migrate/migrate/v4/source/iofs"
    _ "modernc.org/sqlite" // pure-Go SQLite driver; CGO_ENABLED=0 compatible
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store holds the SQLite read and write connection pools.
type Store struct {
    writeDB *sql.DB
    readDB  *sql.DB
}

// walDSN returns the DSN with WAL mode, busy_timeout, synchronous=NORMAL, foreign_keys=ON.
// All pragmas set at open time via DSN, not via EXEC after open.
func walDSN(base string) string {
    return base + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
}

// Open opens the SQLite database, configures WAL + dual pools, and runs embedded migrations.
// For tests, pass ":memory:" as dsn.
func Open(dsn string) (*Store, error) {
    writeDSN := walDSN(dsn)
    readDSN := walDSN(dsn)

    writeDB, err := sql.Open("sqlite", writeDSN)
    if err != nil {
        return nil, fmt.Errorf("store: open write db: %w", err)
    }
    writeDB.SetMaxOpenConns(1) // serialize writes; prevents SQLITE_BUSY

    readDB, err := sql.Open("sqlite", readDSN)
    if err != nil {
        _ = writeDB.Close()
        return nil, fmt.Errorf("store: open read db: %w", err)
    }
    readDB.SetMaxOpenConns(4) // allow concurrent reads in WAL mode

    s := &Store{writeDB: writeDB, readDB: readDB}

    if err := s.runMigrations(); err != nil {
        _ = s.Close()
        return nil, fmt.Errorf("store: run migrations: %w", err)
    }

    return s, nil
}

// runMigrations runs all pending embedded SQL migrations using golang-migrate.
// Uses the pure-Go sqlite driver (NOT sqlite3).
func (s *Store) runMigrations() error {
    sourceDriver, err := iofs.New(migrationsFS, "migrations")
    if err != nil {
        return fmt.Errorf("store: create migration source: %w", err)
    }
    dbDriver, err := sqlite.WithInstance(s.writeDB, &sqlite.Config{})
    if err != nil {
        return fmt.Errorf("store: create migration driver: %w", err)
    }
    m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
    if err != nil {
        return fmt.Errorf("store: create migrate instance: %w", err)
    }
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("store: apply migrations: %w", err)
    }
    return nil
}

// WriteDB returns the serialized write connection pool.
func (s *Store) WriteDB() *sql.DB { return s.writeDB }

// ReadDB returns the concurrent read connection pool.
func (s *Store) ReadDB() *sql.DB { return s.readDB }

// Ping verifies both connection pools are alive.
// Used by /readyz to confirm database reachability.
func (s *Store) Ping(ctx context.Context) error {
    if err := s.writeDB.PingContext(ctx); err != nil {
        return fmt.Errorf("store: write db ping: %w", err)
    }
    if err := s.readDB.PingContext(ctx); err != nil {
        return fmt.Errorf("store: read db ping: %w", err)
    }
    return nil
}

// Close closes both connection pools.
func (s *Store) Close() error {
    var errs []error
    if err := s.writeDB.Close(); err != nil {
        errs = append(errs, err)
    }
    if err := s.readDB.Close(); err != nil {
        errs = append(errs, err)
    }
    if len(errs) > 0 {
        return fmt.Errorf("store: close: %v", errs)
    }
    return nil
}
```

Important: The migrations embed directive `//go:embed migrations/*.sql` must be in the `store` package file that also imports `embed`. The `migrations/` directory must be inside the `internal/store/` directory tree OR at the repo root if accessed from main. Per ARCHITECTURE.md the migrations directory is at repo root — use relative path `../../migrations` in the embed OR place migrations inside `internal/store/migrations/`. Use `internal/store/migrations/` to keep the embed path simple.

Adjust: Create `internal/store/migrations/001_initial.up.sql` and `internal/store/migrations/001_initial.down.sql` (move the SQL there), and the `//go:embed migrations/*.sql` will work without path hacks.

Also create `migrations/` at repo root as a symlink or empty dir for documentation purposes — OR just use `internal/store/migrations/` exclusively and update ARCHITECTURE.md note in the summary.

Write `internal/store/store_test.go` using stdlib `testing`. Use `:memory:` DSN for all tests. The D-07 test must call both `s.WriteDB().PingContext(ctx)` and `s.ReadDB().PingContext(ctx)` separately and assert both succeed.
  </action>
  <verify>
    <automated>cd /var/home/lm/git/polar-flow-mcp && CGO_ENABLED=0 go test -race -count=1 ./internal/store/...</automated>
  </verify>
  <acceptance_criteria>
    - `internal/store/store.go` contains `_ "modernc.org/sqlite"` (blank import, not mattn)
    - `internal/store/store.go` contains `//go:embed migrations/*.sql`
    - `internal/store/store.go` contains `SetMaxOpenConns(1)` for writeDB
    - `internal/store/store.go` contains `SetMaxOpenConns(4)` for readDB
    - `internal/store/store.go` contains `journal_mode(WAL)` in the DSN string
    - `internal/store/store.go` contains `busy_timeout(5000)` in the DSN string
    - `internal/store/store.go` contains `foreign_keys(ON)` in the DSN string
    - `internal/store/migrations/001_initial.up.sql` contains `CREATE TABLE users`
    - `internal/store/migrations/001_initial.up.sql` contains `CREATE TABLE polar_tokens`
    - `internal/store/migrations/001_initial.up.sql` contains `CREATE TABLE pending_auth`
    - `internal/store/migrations/001_initial.up.sql` contains `encrypted_token BLOB`
    - `grep "golang-migrate/migrate/v4/database/sqlite3" internal/store/store.go` returns empty (wrong driver)
    - `CGO_ENABLED=0 go test -race -count=1 ./internal/store/...` exits 0
    - `~/go/bin/golangci-lint run ./internal/store/...` exits 0
  </acceptance_criteria>
  <done>Store.Open() wires WAL dual-pool and runs embedded migrations; all 8 store tests pass; no CGO sqlite3 driver used; lint clean.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| database→application | Encrypted BLOB must be self-contained (nonce||ciphertext) so nonce/ciphertext misalignment is impossible |
| key source→cipher | KeyProvider must return exactly 32 bytes; anything else is rejected before AES cipher creation |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-03-01 | Information Disclosure | AES-GCM nonce reuse | mitigate | `crypto/rand` nonce per Encrypt() call; 1000-nonce uniqueness test (D-06) verifies no reuse |
| T-03-02 | Tampering | GCM authentication | mitigate | GCM produces an authentication tag; `gcm.Open()` returns error on any ciphertext modification |
| T-03-03 | Information Disclosure | BLOB column split | mitigate | nonce and ciphertext stored as single BLOB column — impossible to misalign on read |
| T-03-04 | Tampering | sqlite3 CGO driver import | mitigate | Only `modernc.org/sqlite` blank-imported; CI builds with `CGO_ENABLED=0` to catch any drift |
| T-03-05 | Elevation of Privilege | SQL referential integrity | mitigate | `foreign_keys(ON)` in DSN ensures `polar_tokens.user_id` cascades on user deletion |
| T-03-06 | Denial of Service | SQLITE_BUSY under concurrent writes | mitigate | WAL mode + `busy_timeout(5000)` + write pool `MaxOpenConns(1)` serializes writes |
</threat_model>

<verification>
```bash
cd /var/home/lm/git/polar-flow-mcp

# Crypto tests (includes 1000-nonce uniqueness)
CGO_ENABLED=0 go test -race -count=1 -v ./internal/crypto/... | grep -E "PASS|FAIL|nonce"

# Store tests (includes WAL + dual-pool + table existence)
CGO_ENABLED=0 go test -race -count=1 -v ./internal/store/...

# Full build
CGO_ENABLED=0 go build ./...

# Lint
~/go/bin/golangci-lint run ./internal/crypto/... ./internal/store/...

# Schema spot-check
grep "encrypted_token BLOB" internal/store/migrations/001_initial.up.sql

# No CGO driver
grep -r "go-sqlite3\|sqlite3" internal/ && echo "FAIL" || echo "OK: no CGO sqlite3"
```
</verification>

<success_criteria>
- `CGO_ENABLED=0 go test -race -count=1 ./internal/crypto/... ./internal/store/...` exits 0
- Crypto tests include a test verifying 1000 encrypt calls produce 1000 distinct nonces
- Store tests verify WAL mode, foreign_keys, and both pool pings succeed
- `migrations/001_initial.up.sql` (in `internal/store/migrations/`) defines all 3 tables with correct column types
- No `mattn/go-sqlite3` or `golang-migrate/.../sqlite3` imports anywhere
- `~/go/bin/golangci-lint run ./internal/crypto/... ./internal/store/...` exits 0
</success_criteria>

<output>
After completion, create `.planning/phases/01-project-foundation-and-security-skeleton/01-03-SUMMARY.md` with:
- Cipher API surface (Encrypt/Decrypt signatures)
- KeyProvider implementations and their constructor signatures
- Store API surface (Open, Close, Ping, WriteDB, ReadDB)
- Migration file locations (internal/store/migrations/ vs repo root)
- SQLite DSN string used (with all pragmas)
- Pool configuration: writeDB MaxOpenConns=1, readDB MaxOpenConns=4
- Confirmation 1000-nonce uniqueness test passes
</output>
