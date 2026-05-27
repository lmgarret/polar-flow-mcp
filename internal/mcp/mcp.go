// Package mcp registers Polar Flow MCP tools against a shared *flow.Client.
//
// Single-user mode: the server acts as the single Polar account configured in
// POLAR_EMAIL / POLAR_PASSWORD. The proxy-identity layer that lived here
// previously (when the project wrapped the official AccessLink OAuth API) is
// gone. To serve multiple accounts, run one instance per account.
package mcp

import (
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lm/polar-flow-mcp/internal/flow"
)

// RegisterTools registers all Polar Flow MCP tools with the given server.
func RegisterTools(s *server.MCPServer, fc *flow.Client) {
	s.AddTool(mcpgo.NewTool("get_user_info",
		mcpgo.WithDescription(
			"Return identity, country, and basic profile info for the linked Polar Flow account.",
		),
	), GetUserInfoHandler(fc))

	s.AddTool(mcpgo.NewTool("create_training_target",
		mcpgo.WithDescription(
			"Create a Polar Flow training target on a scheduled date with optional phases "+
				"(warmup, repeat intervals, cooldown). Returns the new target's numeric id.",
		),
		mcpgo.WithString("name", mcpgo.Required(),
			mcpgo.Description("Session name shown in the diary (e.g. \"5x1km Threshold\")")),
		mcpgo.WithString("date", mcpgo.Required(),
			mcpgo.Description("Scheduled date ISO 8601 YYYY-MM-DD (e.g. 2026-05-15)")),
		mcpgo.WithString("time",
			mcpgo.Description("Scheduled local time HH:MM (default: 18:00)")),
		mcpgo.WithNumber("sport_id",
			mcpgo.Description("Polar sport ID (default 1 = running). See /api/sports/sports.")),
		mcpgo.WithString("description",
			mcpgo.Description("Free-text notes (optional)")),
		mcpgo.WithArray("phases",
			mcpgo.Description(
				"Ordered phases. Each phase: type=warmup|repeat|cooldown, "+
					"duration_s (warmup/cooldown), reps (repeat), goal{distance_m|duration_s}, "+
					"intensity{label or hr_zone}, optional recovery{duration_s}."),
			mcpgo.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":       map[string]any{"type": "string", "enum": []string{"warmup", "repeat", "cooldown"}},
					"duration_s": map[string]any{"type": "integer"},
					"reps":       map[string]any{"type": "integer"},
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
	), CreateTrainingTargetHandler(fc))

	s.AddTool(mcpgo.NewTool("list_training_targets",
		mcpgo.WithDescription(
			"List Polar Flow training targets in a date range. Defaults: today through +30 days.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date ISO 8601 YYYY-MM-DD (default: today)")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date ISO 8601 YYYY-MM-DD (default: today + 30 days)")),
	), ListTrainingTargetsHandler(fc))

	s.AddTool(mcpgo.NewTool("delete_training_target",
		mcpgo.WithDescription(
			"Delete a Polar Flow training target by its numeric ID.",
		),
		mcpgo.WithNumber("target_id", mcpgo.Required(),
			mcpgo.Description("Numeric target ID from create_training_target or list_training_targets")),
	), DeleteTrainingTargetHandler(fc))

	s.AddTool(mcpgo.NewTool("get_training_target",
		mcpgo.WithDescription(
			"Return the full server-normalized view of a single training target by ID. "+
				"Use this to inspect a target's structure before editing it.",
		),
		mcpgo.WithNumber("target_id", mcpgo.Required(),
			mcpgo.Description("Numeric target ID from list_training_targets")),
	), GetTrainingTargetHandler(fc))

	s.AddTool(mcpgo.NewTool("update_training_target",
		mcpgo.WithDescription(
			"Replace an existing training target's content. Full-replace semantics — "+
				"supply the complete target body (use get_training_target first to read the "+
				"current state, modify, then update).",
		),
		mcpgo.WithNumber("target_id", mcpgo.Required(),
			mcpgo.Description("Numeric ID of the target to update")),
		mcpgo.WithString("name", mcpgo.Required(),
			mcpgo.Description("Session name shown in the diary")),
		mcpgo.WithString("date", mcpgo.Required(),
			mcpgo.Description("Scheduled date ISO 8601 YYYY-MM-DD")),
		mcpgo.WithString("time",
			mcpgo.Description("Scheduled local time HH:MM (default: 18:00)")),
		mcpgo.WithNumber("sport_id",
			mcpgo.Description("Polar sport ID (default 1 = running)")),
		mcpgo.WithString("description",
			mcpgo.Description("Free-text notes (optional)")),
		mcpgo.WithArray("phases",
			mcpgo.Description("Same shape as create_training_target.phases"),
			mcpgo.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":       map[string]any{"type": "string", "enum": []string{"warmup", "repeat", "cooldown"}},
					"duration_s": map[string]any{"type": "integer"},
					"reps":       map[string]any{"type": "integer"},
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
	), UpdateTrainingTargetHandler(fc))

	s.AddTool(mcpgo.NewTool("get_calendar_week_summary",
		mcpgo.WithDescription(
			"Return one summary entry per ISO week intersecting [from, to] — the right-hand "+
				"\"week totals\" strip in the Polar Flow diary. Range capped at 45 days.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date ISO 8601 YYYY-MM-DD (default: today - 28 days)")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date ISO 8601 YYYY-MM-DD (default: today)")),
	), GetCalendarWeekSummaryHandler(fc))

	s.AddTool(mcpgo.NewTool("get_progress_summary",
		mcpgo.WithDescription(
			"Aggregated training totals (sessions, distance, duration, zone time, sport "+
				"distribution, training-benefit distribution) for the [from, to] range. "+
				"Good for coaching context: \"how is my training going this month?\"",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date ISO 8601 YYYY-MM-DD (default: today - 90 days)")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date ISO 8601 YYYY-MM-DD (default: today)")),
		mcpgo.WithNumber("sport_id",
			mcpgo.Description("Optional sport ID filter (omit or 0 for all sports)")),
		mcpgo.WithString("time_frame",
			mcpgo.Description("Bucket size for per-slice breakdowns: \"6w\", \"3m\", or \"1y\" (default: \"3m\")")),
	), GetProgressSummaryHandler(fc))

	s.AddTool(mcpgo.NewTool("get_calendar_events",
		mcpgo.WithDescription(
			"Return the raw Polar Flow calendar events (training targets, exercises, etc.) in a date range.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date ISO 8601 YYYY-MM-DD (default: today)")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date ISO 8601 YYYY-MM-DD (default: today + 30 days)")),
	), GetCalendarEventsHandler(fc))

	s.AddTool(mcpgo.NewTool("list_training_sessions",
		mcpgo.WithDescription(
			"List completed Polar Flow training sessions in a date range. Defaults: last 30 days.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date ISO 8601 YYYY-MM-DD (default: today - 30 days)")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date ISO 8601 YYYY-MM-DD (default: today)")),
	), ListTrainingSessionsHandler(fc))

	s.AddTool(mcpgo.NewTool("get_training_session_summary",
		mcpgo.WithDescription(
			"Return a summary view of a completed training session by its numeric ID.",
		),
		mcpgo.WithNumber("session_id", mcpgo.Required(),
			mcpgo.Description("Numeric session ID from list_training_sessions")),
	), GetTrainingSessionSummaryHandler(fc))

	s.AddTool(mcpgo.NewTool("get_training_session_details",
		mcpgo.WithDescription(
			"Return the lap- and sample-level details for a completed training session.",
		),
		mcpgo.WithNumber("session_id", mcpgo.Required(),
			mcpgo.Description("Numeric session ID from list_training_sessions")),
	), GetTrainingSessionDetailsHandler(fc))
}
