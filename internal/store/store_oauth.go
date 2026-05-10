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

// ConsumeOAuthState atomically deletes and returns the state row (per D-12).
// Returns ErrNotFound if state is absent, ErrExpired if state existed but is past expiry.
// Single-use: a subsequent call with the same state returns ErrNotFound.
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
