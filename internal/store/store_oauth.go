// Package store: OAuth CSRF state CRUD (per D-11, D-12).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound indicates a record was not found.
//
//nolint:gochecknoglobals
var ErrNotFound = errors.New("store: record not found")

// ErrExpired indicates a record was found but has passed its expiry time.
//
//nolint:gochecknoglobals
var ErrExpired = errors.New("store: record expired")

// CreateOAuthState inserts a CSRF state token bound to identity with the given expiry (per D-11).
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

// ConsumeOAuthState atomically deletes and returns a non-expired state row (per D-12).
// Returns ErrNotFound if state is absent. Returns ErrExpired if the state exists but
// is past its expiry — the row is NOT deleted in that case (so it remains visible to
// operators / future TTL sweeps; a single expired state cannot be "consumed" silently).
// Single-use: a subsequent call with the same (non-expired) state returns ErrNotFound.
func (s *Store) ConsumeOAuthState(ctx context.Context, state string) (string, error) {
	// Atomically delete the row only if it is still within its TTL.
	// SQLite evaluates the WHERE clause inside the DELETE, so this is a single statement
	// and not subject to a SELECT-then-DELETE race.
	var identity string
	err := s.writeDB.QueryRowContext(ctx,
		`DELETE FROM pending_auth
		 WHERE state = ? AND expires_at >= datetime('now')
		 RETURNING identity`,
		state,
	).Scan(&identity)
	if err == nil {
		return identity, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("store: consume oauth state: %w", err)
	}

	// No row deleted. Determine whether the state is missing or merely expired.
	var expiresAt time.Time
	err = s.readDB.QueryRowContext(ctx,
		`SELECT expires_at FROM pending_auth WHERE state = ?`,
		state,
	).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: lookup oauth state: %w", err)
	}
	// Row exists but the conditional DELETE skipped it → it must be expired.
	// Defensive sanity check (handles clock-skew edge cases).
	if time.Now().UTC().After(expiresAt) {
		return "", ErrExpired
	}
	// Extremely unlikely: row exists, not expired, but DELETE returned no rows.
	// Treat as a transient race (e.g. concurrent expiry tick) — surface as ErrNotFound.
	return "", ErrNotFound
}
