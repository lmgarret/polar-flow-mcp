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
	getUserInfoTool := mcpgo.NewTool("get_user_info",
		mcpgo.WithDescription(
			"Returns the Polar account linked to the calling user's proxy identity. "+
				"If no Polar account is linked, returns instructions to visit /oauth/login.",
		),
	)
	s.AddTool(getUserInfoTool, GetUserInfoHandler(st))

	createTool := mcpgo.NewTool("create_training_target",
		mcpgo.WithDescription(
			"Create a Polar Flow training target with phases (warmup, repeat intervals, cooldown). "+
				"Claude constructs the full phases array; the handler maps intensity labels to HR zones "+
				"and builds the nested Polar API JSON. See the polar-coach skill for examples.",
		),
		mcpgo.WithString("name", mcpgo.Required(), mcpgo.Description("Session name, max 45 chars (e.g. \"5x1km Threshold\")")),
		mcpgo.WithString("date", mcpgo.Required(), mcpgo.Description("Scheduled date ISO 8601 YYYY-MM-DD (e.g. 2026-05-15)")),
		mcpgo.WithString("sport", mcpgo.Description("Sport type (default: RUNNING)")),
		mcpgo.WithString("time", mcpgo.Description("Scheduled time HH:MM (default: 18:00)")),
		mcpgo.WithArray("phases", mcpgo.Required(),
			mcpgo.Description("Ordered phases: warmup/repeat/cooldown. Warmup and cooldown have duration_s. Repeat has reps, goal{distance_m|duration_s}, intensity{label|hr_zone}, optional recovery{duration_s}."),
			mcpgo.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":       map[string]any{"type": "string", "enum": []string{"warmup", "repeat", "cooldown"}},
					"duration_s": map[string]any{"type": "integer", "description": "Duration in seconds (warmup/cooldown)"},
					"reps":       map[string]any{"type": "integer", "description": "Repeat count (repeat type only)"},
					"goal": map[string]any{"type": "object", "properties": map[string]any{
						"distance_m": map[string]any{"type": "number"},
						"duration_s": map[string]any{"type": "integer"},
					}},
					"intensity": map[string]any{"type": "object", "properties": map[string]any{
						"label":   map[string]any{"type": "string", "enum": []string{"easy", "aerobic", "tempo", "threshold", "vo2max"}},
						"hr_zone": map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
					}},
					"recovery": map[string]any{"type": "object", "properties": map[string]any{
						"duration_s": map[string]any{"type": "integer"},
					}},
				},
				"required": []string{"type"},
			}),
		),
	)
	s.AddTool(createTool, CreateTrainingTargetHandler(st, cipher))

	listTool := mcpgo.NewTool("list_training_targets",
		mcpgo.WithDescription(
			"List Polar Flow training targets in a date range. Defaults: today through +30 days (UTC).",
		),
		mcpgo.WithString("from_date", mcpgo.Description("Start date ISO 8601 YYYY-MM-DD (default: today)")),
		mcpgo.WithString("to_date", mcpgo.Description("End date ISO 8601 YYYY-MM-DD (default: today + 30 days)")),
	)
	s.AddTool(listTool, ListTrainingTargetsHandler(st, cipher))

	deleteTool := mcpgo.NewTool("delete_training_target",
		mcpgo.WithDescription(
			"Delete a Polar Flow training target by its ID. Returns a clear message if the target does not exist.",
		),
		mcpgo.WithString("target_id", mcpgo.Required(),
			mcpgo.Description("Target ID returned by create_training_target or list_training_targets")),
	)
	s.AddTool(deleteTool, DeleteTrainingTargetHandler(st, cipher))
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
