// Package store provides SQLite-backed persistent storage with dual connection pools.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // pure-Go SQLite driver; CGO_ENABLED=0 compatible.
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store holds the SQLite read and write connection pools.
type Store struct {
	writeDB *sql.DB
	readDB  *sql.DB
}

// walDSN returns the DSN with WAL mode, busy_timeout, synchronous=NORMAL, and foreign_keys=ON.
// All pragmas are set at open time via the DSN — not via EXEC after open.
func walDSN(base string) string {
	return base + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
}

// Open opens the SQLite database, configures WAL mode and dual connection pools, then runs the
// embedded schema migrations. Pass ":memory:" as dsn for tests.
func Open(dsn string) (*Store, error) {
	writeDSN := walDSN(dsn)
	readDSN := walDSN(dsn)

	writeDB, err := sql.Open("sqlite", writeDSN)
	if err != nil {
		return nil, fmt.Errorf("store: open write db: %w", err)
	}
	writeDB.SetMaxOpenConns(1) // serialize writes; prevents SQLITE_BUSY.

	readDB, err := sql.Open("sqlite", readDSN)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("store: open read db: %w", err)
	}
	readDB.SetMaxOpenConns(4) // allow concurrent reads in WAL mode.

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
	dbDriver, err := migratesqlite.WithInstance(s.writeDB, &migratesqlite.Config{})
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
