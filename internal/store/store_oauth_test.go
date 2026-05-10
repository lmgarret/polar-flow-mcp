// Package store provides tests for OAuth CSRF state CRUD operations.
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestCreateAndConsumeOAuthState verifies that CreateOAuthState inserts a state row and
// ConsumeOAuthState atomically deletes and returns the identity.
func TestCreateAndConsumeOAuthState(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	state := "abc"
	identity := "alice"
	expiresAt := time.Now().UTC().Add(10 * time.Minute)

	if err := s.CreateOAuthState(ctx, state, identity, expiresAt); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}

	got, err := s.ConsumeOAuthState(ctx, state)
	if err != nil {
		t.Fatalf("ConsumeOAuthState: %v", err)
	}
	if got != identity {
		t.Errorf("ConsumeOAuthState: got identity %q, want %q", got, identity)
	}

	// Verify the row was deleted — re-consume must return ErrNotFound.
	_, err = s.ConsumeOAuthState(ctx, state)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("second ConsumeOAuthState: expected ErrNotFound, got %v", err)
	}

	// Verify no pending_auth rows remain for this state.
	var count int
	row := s.WriteDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM pending_auth WHERE state = ?", state)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT: %v", err)
	}
	if count != 0 {
		t.Errorf("pending_auth count for %q: got %d, want 0", state, count)
	}
}

// TestConsumeOAuthState_NotFound verifies that ConsumeOAuthState returns ErrNotFound for
// a state that was never inserted.
func TestConsumeOAuthState_NotFound(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	_, err := s.ConsumeOAuthState(ctx, "never-existed")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestConsumeOAuthState_Expired verifies that a state row past its expiry time causes
// ConsumeOAuthState to return ErrExpired and delete the row.
func TestConsumeOAuthState_Expired(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()

	state := "stale-state"
	identity := "bob"
	// Insert directly with an already-expired timestamp to simulate a stale row.
	_, err := s.WriteDB().ExecContext(ctx,
		"INSERT INTO pending_auth (state, identity, expires_at) VALUES (?, ?, ?)",
		state, identity, time.Now().UTC().Add(-time.Minute),
	)
	if err != nil {
		t.Fatalf("INSERT stale state: %v", err)
	}

	_, err = s.ConsumeOAuthState(ctx, state)
	if !errors.Is(err, ErrExpired) {
		t.Errorf("expected ErrExpired, got %v", err)
	}

	// Row must be deleted even when expired — re-consume must return ErrNotFound.
	_, err = s.ConsumeOAuthState(ctx, state)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("second ConsumeOAuthState after expiry: expected ErrNotFound, got %v", err)
	}
}
