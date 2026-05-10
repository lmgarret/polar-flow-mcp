// Package store: users table CRUD (per D-08, D-10).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// UpsertUser inserts or updates the users row for identity, storing polar_user_id as TEXT (per D-08).
// The schema declares polar_user_id TEXT — pass strconv.FormatInt for numeric Polar IDs.
func (s *Store) UpsertUser(ctx context.Context, identity, polarUserID string) error {
	_, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO users (identity, polar_user_id, created_at)
         VALUES (?, ?, datetime('now'))
         ON CONFLICT(identity) DO UPDATE SET polar_user_id = excluded.polar_user_id`,
		identity, polarUserID,
	)
	if err != nil {
		return fmt.Errorf("store: upsert user: %w", err)
	}
	return nil
}

// GetPolarUserID returns the Polar user ID linked to identity (per D-10).
// Returns ("", false, nil) if no user row exists OR polar_user_id is empty/NULL.
// Returns a non-nil error only on database failure.
func (s *Store) GetPolarUserID(ctx context.Context, identity string) (string, bool, error) {
	var polarUserID sql.NullString
	err := s.readDB.QueryRowContext(ctx,
		`SELECT polar_user_id FROM users WHERE identity = ?`,
		identity,
	).Scan(&polarUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get polar user id: %w", err)
	}
	if !polarUserID.Valid || polarUserID.String == "" {
		return "", false, nil
	}
	return polarUserID.String, true, nil
}
