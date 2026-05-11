//go:build polartest

package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/mcp"
	"github.com/lm/polar-flow-mcp/internal/polar"
)

// newTestCipher builds a deterministic cipher for tests using a fixed 32-byte key.
func newTestCipher(_ *testing.T) *crypto.Cipher {
	return crypto.NewCipher(crypto.NewBytesKeyProvider(bytes.Repeat([]byte{0x42}, 32)))
}

// makeRequest builds a CallToolRequest with the given arguments map.
func makeRequest(args map[string]any) mcpgo.CallToolRequest {
	return mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Arguments: args,
		},
	}
}

func TestCreateTrainingTarget_NoIdentity(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.Background() // no identity

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result, got: %s", resultText(result))
	}
	if !strings.Contains(strings.ToLower(resultText(result)), "identity") {
		t.Errorf("error text %q should contain 'identity'", resultText(result))
	}
}

func TestCreateTrainingTarget_NoLinkedAccount(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")
	// No UpsertUser — no linked account.

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name":   "Test",
		"date":   "2026-05-15",
		"phases": []any{},
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

func TestCreateTrainingTarget_NoToken(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	// UpsertUser only — no UpsertToken.
	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name":   "Test",
		"date":   "2026-05-15",
		"phases": []any{},
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

func TestCreateTrainingTarget_Success_5x1kmThreshold(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"99"}`))
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
	// Encrypt the token "tok-test" using the test cipher.
	blob, err := cipher.Encrypt([]byte("tok-test"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := st.UpsertToken(ctx, "alice", blob, 1); err != nil {
		t.Fatalf("UpsertToken: %v", err)
	}

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name": "5x1km Threshold",
		"date": "2026-05-15",
		"time": "18:00",
		"phases": []any{
			map[string]any{
				"type":       "warmup",
				"duration_s": float64(600),
			},
			map[string]any{
				"type": "repeat",
				"reps": float64(5),
				"goal": map[string]any{
					"distance_m": float64(1000),
				},
				"intensity": map[string]any{
					"label": "threshold",
				},
				"recovery": map[string]any{
					"duration_s": float64(120),
				},
			},
			map[string]any{
				"type":       "cooldown",
				"duration_s": float64(300),
			},
		},
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
	if !strings.Contains(text, "ID: 99") {
		t.Errorf("text %q should contain 'ID: 99'", text)
	}

	// Inspect captured request body JSON.
	bodyStr := string(capturedBody)
	for _, want := range []string{
		`"duration":600000`,
		`"distance":1000`,
		`"repeatCount":5`,
		`"lowerZone":4`,
		`"upperZone":4`,
		`"duration":120000`,
		`"duration":300000`,
		`"name":"5x1km Threshold"`,
		`"year":2026`,
		`"month":5`,
		`"day":15`,
		`"hour":18`,
		`"min":0`,
	} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("body missing %q; body = %s", want, bodyStr)
		}
	}
}

func TestCreateTrainingTarget_HRZoneEscapeHatch(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1"}`))
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

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name": "Tempo",
		"date": "2026-05-15",
		"phases": []any{
			map[string]any{
				"type": "repeat",
				"reps": float64(3),
				"goal": map[string]any{
					"duration_s": float64(600),
				},
				"intensity": map[string]any{
					"hr_zone": float64(3), // escape hatch
				},
			},
		},
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
	bodyStr := string(capturedBody)
	if !strings.Contains(bodyStr, `"lowerZone":3`) || !strings.Contains(bodyStr, `"upperZone":3`) {
		t.Errorf("expected lowerZone=3, upperZone=3 in body: %s", bodyStr)
	}
}

func TestCreateTrainingTarget_MultipleRepeatBlocks(t *testing.T) {
	var capturedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1"}`))
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

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name": "Multi-interval",
		"date": "2026-05-15",
		"phases": []any{
			map[string]any{
				"type": "repeat",
				"reps": float64(4),
				"goal": map[string]any{"distance_m": float64(400)},
			},
			map[string]any{
				"type": "repeat",
				"reps": float64(3),
				"goal": map[string]any{"distance_m": float64(1000)},
			},
		},
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

	// Verify two repeat nodes appear in the body.
	var parsed map[string]any
	if err := json.Unmarshal(capturedBody, &parsed); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	exercises, _ := parsed["exercise"].([]any)
	if len(exercises) == 0 {
		t.Fatal("no exercises in body")
	}
	exercise0, _ := exercises[0].(map[string]any)
	phases, _ := exercise0["phaseOrRepeat"].([]any)
	if len(phases) != 2 {
		t.Errorf("expected 2 repeat nodes in phaseOrRepeat, got %d", len(phases))
	}
	for i, p := range phases {
		phase, _ := p.(map[string]any)
		if _, hasRepeat := phase["repeatCount"]; !hasRepeat {
			t.Errorf("phase[%d] missing repeatCount", i)
		}
	}
}

func TestCreateTrainingTarget_PolarErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request from polar"))
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

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name": "Test",
		"date": "2026-05-15",
		"phases": []any{
			map[string]any{"type": "warmup", "duration_s": float64(300)},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result on Polar 400, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "status 400") {
		t.Errorf("error text %q should contain 'status 400'", resultText(result))
	}
}

func TestCreateTrainingTarget_InvalidDate(t *testing.T) {
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

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name": "Test",
		"date": "not-a-date",
		"phases": []any{
			map[string]any{"type": "warmup", "duration_s": float64(300)},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result for bad date, got: %s", resultText(result))
	}
	if !strings.Contains(strings.ToUpper(resultText(result)), "ISO 8601") {
		t.Errorf("error text %q should contain 'ISO 8601'", resultText(result))
	}
}

func TestCreateTrainingTarget_EmptyPhases(t *testing.T) {
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

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name":   "Test",
		"date":   "2026-05-15",
		"phases": []any{},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result for empty phases, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "at least one phase") {
		t.Errorf("error text %q should contain 'at least one phase'", resultText(result))
	}
}

func TestCreateTrainingTarget_UnknownPhaseType(t *testing.T) {
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

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(ctx, makeRequest(map[string]any{
		"name": "Test",
		"date": "2026-05-15",
		"phases": []any{
			map[string]any{"type": "sprint"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result for unknown phase type, got: %s", resultText(result))
	}
	if !strings.Contains(resultText(result), "unknown type") {
		t.Errorf("error text %q should contain 'unknown type'", resultText(result))
	}
}
