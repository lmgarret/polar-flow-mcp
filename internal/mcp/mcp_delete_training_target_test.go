//go:build polartest

package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lm/polar-flow-mcp/internal/mcp"
)

// TestDeleteTrainingTarget_NotSupported verifies the stub returns an informational
// error explaining that the Polar v4 API is read-only for training targets.
func TestDeleteTrainingTarget_NotSupported(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)

	handler := mcp.DeleteTrainingTargetHandler(st, cipher)
	result, err := handler(context.Background(), makeRequest(map[string]any{
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
		t.Fatalf("expected error result (not supported), got success: %s", resultText(result))
	}
	text := resultText(result)
	if !strings.Contains(strings.ToLower(text), "not supported") {
		t.Errorf("error text %q should contain 'not supported'", text)
	}
	if !strings.Contains(text, "Polar Flow app") {
		t.Errorf("error text %q should mention 'Polar Flow app'", text)
	}
}
