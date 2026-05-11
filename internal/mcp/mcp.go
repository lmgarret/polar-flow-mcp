// Package mcp provides MCP tool registration for the polar-flow-mcp server.
package mcp

import (
	"context"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/store"
)

// RegisterTools registers all MCP tools with the given server (per MCP-01, D-07).
// Phase 2 adds get_user_info; Phase 3 adds create_training_target.
func RegisterTools(s *server.MCPServer, st *store.Store, cipher *crypto.Cipher) {
	// cipher is consumed by Phase 3 tools added in subsequent tasks.
	_ = cipher
	getUserInfoTool := mcpgo.NewTool("get_user_info",
		mcpgo.WithDescription(
			"Returns the Polar account linked to the calling user's proxy identity. "+
				"If no Polar account is linked, returns instructions to visit /oauth/login.",
		),
	)
	s.AddTool(getUserInfoTool, GetUserInfoHandler(st))
}

// GetUserInfoHandler returns the handler closure for the get_user_info tool.
// Exported so tests can invoke it directly without needing a live MCP server transport.
func GetUserInfoHandler(st *store.Store) func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		identity, ok := auth.UserIDFromContext(ctx)
		if !ok {
			return mcpgo.NewToolResultError("no identity in context — auth middleware not applied"), nil
		}

		polarUserID, found, err := st.GetPolarUserID(ctx, identity)
		if err != nil {
			return mcpgo.NewToolResultError("database error: " + err.Error()), nil
		}

		if !found {
			return mcpgo.NewToolResultText(fmt.Sprintf(
				"No Polar account linked to your identity (%s). "+
					"Visit /oauth/login to link your Polar account.",
				identity,
			)), nil
		}

		return mcpgo.NewToolResultText(fmt.Sprintf(
			"Polar account linked.\nIdentity: %s\nPolar user ID: %s",
			identity, polarUserID,
		)), nil
	}
}
