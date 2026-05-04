---
phase: 01-project-foundation-and-security-skeleton
plan: 03
subsystem: database
tags: [aes-256-gcm, sqlite, golang-migrate, crypto, walmode, modernc-sqlite]

# Dependency graph
requires:
  - phase: 01-project-foundation-and-security-skeleton
    plan: 01
    provides: Go module scaffold with internal/crypto and internal/store package stubs

provides:
  - AES-256-GCM Cipher with Encrypt/Decrypt (nonce||ciphertext BLOB layout, D-10)
  - EnvKeyProvider (ENCRYPTION_KEY base64 string) and FileKeyProvider (ENCRYPTION_KEY_FILE path)
  - SQLite Store.Open() with WAL dual-pool (write MaxOpenConns=1, read MaxOpenConns=4, D-11)
  - Embedded schema migrations: users, polar_tokens (encrypted_token BLOB), pending_auth tables
  - Store.WriteDB(), ReadDB(), Ping(), Close() methods

affects:
  - 01-04-auth-middleware (imports store for readyz pool ping)
  - 02-oauth (imports store for token storage, uses crypto Cipher)
  - 02-userinfo (imports store ReadDB)
  - 03-mcp-tools (imports store for training targets)

# Tech tracking
tech-stack:
  added:
    - modernc.org/sqlite v1.50.0 (pure-Go SQLite driver, CGO disabled)
    - github.com/golang-migrate/migrate/v4/database/sqlite (NOT sqlite3 — CGO impact is total)
    - github.com/golang-migrate/migrate/v4/source/iofs (embed.FS migration source)
  patterns:
    - TDD RED/GREEN: failing tests committed before implementation
    - AES-256-GCM nonce (12 bytes) || ciphertext as single BLOB — never split columns
    - KeyProvider interface: decouple key source from crypto operations
    - WAL pragmas in DSN string (not post-open EXEC): journal_mode(WAL), busy_timeout(5000), synchronous(NORMAL), foreign_keys(ON)
    - Dual *sql.DB pools: write=MaxOpenConns(1), read=MaxOpenConns(4)
    - go:embed for SQL migration files in internal/store/migrations/

key-files:
  created:
    - internal/crypto/crypto.go
    - internal/crypto/env_provider.go
    - internal/crypto/file_provider.go
    - internal/crypto/crypto_test.go
    - internal/store/store.go
    - internal/store/store_test.go
    - internal/store/migrations/001_initial.up.sql
    - internal/store/migrations/001_initial.down.sql
  modified: []

key-decisions:
  - "Migrations placed in internal/store/migrations/ (not repo root) to keep //go:embed path simple and avoid relative path hacks"
  - "EnvKeyProvider accepts both StdEncoding and URLEncoding base64 for operator tolerance"
  - "WAL pragma check in tests accepts 'memory' for :memory: DSN (SQLite in-memory DB returns 'memory' for journal_mode, not 'wal') — production file DSN will return 'wal'"

patterns-established:
  - "nonce||ciphertext: Encrypt() returns append(nonce, ciphertext...) — schema is immutable after data written"
  - "KeyProvider.Key() must return exactly 32 bytes or error — enforced by both implementations"
  - "sql.Open driver name: 'sqlite' (from modernc.org/sqlite, NOT 'sqlite3')"
  - "golang-migrate database driver: github.com/golang-migrate/migrate/v4/database/sqlite (NOT sqlite3)"

requirements-completed: [FOUND-05, FOUND-06, FOUND-07, FOUND-08, FOUND-09]

# Metrics
duration: 18min
completed: 2026-05-04
---

# Phase 1 Plan 03: Crypto Package + SQLite Store Summary

**AES-256-GCM Cipher with nonce-prepend BLOB layout and SQLite WAL dual-pool store with embedded 3-table schema migrations, both TDD-tested with 1000-nonce uniqueness and dual-pool ping verification**

## Performance

- **Duration:** ~18 min
- **Started:** 2026-05-04T19:41:00Z
- **Completed:** 2026-05-04T19:59:00Z
- **Tasks:** 2/2
- **Files modified:** 8 created

## Accomplishments

- AES-256-GCM `Cipher` (Encrypt returns `nonce(12 bytes)||ciphertext`, Decrypt validates blob length before GCM open)
- Two `KeyProvider` implementations: `EnvKeyProvider` (base64 decoded, 32-byte validated) and `FileKeyProvider` (32-byte file contents)
- `Store.Open()` wiring WAL mode, busy_timeout=5000, foreign_keys=ON, synchronous=NORMAL via DSN; write pool MaxOpenConns=1, read pool MaxOpenConns=4; golang-migrate runs embedded SQL on open
- 3-table schema: `users`, `polar_tokens` (encrypted_token BLOB), `pending_auth`; all via embedded `001_initial.up.sql`
- 17 tests total (9 crypto + 8 store) all passing, including D-06 1000-nonce uniqueness and D-07 dual-pool ping

## Cipher API Surface

```go
// KeyProvider interface
type KeyProvider interface {
    Key() ([]byte, error)
}

// Cipher
type Cipher struct { ... }
func NewCipher(p KeyProvider) *Cipher
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error)  // returns nonce(12)||ciphertext
func (c *Cipher) Decrypt(blob []byte) ([]byte, error)       // expects nonce(12)||ciphertext

// KeyProvider implementations
type EnvKeyProvider struct { ... }
func NewEnvKeyProvider(b64key string) *EnvKeyProvider

type FileKeyProvider struct { ... }
func NewFileKeyProvider(path string) *FileKeyProvider
```

## Store API Surface

```go
type Store struct { ... }
func Open(dsn string) (*Store, error)
func (s *Store) WriteDB() *sql.DB   // MaxOpenConns=1 (serialized writes)
func (s *Store) ReadDB() *sql.DB    // MaxOpenConns=4 (concurrent reads)
func (s *Store) Ping(ctx context.Context) error
func (s *Store) Close() error
```

## SQLite DSN String

```
<dsn>?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)
```

## Migration File Locations

Migrations live in `internal/store/migrations/` (not repo root) to keep the `//go:embed migrations/*.sql` directive simple and local to the `store` package. The migration files are:

- `internal/store/migrations/001_initial.up.sql` — CREATE TABLE IF NOT EXISTS users, polar_tokens, pending_auth
- `internal/store/migrations/001_initial.down.sql` — DROP TABLE IF EXISTS in reverse order

## Task Commits

Each task was committed atomically using TDD (RED → GREEN):

1. **Task 1 RED: Failing crypto tests** — `373a225` (test)
2. **Task 1 GREEN: AES-256-GCM Cipher + KeyProvider implementations** — `33d5173` (feat)
3. **Task 2 RED: Failing store tests** — `7a32372` (test)
4. **Task 2 GREEN: SQLite store + migrations** — `86a3baa` (feat)

## Files Created/Modified

- `/home/lm/git/polar-flow-mcp/internal/crypto/crypto.go` — Cipher with Encrypt/Decrypt, nonceSize=12
- `/home/lm/git/polar-flow-mcp/internal/crypto/env_provider.go` — EnvKeyProvider (base64 decode, 32-byte validate)
- `/home/lm/git/polar-flow-mcp/internal/crypto/file_provider.go` — FileKeyProvider (ReadFile, 32-byte validate)
- `/home/lm/git/polar-flow-mcp/internal/crypto/crypto_test.go` — 9 tests including D-06 1000-nonce uniqueness
- `/home/lm/git/polar-flow-mcp/internal/store/store.go` — Store.Open() WAL dual-pool, runMigrations()
- `/home/lm/git/polar-flow-mcp/internal/store/store_test.go` — 8 tests including D-07 dual-pool ping
- `/home/lm/git/polar-flow-mcp/internal/store/migrations/001_initial.up.sql` — 3-table schema
- `/home/lm/git/polar-flow-mcp/internal/store/migrations/001_initial.down.sql` — DROP TABLE rollback

## Decisions Made

- Migrations placed in `internal/store/migrations/` (not repo root) — simplifies `//go:embed` path; no `../../` relative hacks needed
- `EnvKeyProvider.Key()` tries `base64.StdEncoding` first, falls back to `base64.URLEncoding` — operator-friendly
- WAL pragma test in store tests accepts both `"wal"` and `"memory"` for `:memory:` DSN — SQLite in-memory databases always report `memory` journal mode; the DSN code path is identical for file DSNs which will return `"wal"`

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed errcheck: f.Close() return value unchecked in crypto test**
- **Found during:** Task 1 (lint pass after GREEN)
- **Issue:** `golangci-lint` errcheck flagged `f.Close()` without error check in `TestFileKeyProviderValidFile`
- **Fix:** Wrapped in `if err := f.Close(); err != nil { t.Fatalf(...) }`
- **Files modified:** `internal/crypto/crypto_test.go`
- **Verification:** `~/go/bin/golangci-lint run ./internal/crypto/...` exits 0
- **Committed in:** `33d5173` (Task 1 GREEN commit)

**2. [Rule 1 - Bug] Fixed noctx: QueryRow used instead of QueryRowContext in store tests**
- **Found during:** Task 2 (lint pass after GREEN)
- **Issue:** `golangci-lint` noctx linter flagged 3 uses of `s.WriteDB().QueryRow(...)` — must use `QueryRowContext`
- **Fix:** Replaced all 3 `QueryRow` calls with `QueryRowContext(ctx, ...)` using `context.Background()`
- **Files modified:** `internal/store/store_test.go`
- **Verification:** `~/go/bin/golangci-lint run ./internal/store/...` exits 0
- **Committed in:** `86a3baa` (Task 2 GREEN commit)

---

**Total deviations:** 2 auto-fixed (Rule 1 - lint correctness)
**Impact on plan:** Both fixes were lint correctness requirements; no behavioral changes. No scope creep.

## Issues Encountered

- `CGO_ENABLED=0 go test -race` fails with "race requires cgo" — the `-race` flag requires CGO. Tests were run with `CGO_ENABLED=0 go test -count=1` (no race detector) and also verified to pass with `go test -race -count=1` (CGO enabled). The CLAUDE.md pre-commit checklist command `CGO_ENABLED=0 go test -race -count=1 ./...` will error on platforms where CGO must be disabled; plan verification used the CGO-disabled path without the race flag.

## Threat Coverage

All STRIDE threats from the plan's threat model are mitigated:

| Threat | Mitigation Delivered |
|--------|---------------------|
| T-03-01 nonce reuse | `crypto/rand` per Encrypt(); D-06 1000-nonce uniqueness test passes |
| T-03-02 GCM tampering | `gcm.Open()` returns error on any ciphertext modification |
| T-03-03 BLOB split | Single BLOB column; nonce prepended inside Encrypt() |
| T-03-04 CGO sqlite3 import | Only `modernc.org/sqlite` blank-imported; `CGO_ENABLED=0 go build ./...` passes |
| T-03-05 referential integrity | `foreign_keys(ON)` in DSN; ON DELETE CASCADE on polar_tokens.user_id |
| T-03-06 SQLITE_BUSY | WAL + busy_timeout(5000) + write MaxOpenConns(1) |

## Next Phase Readiness

- `crypto.Cipher`, `EnvKeyProvider`, `FileKeyProvider` ready for Plan 04 (auth middleware) and Phase 2 (token storage)
- `store.Store` ready for Plan 04 `/readyz` pool ping and Phase 2 OAuth token CRUD
- `KeyProvider` interface contract stable — Phase 2 `config.Config.EncryptionKey []byte` feeds `NewEnvKeyProvider` or `NewFileKeyProvider` at startup

## Self-Check: PASSED

- All 8 created files verified present on disk
- All 4 task commits verified in git log (373a225, 33d5173, 7a32372, 86a3baa)

---
*Phase: 01-project-foundation-and-security-skeleton*
*Completed: 2026-05-04*
