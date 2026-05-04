// Package store provides SQLite-backed persistent storage with dual connection pools.
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
