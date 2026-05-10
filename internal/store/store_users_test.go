// Package store provides tests for users table CRUD operations.
package store

import (
	"context"
	"testing"
)

// TestUpsertUser_Insert verifies that UpsertUser creates a user row with the given polar_user_id.
func TestUpsertUser_Insert(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	var polarUserID string
	row := s.ReadDB().QueryRowContext(ctx, "SELECT polar_user_id FROM users WHERE identity = ?", "alice")
	if err := row.Scan(&polarUserID); err != nil {
		t.Fatalf("SELECT polar_user_id: %v", err)
	}
	if polarUserID != "12345" {
		t.Errorf("polar_user_id: got %q, want %q", polarUserID, "12345")
	}
}

// TestUpsertUser_UpdateOnConflict verifies that a second UpsertUser for the same identity
// updates polar_user_id and results in exactly one row.
func TestUpsertUser_UpdateOnConflict(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("first UpsertUser: %v", err)
	}
	if err := s.UpsertUser(ctx, "alice", "99999"); err != nil {
		t.Fatalf("second UpsertUser: %v", err)
	}

	var polarUserID string
	row := s.ReadDB().QueryRowContext(ctx, "SELECT polar_user_id FROM users WHERE identity = ?", "alice")
	if err := row.Scan(&polarUserID); err != nil {
		t.Fatalf("SELECT polar_user_id: %v", err)
	}
	if polarUserID != "99999" {
		t.Errorf("polar_user_id after update: got %q, want %q", polarUserID, "99999")
	}

	var count int
	row = s.ReadDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE identity = ?", "alice")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT: %v", err)
	}
	if count != 1 {
		t.Errorf("user row count for alice: got %d, want 1", count)
	}
}

// TestGetPolarUserID_Found verifies that GetPolarUserID returns the linked Polar user ID
// after a successful UpsertUser.
func TestGetPolarUserID_Found(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	id, found, err := s.GetPolarUserID(ctx, "alice")
	if err != nil {
		t.Fatalf("GetPolarUserID: %v", err)
	}
	if !found {
		t.Error("GetPolarUserID: expected found=true, got false")
	}
	if id != "12345" {
		t.Errorf("GetPolarUserID: got %q, want %q", id, "12345")
	}
}

// TestGetPolarUserID_NotFound verifies that GetPolarUserID returns ("", false, nil) when
// no user row exists for the given identity.
func TestGetPolarUserID_NotFound(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	id, found, err := s.GetPolarUserID(ctx, "nobody")
	if err != nil {
		t.Fatalf("GetPolarUserID: unexpected error: %v", err)
	}
	if found {
		t.Error("GetPolarUserID: expected found=false, got true")
	}
	if id != "" {
		t.Errorf("GetPolarUserID: expected empty string, got %q", id)
	}
}
