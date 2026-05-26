package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/flow"
)

const isoDate = "2006-01-02"

// GetUserInfoHandler returns the current Polar Flow account identity.
func GetUserInfoHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		ui, err := fc.GetUserInfo(ctx)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		body, _ := json.MarshalIndent(ui, "", "  ")
		return mcpgo.NewToolResultText(string(body)), nil
	}
}

// ListTrainingTargetsHandler lists scheduled training targets in [from, to].
func ListTrainingTargetsHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		now := time.Now().UTC()
		from, to, err := parseFromTo(req, now, now.AddDate(0, 0, 30))
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		targets, err := fc.ListTrainingTargets(ctx, from, to)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if len(targets) == 0 {
			return mcpgo.NewToolResultText(fmt.Sprintf(
				"No training targets between %s and %s.", from.Format(isoDate), to.Format(isoDate))), nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Training targets between %s and %s (%d):\n",
			from.Format(isoDate), to.Format(isoDate), len(targets))
		for _, t := range targets {
			title := t.Title
			if title == "" {
				title = "(unnamed)"
			}
			fmt.Fprintf(&b, "- %d: %s (%s)\n", t.ID, title, t.Start)
		}
		return mcpgo.NewToolResultText(b.String()), nil
	}
}

// DeleteTrainingTargetHandler deletes a training target by id.
func DeleteTrainingTargetHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, ok := requireInt(req, "target_id")
		if !ok {
			return mcpgo.NewToolResultError("target_id is required and must be a positive integer"), nil
		}
		if err := fc.DeleteTrainingTarget(ctx, id); err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No target with id %d.", id)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		return mcpgo.NewToolResultText(fmt.Sprintf("Deleted target %d.", id)), nil
	}
}

// GetCalendarEventsHandler returns raw calendar events in [from, to].
func GetCalendarEventsHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		now := time.Now().UTC()
		from, to, err := parseFromTo(req, now, now.AddDate(0, 0, 30))
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		events, err := fc.GetCalendarEvents(ctx, from, to)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		body, _ := json.MarshalIndent(events, "", "  ")
		return mcpgo.NewToolResultText(string(body)), nil
	}
}

// ListTrainingSessionsHandler lists completed training sessions in [from, to].
func ListTrainingSessionsHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		now := time.Now().UTC()
		from, to, err := parseFromTo(req, now.AddDate(0, 0, -30), now)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		// Need userID for listTrainingSessions — fetch it lazily.
		ui, err := fc.GetUserInfo(ctx)
		if err != nil {
			return mcpgo.NewToolResultError("could not resolve userId: " + err.Error()), nil
		}
		sessions, err := fc.ListTrainingSessions(ctx, ui.ID, from, to)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		body, _ := json.MarshalIndent(sessions, "", "  ")
		return mcpgo.NewToolResultText(string(body)), nil
	}
}

// GetTrainingSessionSummaryHandler returns the summary view of a session.
func GetTrainingSessionSummaryHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, ok := requireInt(req, "session_id")
		if !ok {
			return mcpgo.NewToolResultError("session_id is required and must be a positive integer"), nil
		}
		summary, err := fc.GetTrainingSessionSummary(ctx, id)
		if err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No session with id %d.", id)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		body, _ := json.MarshalIndent(summary, "", "  ")
		return mcpgo.NewToolResultText(string(body)), nil
	}
}

// GetTrainingSessionDetailsHandler returns lap/sample-level detail for a session.
func GetTrainingSessionDetailsHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, ok := requireInt(req, "session_id")
		if !ok {
			return mcpgo.NewToolResultError("session_id is required and must be a positive integer"), nil
		}
		details, err := fc.GetTrainingSessionDetails(ctx, id)
		if err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No session with id %d.", id)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		body, _ := json.MarshalIndent(details, "", "  ")
		return mcpgo.NewToolResultText(string(body)), nil
	}
}

// parseFromTo extracts from_date / to_date with ISO-8601 validation, falling
// back to the supplied defaults if absent.
func parseFromTo(req mcpgo.CallToolRequest, defFrom, defTo time.Time) (time.Time, time.Time, error) {
	fromStr := req.GetString("from_date", defFrom.Format(isoDate))
	toStr := req.GetString("to_date", defTo.Format(isoDate))
	from, err := time.Parse(isoDate, fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("from_date must be ISO 8601 YYYY-MM-DD: %w", err)
	}
	to, err := time.Parse(isoDate, toStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("to_date must be ISO 8601 YYYY-MM-DD: %w", err)
	}
	return from, to, nil
}

// requireInt extracts a positive int64 from the call args under the given name.
func requireInt(req mcpgo.CallToolRequest, name string) (int64, bool) {
	// mcp-go normalizes JSON numbers to float64.
	v := req.GetFloat(name, 0)
	if v <= 0 {
		return 0, false
	}
	return int64(v), true
}
