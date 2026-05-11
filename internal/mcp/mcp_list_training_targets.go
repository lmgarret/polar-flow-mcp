// Package mcp: list_training_targets tool handler (per MCP-04, MCP-06, MCP-07).
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/crypto"
	"github.com/lm/polar-flow-mcp/internal/polar"
	"github.com/lm/polar-flow-mcp/internal/store"
)

// ListTrainingTargetsHandler returns the handler closure for list_training_targets (per MCP-04).
// Defaults from_date to today and to_date to today+30 days when omitted (per D-09).
func ListTrainingTargetsHandler(st *store.Store, cipher *crypto.Cipher) func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
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

		today := time.Now().UTC().Format("2006-01-02")
		defaultTo := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")
		fromDate := req.GetString("from_date", today)
		toDate := req.GetString("to_date", defaultTo)

		// Light validation: confirm parseable when provided non-empty.
		if _, parseErr := time.Parse("2006-01-02", fromDate); parseErr != nil {
			return mcpgo.NewToolResultError("from_date must be ISO 8601 YYYY-MM-DD: " + parseErr.Error()), nil
		}
		if _, parseErr := time.Parse("2006-01-02", toDate); parseErr != nil {
			return mcpgo.NewToolResultError("to_date must be ISO 8601 YYYY-MM-DD: " + parseErr.Error()), nil
		}

		client := polar.NewClient(string(tokenBytes))
		targets, err := client.ListTrainingTargets(ctx, polarUserID, fromDate, toDate)
		if err != nil {
			slog.Error("polar list training targets failed", "identity", identity, "polar_user_id", polarUserID, "error", err)
			return mcpgo.NewToolResultError(err.Error()), nil
		}

		if len(targets) == 0 {
			return mcpgo.NewToolResultText(fmt.Sprintf("No training targets between %s and %s.", fromDate, toDate)), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Training targets between %s and %s (%d):\n", fromDate, toDate, len(targets))
		for _, t := range targets {
			id := t.ID
			if id == "" {
				id = "(no id)"
			}
			name := t.Name
			if name == "" {
				name = "(unnamed)"
			}
			dt := strings.TrimSpace(t.Date + " " + t.Time)
			if dt == "" {
				dt = "(no scheduled time)"
			}
			fmt.Fprintf(&b, "- %s: %s (%s)\n", id, name, dt)
		}
		return mcpgo.NewToolResultText(b.String()), nil
	}
}
