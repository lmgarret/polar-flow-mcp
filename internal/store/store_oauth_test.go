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
// ConsumeOAuthState to return ErrExpired. The row is NOT deleted (CR-01 fix).
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

	// CR-01: expired row must NOT be deleted — second call must still return ErrExpired.
	_, err = s.ConsumeOAuthState(ctx, state)
	if !errors.Is(err, ErrExpired) {
		t.Errorf("second ConsumeOAuthState after expiry: expected ErrExpired (row retained), got %v", err)
	}
}

// TestConsumeOAuthState_Expired_DoesNotDelete verifies that ConsumeOAuthState does not delete
// expired rows, and the row count remains 1 after the call (CR-01 regression guard).
func TestConsumeOAuthState_Expired_DoesNotDelete(t *testing.T) {
	s := openForTest(t)
	ctx := context.Background()
	const state = "expired-state-hex"
	// Insert with an expiry 1 hour in the past.
	if err := s.CreateOAuthState(ctx, state, "alice", time.Now().UTC().Add(-1*time.Hour)); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}
	_, err := s.ConsumeOAuthState(ctx, state)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
	// Row must still be present (CR-01 fix).
	var n int
	if err := s.ReadDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pending_auth WHERE state = ?`, state,
	).Scan(&n); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected expired row to remain (CR-01); count=%d", n)
	}
	// Second call still returns ErrExpired (idempotent — row was not consumed).
	_, err2 := s.ConsumeOAuthState(ctx, state)
	if !errors.Is(err2, ErrExpired) {
		t.Fatalf("second call: expected ErrExpired, got %v", err2)
	}
}
