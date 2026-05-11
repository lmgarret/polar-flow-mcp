//go:build polartest

package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/mcp"
	"github.com/lm/polar-flow-mcp/internal/polar"
)

func TestListTrainingTargets_NoIdentity(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.Background() // no identity

	handler := mcp.ListTrainingTargetsHandler(st, cipher)
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

func TestListTrainingTargets_NoLinkedAccount(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")
	// No UpsertUser — no linked account.

	handler := mcp.ListTrainingTargetsHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{}))
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

func TestListTrainingTargets_NoToken(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	// UpsertUser only — no UpsertToken.
	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	handler := mcp.ListTrainingTargetsHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{}))
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

func TestListTrainingTargets_DefaultDates(t *testing.T) {
	var capturedQuery string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
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

	handler := mcp.ListTrainingTargetsHandler(st, cipher)
	// Call with NO from/to args — handler should supply defaults.
	result, err := handler(ctx, makeRequest(map[string]any{}))
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

	// Verify defaults were applied: today and today+30.
	today := time.Now().UTC().Format("2006-01-02")
	todayPlus30 := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")

	if !strings.Contains(capturedQuery, "from_date="+today) {
		t.Errorf("query %q should contain from_date=%s", capturedQuery, today)
	}
	if !strings.Contains(capturedQuery, "to_date="+todayPlus30) {
		t.Errorf("query %q should contain to_date=%s", capturedQuery, todayPlus30)
	}

	text := resultText(result)
	if !strings.Contains(text, "No training targets between") {
		t.Errorf("text %q should contain 'No training targets between'", text)
	}
	if !strings.Contains(text, today) {
		t.Errorf("text %q should contain today's date %s", text, today)
	}
	if !strings.Contains(text, todayPlus30) {
		t.Errorf("text %q should contain to_date %s", text, todayPlus30)
	}
}

func TestListTrainingTargets_PopulatedList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"t1","name":"Easy run","date":"2026-05-15","time":"18:00"},{"id":"t2","name":"Intervals","date":"2026-05-17","time":"07:00"}]`))
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

	handler := mcp.ListTrainingTargetsHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"from_date": "2026-05-15",
		"to_date":   "2026-06-15",
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
	if !strings.Contains(text, "Training targets between") {
		t.Errorf("text %q should contain 'Training targets between'", text)
	}
	if !strings.Contains(text, "t1: Easy run (2026-05-15 18:00)") {
		t.Errorf("text %q should contain 't1: Easy run (2026-05-15 18:00)'", text)
	}
	if !strings.Contains(text, "t2: Intervals (2026-05-17 07:00)") {
		t.Errorf("text %q should contain 't2: Intervals (2026-05-17 07:00)'", text)
	}
}

func TestListTrainingTargets_InvalidFromDate(t *testing.T) {
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

	handler := mcp.ListTrainingTargetsHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"from_date": "bogus",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result for bogus from_date, got: %s", resultText(result))
	}
	if !strings.Contains(strings.ToUpper(resultText(result)), "ISO 8601") {
		t.Errorf("error text %q should contain 'ISO 8601'", resultText(result))
	}
}

func TestListTrainingTargets_PolarError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal polar error"))
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

	handler := mcp.ListTrainingTargetsHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{}))
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

// TestConcurrentListAndDeleteHandlers verifies that concurrent invocations of
// list and delete handlers with distinct identities are race-clean.
func TestConcurrentListAndDeleteHandlers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	restore := polar.SetTrainingTargetsBaseURL(ts.URL)
	t.Cleanup(restore)

	st := openTestStore(t)
	cipher := newTestCipher(t)

	// Set up two distinct users.
	ctxAlice := context.WithValue(context.Background(), auth.UserIDKey, "alice")
	ctxBob := context.WithValue(context.Background(), auth.UserIDKey, "bob")

	for _, setup := range []struct {
		ctx      context.Context
		identity string
		polarID  string
	}{
		{ctxAlice, "alice", "11111"},
		{ctxBob, "bob", "22222"},
	} {
		if err := st.UpsertUser(setup.ctx, setup.identity, setup.polarID); err != nil {
			t.Fatalf("UpsertUser(%s): %v", setup.identity, err)
		}
		blob, err := cipher.Encrypt([]byte("tok-" + setup.identity))
		if err != nil {
			t.Fatalf("Encrypt(%s): %v", setup.identity, err)
		}
		if err := st.UpsertToken(setup.ctx, setup.identity, blob, 1); err != nil {
			t.Fatalf("UpsertToken(%s): %v", setup.identity, err)
		}
	}

	listHandler := mcp.ListTrainingTargetsHandler(st, cipher)
	deleteHandler := mcp.DeleteTrainingTargetHandler(st, cipher)

	errs := make(chan error, 20)
	for i := 0; i < 10; i++ {
		go func() {
			result, err := listHandler(ctxAlice, makeRequest(map[string]any{}))
			if err != nil {
				errs <- err
				return
			}
			if result == nil {
				errs <- nil
				return
			}
			errs <- nil
		}()
		go func() {
			result, err := deleteHandler(ctxBob, makeRequest(map[string]any{
				"target_id": "target-concurrent",
			}))
			if err != nil {
				errs <- err
				return
			}
			if result == nil {
				errs <- nil
				return
			}
			errs <- nil
		}()
	}

	for i := 0; i < 20; i++ {
		if err := <-errs; err != nil {
			t.Errorf("goroutine returned error: %v", err)
		}
	}
}
