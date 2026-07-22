package mcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
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
		return widgetResult(map[string]any{
			"type": "target_detail", "id": id, "target": t,
			"sport_category": convert.SportCategory(t.Name, 0),
		}), nil
	}
}

// UpdateTrainingTargetHandler replaces a target's body. Reuses the create-target
// builder, so the user-facing phase array shape is identical.
//
// The update is a full replace, but Polar keys the target by its server-assigned
// exerciseTargets[].id. A create-shaped body omits that id, and Polar then treats
// the request as a *new* target landing on an already-occupied time slot — it
// rejects with 400 error.trainingTarget.twoTargetsForSameTime (or, in other
// shapes, silently no-ops the phases). So we first read the live target and carry
// its exerciseTargets[].id onto the rebuilt body before sending.
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
		existing, err := fc.GetTrainingTarget(ctx, id)
		if err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No target with id %d.", id)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		preserveExerciseTargetIDs(body, existing.ExerciseTargets)
		if err := fc.UpdateTrainingTarget(ctx, id, body); err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No target with id %d.", id)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		// Read the target back so the result carries the same server-normalized
		// view as get_training_target — the model (and the MCP-app UI) get the
		// full post-update state without a follow-up get_training_target call.
		// The update itself already succeeded, so a failed read-back is not fatal:
		// fall back to the plain confirmation.
		updated, err := fc.GetTrainingTarget(ctx, id)
		if err != nil {
			return mcpgo.NewToolResultText(fmt.Sprintf("Updated target %d.", id)), nil
		}
		return widgetResultText(fmt.Sprintf("Updated target %d.", id), map[string]any{
			"type": "target_detail", "id": id, "target": updated,
			"sport_category": convert.SportCategory(updated.Name, 0),
		}), nil
	}
}

// preserveExerciseTargetIDs copies the server-assigned exerciseTargets[].id from
// the live target onto the rebuilt update body, matched by position. Polar uses
// these ids to recognise which target/exercise the full-replace update refers to;
// dropping them breaks the update (see UpdateTrainingTargetHandler). Phase ids are
// not carried — they round-trip safely as null.
func preserveExerciseTargetIDs(body *gen.TrainingTargetCreate, existing []gen.ExerciseTarget) {
	if body == nil {
		return
	}
	for i := range body.ExerciseTargets {
		if i < len(existing) {
			body.ExerciseTargets[i].ID = existing[i].ID
		}
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
		return widgetResult(map[string]any{"type": "week_summary", "weeks": items}), nil
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
		group := req.GetString("group", "MONTH")
		timeFrame := req.GetString("time_frame", "3m")
		summary, err := fc.GetProgressViewSummary(ctx, from, to, group, timeFrame)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		dto := convert.FromWireProgressSummary(summary, from.Format(isoDate), to.Format(isoDate))
		return widgetResult(map[string]any{"type": "progress_summary", "data": dto}), nil
	}
}
