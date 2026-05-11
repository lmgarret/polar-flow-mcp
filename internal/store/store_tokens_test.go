// Package store provides tests for polar_tokens CRUD operations.
package store

import (
	"bytes"
	"context"
	"testing"
)

// TestGetEncryptedToken_Found verifies that GetEncryptedToken returns the stored blob
// after UpsertUser + UpsertToken for the same identity.
func TestGetEncryptedToken_Found(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	blob := []byte("nonce123456abcdefciphertext")
	if err := s.UpsertToken(ctx, "alice", blob, 1); err != nil {
		t.Fatalf("UpsertToken: %v", err)
	}

	got, found, err := s.GetEncryptedToken(ctx, "alice")
	if err != nil {
		t.Fatalf("GetEncryptedToken returned unexpected error: %v", err)
	}
	if !found {
		t.Fatal("GetEncryptedToken: expected found=true, got false")
	}
	if !bytes.Equal(got, blob) {
		t.Errorf("GetEncryptedToken: got %v, want %v", got, blob)
	}
}

// TestGetEncryptedToken_NotFound verifies that GetEncryptedToken returns (nil, false, nil)
// when no row exists for the given identity.
func TestGetEncryptedToken_NotFound(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	got, found, err := s.GetEncryptedToken(ctx, "ghost")
	if err != nil {
		t.Fatalf("GetEncryptedToken returned unexpected error: %v", err)
	}
	if found {
		t.Fatal("GetEncryptedToken: expected found=false for unknown identity, got true")
	}
	if got != nil {
		t.Errorf("GetEncryptedToken: expected nil blob, got %v", got)
	}
}

// TestGetEncryptedToken_UserWithoutToken verifies that GetEncryptedToken returns (nil, false, nil)
// when the user row exists but no polar_tokens row has been inserted.
func TestGetEncryptedToken_UserWithoutToken(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	got, found, err := s.GetEncryptedToken(ctx, "alice")
	if err != nil {
		t.Fatalf("GetEncryptedToken returned unexpected error: %v", err)
	}
	if found {
		t.Fatal("GetEncryptedToken: expected found=false when no token row, got true")
	}
	if got != nil {
		t.Errorf("GetEncryptedToken: expected nil blob, got %v", got)
	}
}

// TestUpsertToken_RequiresUser verifies that UpsertToken returns a non-nil error when no
// users row exists for the given identity (FK NOT NULL violation).
func TestUpsertToken_RequiresUser(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	// No prior UpsertUser — subquery returns NULL, NOT NULL constraint must reject it.
	err := s.UpsertToken(ctx, "ghost", []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, 1)
	if err == nil {
		t.Fatal("UpsertToken without user: expected non-nil error, got nil")
	}
}

// TestUpsertToken_Insert verifies that UpsertToken stores the encrypted blob and key_version
// after a prior UpsertUser.
func TestUpsertToken_Insert(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	blob := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	if err := s.UpsertToken(ctx, "alice", blob, 1); err != nil {
		t.Fatalf("UpsertToken: %v", err)
	}

	var gotBlob []byte
	var gotKeyVersion int
	row := s.ReadDB().QueryRowContext(ctx,
		`SELECT pt.encrypted_token, pt.key_version
		 FROM polar_tokens pt
		 JOIN users u ON u.id = pt.user_id
		 WHERE u.identity = ?`,
		"alice",
	)
	if err := row.Scan(&gotBlob, &gotKeyVersion); err != nil {
		t.Fatalf("SELECT encrypted_token, key_version: %v", err)
	}
	if !bytes.Equal(gotBlob, blob) {
		t.Errorf("encrypted_token mismatch: got %v, want %v", gotBlob, blob)
	}
	if gotKeyVersion != 1 {
		t.Errorf("key_version: got %d, want 1", gotKeyVersion)
	}
}

// TestUpsertToken_UpdateOnConflict verifies that a second UpsertToken call for the same
// identity overwrites the encrypted blob.
func TestUpsertToken_UpdateOnConflict(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	blob1 := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	blob2 := []byte{20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}

	if err := s.UpsertToken(ctx, "alice", blob1, 1); err != nil {
		t.Fatalf("first UpsertToken: %v", err)
	}
	if err := s.UpsertToken(ctx, "alice", blob2, 2); err != nil {
		t.Fatalf("second UpsertToken: %v", err)
	}

	var gotBlob []byte
	row := s.ReadDB().QueryRowContext(ctx,
		`SELECT pt.encrypted_token
		 FROM polar_tokens pt
		 JOIN users u ON u.id = pt.user_id
		 WHERE u.identity = ?`,
		"alice",
	)
	if err := row.Scan(&gotBlob); err != nil {
		t.Fatalf("SELECT encrypted_token: %v", err)
	}
	if !bytes.Equal(gotBlob, blob2) {
		t.Errorf("encrypted_token after update: got %v, want %v", gotBlob, blob2)
	}
}
