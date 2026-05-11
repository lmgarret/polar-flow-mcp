// Package mcp: delete_training_target tool handler (per MCP-05, MCP-06, MCP-07).
package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/polar"
	"github.com/lm/polar-flow-mcp/internal/store"
)

// DeleteTrainingTargetHandler returns the handler closure for delete_training_target (per MCP-05).
// Returns ToolResultText (NOT error) on 404 — "not found" is informational, not a failure.
func DeleteTrainingTargetHandler(st *store.Store, cipher *crypto.Cipher) func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
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
				"No Polar account linked to your identity (%s). Visit /oauth/login to link your Polar account.",
				identity,
			)), nil
		}

		blob, found, err := st.GetEncryptedToken(ctx, identity)
		if err != nil {
			return mcpgo.NewToolResultError("database error: " + err.Error()), nil
		}
		if !found {
			return mcpgo.NewToolResultText(fmt.Sprintf(
				"No Polar token stored for your identity (%s). Visit /oauth/login to re-link your Polar account.",
				identity,
			)), nil
		}

		tokenBytes, err := cipher.Decrypt(blob)
		if err != nil {
			slog.Error("token decryption failed", "identity", identity, "error", err)
			return mcpgo.NewToolResultError("token decryption error: " + err.Error()), nil
		}

		targetID := req.GetString("target_id", "")
		if targetID == "" {
			return mcpgo.NewToolResultError("target_id is required"), nil
		}

		client := polar.NewClient(string(tokenBytes))
		if deleteErr := client.DeleteTrainingTarget(ctx, polarUserID, targetID); deleteErr != nil {
			if errors.Is(deleteErr, polar.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf(
					"Training target %s not found. It may have already been deleted.",
					targetID,
				)), nil
			}
			slog.Error("polar delete training target failed",
				"identity", identity,
				"polar_user_id", polarUserID,
				"target_id", targetID,
				"error", deleteErr,
			)
			return mcpgo.NewToolResultError(deleteErr.Error()), nil
		}
		return mcpgo.NewToolResultText(fmt.Sprintf("Training target %s deleted.", targetID)), nil
	}
}
