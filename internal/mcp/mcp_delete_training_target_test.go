//go:build polartest

package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/mcp"
	"github.com/lm/polar-flow-mcp/internal/polar"
)

func TestDeleteTrainingTarget_NoIdentity(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.Background() // no identity

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result for missing identity, got: %s", resultText(result))
	}
	if !strings.Contains(strings.ToLower(resultText(result)), "identity") {
		t.Errorf("error text %q should contain 'identity'", resultText(result))
	}
}

func TestDeleteTrainingTarget_NoLinkedAccount(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")
	// No UpsertUser — no linked account.

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"target_id": "t1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if result.IsError {
		t.Fatalf("expected non-error result (login hint), got error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "/oauth/login") {
		t.Errorf("text %q should contain /oauth/login", text)
	}
	if !strings.Contains(strings.ToLower(text), "no polar account") {
		t.Errorf("text %q should contain 'no polar account'", text)
	}
}

func TestDeleteTrainingTarget_NoToken(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	// UpsertUser only — no UpsertToken.
	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"target_id": "t1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if result.IsError {
		t.Fatalf("expected non-error result (login hint), got error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(text, "/oauth/login") {
		t.Errorf("text %q should contain /oauth/login", text)
	}
	if !strings.Contains(strings.ToLower(text), "no polar token") {
		t.Errorf("text %q should contain 'no polar token'", text)
	}
}

func TestDeleteTrainingTarget_MissingTargetID(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	blob, err := cipher.Encrypt([]byte("tok-test"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := st.UpsertToken(ctx, "alice", blob, 1); err != nil {
		t.Fatalf("UpsertToken: %v", err)
	}

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	// Call with no target_id.
	result, err := handler(ctx, makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result for missing target_id, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "target_id is required") {
		t.Errorf("error text %q should contain 'target_id is required'", resultText(result))
	}
}

func TestDeleteTrainingTarget_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	restore := polar.SetTrainingTargetsBaseURL(ts.URL)
	t.Cleanup(restore)

	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	blob, err := cipher.Encrypt([]byte("tok-test"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := st.UpsertToken(ctx, "alice", blob, 1); err != nil {
		t.Fatalf("UpsertToken: %v", err)
	}

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"target_id": "target-abc",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(strings.ToLower(text), "deleted") {
		t.Errorf("text %q should contain 'deleted'", text)
	}
	if !strings.Contains(text, "target-abc") {
		t.Errorf("text %q should contain the target_id 'target-abc'", text)
	}
}

func TestDeleteTrainingTarget_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	restore := polar.SetTrainingTargetsBaseURL(ts.URL)
	t.Cleanup(restore)

	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	blob, err := cipher.Encrypt([]byte("tok-test"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := st.UpsertToken(ctx, "alice", blob, 1); err != nil {
		t.Fatalf("UpsertToken: %v", err)
	}

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"target_id": "missing-target",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	// 404 is INFORMATIONAL — must NOT be an error result.
	if result.IsError {
		t.Fatalf("expected non-error (informational) result on 404, got error: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(strings.ToLower(text), "not found") {
		t.Errorf("text %q should contain 'not found'", text)
	}
	if !strings.Contains(text, "missing-target") {
		t.Errorf("text %q should contain the target_id 'missing-target'", text)
	}
}

func TestDeleteTrainingTarget_PolarError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("polar server error"))
	}))
	defer ts.Close()

	restore := polar.SetTrainingTargetsBaseURL(ts.URL)
	t.Cleanup(restore)

	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	blob, err := cipher.Encrypt([]byte("tok-test"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := st.UpsertToken(ctx, "alice", blob, 1); err != nil {
		t.Fatalf("UpsertToken: %v", err)
	}

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"target_id": "t1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result on Polar 500, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "status 500") {
		t.Errorf("error text %q should contain 'status 500'", resultText(result))
	}
}

func TestDeleteTrainingTarget_VerifiesHTTPMethod(t *testing.T) {
	var capturedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	restore := polar.SetTrainingTargetsBaseURL(ts.URL)
	t.Cleanup(restore)

	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	blob, err := cipher.Encrypt([]byte("tok-test"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := st.UpsertToken(ctx, "alice", blob, 1); err != nil {
		t.Fatalf("UpsertToken: %v", err)
	}

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"target_id": "t1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}
	if capturedMethod != http.MethodDelete {
		t.Errorf("expected HTTP method DELETE, got %q", capturedMethod)
	}
}
