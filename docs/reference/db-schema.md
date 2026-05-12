# Database Schema

polar-flow-mcp uses a single SQLite database with three tables. The schema is managed
by embedded migrations using `golang-migrate` with the pure-Go `modernc.org/sqlite`
driver (CGO is disabled throughout).

---

## Design notes

### WAL mode and dual-pool design

The database is opened in **WAL (Write-Ahead Log) mode**. WAL allows concurrent
readers without blocking writers, which is essential because HTTP requests can arrive
simultaneously — a tool call (read) and an OAuth callback (write) may overlap.

Two separate `*sql.DB` connection pools are created:

- **Write pool**: `MaxOpenConns = 1` — serializes all writes, preventing
  `SQLITE_BUSY` errors under concurrent writes.
- **Read pool**: `MaxOpenConns = 4` — allows up to four concurrent read queries.

All read queries (token lookup, user lookup, list targets) use the read pool. All write
queries (upsert user, store token, insert OAuth state) use the write pool.

### Encryption layout

The `polar_tokens.encrypted_token` column stores AES-256-GCM ciphertext as a single
BLOB with the format:

```
| nonce (12 bytes) | ciphertext (variable) |
```

The nonce is prepended to the ciphertext before storing, so a single column carries all
the information needed to decrypt. This prevents nonce/ciphertext misalignment that
would occur if they were stored in separate columns. The schema is immutable after data
is written — no migrations will change this layout.

---

## Tables

### `users`

Stores one row per authenticated user identity. The `identity` value comes from the
reverse proxy's identity header (e.g., `Remote-User: alice` → identity `alice`).

```sql
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    identity      TEXT    UNIQUE NOT NULL,
    polar_user_id TEXT,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER | Auto-increment primary key (internal FK target) |
| `identity` | TEXT UNIQUE NOT NULL | User identity from reverse proxy header |
| `polar_user_id` | TEXT | Polar API user ID, populated after OAuth registration. `NULL` until linked. |
| `created_at` | DATETIME | UTC timestamp, auto-set on insert |

The `identity` column has a UNIQUE constraint — each proxy user maps to exactly one row.
`polar_user_id` is TEXT (not an integer) because the Polar API returns it as a string.

---

### `polar_tokens`

Stores the encrypted OAuth token for each linked user. One row per user.

```sql
CREATE TABLE IF NOT EXISTS polar_tokens (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    encrypted_token BLOB    NOT NULL,
    key_version     INTEGER NOT NULL DEFAULT 1,
    updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER | Auto-increment primary key |
| `user_id` | INTEGER UNIQUE NOT NULL | Foreign key to `users.id`; UNIQUE enforces one token per user |
| `encrypted_token` | BLOB NOT NULL | `nonce (12 bytes) \|\| AES-256-GCM ciphertext` |
| `key_version` | INTEGER NOT NULL DEFAULT 1 | Identifies which encryption key was used |
| `updated_at` | DATETIME NOT NULL | UTC timestamp, updated on each token refresh |

The `ON DELETE CASCADE` constraint means deleting a user also deletes their token —
useful if a user account is removed from the system.

**`key_version`**: In v1, only version 1 is supported. The column exists to enable key
rotation in v2: a migration tool could decrypt with the old key (version 1) and
re-encrypt with a new key (version 2), then update this column. See
[Security — Key rotation plan](../security.md#key-rotation-plan) for details.

---

### `pending_auth`

Stores CSRF state tokens for in-flight OAuth authorization flows. State tokens are
single-use and expire after a short window.

```sql
CREATE TABLE IF NOT EXISTS pending_auth (
    state      TEXT     PRIMARY KEY,
    identity   TEXT     NOT NULL,
    expires_at DATETIME NOT NULL
);
```

| Column | Type | Notes |
|--------|------|-------|
| `state` | TEXT PRIMARY KEY | Cryptographically random state token (URL-safe) |
| `identity` | TEXT NOT NULL | User identity that initiated the OAuth flow |
| `expires_at` | DATETIME NOT NULL | UTC expiry time; expired rows are rejected on callback |

**Atomic consume-on-first-use**: The OAuth callback consumes the state token using a
`DELETE ... WHERE expires_at >= datetime('now') RETURNING state` query. This is atomic:
the row is deleted and returned in a single operation. If the state was already consumed
(already deleted) or has expired, the query returns no rows and the callback rejects
the request.

Storing CSRF state in SQLite (rather than an in-memory map) makes the server
restart-safe: an in-flight OAuth flow survives a container restart.

---

## Migrations

Schema migrations are stored in `internal/store/migrations/` as numbered SQL files:

```
001_initial.up.sql    — creates users, polar_tokens, pending_auth
001_initial.down.sql  — drops all three tables
```

Migrations are embedded into the binary at compile time using `//go:embed` and applied
automatically at startup via `golang-migrate`. The database is created if it does not
exist.
