//go:build polartest

package mcp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/mcp"
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

// TestCreateTrainingTarget_NotSupported verifies the stub returns an informational
// error explaining that the Polar v4 API is read-only for training targets.
func TestCreateTrainingTarget_NotSupported(t *testing.T) {
	st := openTestStore(t)
	cipher := newTestCipher(t)

	handler := mcp.CreateTrainingTargetHandler(st, cipher)
	result, err := handler(context.Background(), makeRequest(map[string]any{
		"name":   "5x1km Threshold",
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
