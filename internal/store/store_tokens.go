// Package store: polar_tokens CRUD (per D-09, OAUTH-05).
package store

import (
	"context"
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
