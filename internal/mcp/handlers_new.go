package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// GetTrainingTargetHandler returns one target by ID.
func GetTrainingTargetHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, ok := requireInt(req, "target_id")
		if !ok {
			return mcpgo.NewToolResultError("target_id is required and must be a positive integer"), nil
		}
		t, err := fc.GetTrainingTarget(ctx, id)
		if err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No target with id %d.", id)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		body, _ := json.MarshalIndent(t, "", "  ")
		result := mcpgo.NewToolResultText(string(body))
		result.StructuredContent = map[string]any{"type": "target_detail", "id": id, "target": t}
		return result, nil
	}
}

// UpdateTrainingTargetHandler replaces a target's body. Reuses the create-target
// builder, so the user-facing phase array shape is identical.
func UpdateTrainingTargetHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, ok := requireInt(req, "target_id")
		if !ok {
			return mcpgo.NewToolResultError("target_id is required and must be a positive integer"), nil
		}
		body, errMsg := buildTrainingTargetCreate(req)
		if errMsg != "" {
			return mcpgo.NewToolResultError(errMsg), nil
		}
		if err := fc.UpdateTrainingTarget(ctx, id, body); err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No target with id %d.", id)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		return mcpgo.NewToolResultText(fmt.Sprintf("Updated target %d.", id)), nil
	}
}

// GetCalendarWeekSummaryHandler returns weekly totals over the diary date range.
func GetCalendarWeekSummaryHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		now := time.Now().UTC()
		from, to, err := parseFromTo(req, now.AddDate(0, 0, -28), now)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		items, err := fc.GetCalendarWeekSummary(ctx, from, to)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		body, _ := json.MarshalIndent(items, "", "  ")
		result := mcpgo.NewToolResultText(string(body))
		result.StructuredContent = map[string]any{"type": "week_summary", "weeks": items}
		return result, nil
	}
}

// GetProgressSummaryHandler returns aggregated training totals over the range.
func GetProgressSummaryHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		now := time.Now().UTC()
		from, to, err := parseFromTo(req, now.AddDate(0, 0, -90), now)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		sportID := req.GetInt("sport_id", 0)
		timeFrame := req.GetString("time_frame", "3m")
		summary, err := fc.GetProgressViewSummary(ctx, from, to, sportID, timeFrame)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		body, _ := json.MarshalIndent(summary, "", "  ")
		result := mcpgo.NewToolResultText(string(body))
		result.StructuredContent = map[string]any{"type": "progress_summary", "data": summary}
		return result, nil
	}
}
