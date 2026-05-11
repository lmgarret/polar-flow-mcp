// Package store: polar_tokens CRUD (per D-09, OAUTH-05).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// UpsertToken inserts or replaces the encrypted token blob for identity (per D-09).
// The caller MUST have already called UpsertUser(identity, ...) — polar_tokens.user_id has
// a NOT NULL FK to users(id), and the subquery returns NULL if the user row is missing,
// which the NOT NULL constraint will reject.
// encryptedBlob is the raw output of crypto.Cipher.Encrypt (nonce||ciphertext).
func (s *Store) UpsertToken(ctx context.Context, identity string, encryptedBlob []byte, keyVersion int) error {
	_, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO polar_tokens (user_id, encrypted_token, key_version, updated_at)
         VALUES ((SELECT id FROM users WHERE identity = ?), ?, ?, datetime('now'))
         ON CONFLICT(user_id) DO UPDATE SET
             encrypted_token = excluded.encrypted_token,
             key_version     = excluded.key_version,
             updated_at      = datetime('now')`,
		identity, encryptedBlob, keyVersion,
	)
	if err != nil {
		return fmt.Errorf("store: upsert token: %w", err)
	}
	return nil
}

// GetEncryptedToken returns the encrypted token blob for identity (per MCP-06).
// Returns (nil, false, nil) if no token row exists for the identity.
// Returns a non-nil error only on database failure.
// Uses the read pool (s.readDB) — same pattern as GetPolarUserID.
func (s *Store) GetEncryptedToken(ctx context.Context, identity string) ([]byte, bool, error) {
	var blob []byte
	err := s.readDB.QueryRowContext(ctx,
		`SELECT pt.encrypted_token
		 FROM polar_tokens pt
		 JOIN users u ON u.id = pt.user_id
		 WHERE u.identity = ?`,
		identity,
	).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: get encrypted token: %w", err)
	}
	return blob, true, nil
}
