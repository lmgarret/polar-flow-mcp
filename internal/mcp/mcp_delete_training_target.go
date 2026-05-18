// Package mcp: delete_training_target tool handler (per MCP-05, MCP-06, MCP-07).
package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/store"
)

// DeleteTrainingTargetHandler returns the handler closure for delete_training_target (per MCP-05).
// The Polar v4 API is read-only for training targets; this always returns an informational error.
func DeleteTrainingTargetHandler(_ *store.Store, _ *crypto.Cipher) func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return mcpgo.NewToolResultError(
			"Deleting training targets is not supported by the Polar v4 API. " +
				"Training targets can only be deleted in the Polar Flow app."), nil
	}
}
