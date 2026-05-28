package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-faster/jx"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/flow"
	"github.com/lm/polar-flow-mcp/internal/flow/gen"
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
		result := mcpgo.NewToolResultText(string(body))
		result.StructuredContent = map[string]any{"type": "user_info", "data": ui}
		return result, nil
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
			result := mcpgo.NewToolResultText(fmt.Sprintf(
				"No training targets between %s and %s.", from.Format(isoDate), to.Format(isoDate)))
			result.StructuredContent = map[string]any{
				"type": "target_list", "from": from.Format(isoDate), "to": to.Format(isoDate), "targets": []any{},
			}
			return result, nil
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
		result := mcpgo.NewToolResultText(b.String())
		result.StructuredContent = map[string]any{
			"type": "target_list", "from": from.Format(isoDate), "to": to.Format(isoDate), "targets": targets,
		}
		return result, nil
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
		result := mcpgo.NewToolResultText(string(body))
		result.StructuredContent = map[string]any{
			"type": "calendar_events", "from": from.Format(isoDate), "to": to.Format(isoDate), "events": events,
		}
		return result, nil
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
		result := mcpgo.NewToolResultText(string(body))
		result.StructuredContent = map[string]any{
			"type": "session_list", "from": from.Format(isoDate), "to": to.Format(isoDate), "sessions": sessions,
		}
		return result, nil
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
		result := mcpgo.NewToolResultText(string(body))
		result.StructuredContent = map[string]any{"type": "session_summary", "summary": summary}
		return result, nil
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
		result := mcpgo.NewToolResultText(string(body))
		result.StructuredContent = map[string]any{"type": "session_details", "details": details}
		return result, nil
	}
}

// CreateTrainingSessionHandler creates a manual training session entry. This
// is what the Polar Flow web UI calls the "Manual training result" form
// (/exercises/add). The created session counts as a real training session —
// it affects weekly totals, progress summaries, and Polar's training-load
// model. Intended for user-initiated off-watch logging, not synthetic data.
func CreateTrainingSessionHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		name := req.GetString("name", "")
		if strings.TrimSpace(name) == "" {
			return mcpgo.NewToolResultError("name is required"), nil
		}
		dateStr := req.GetString("date", "")
		if strings.TrimSpace(dateStr) == "" {
			return mcpgo.NewToolResultError("date is required (YYYY-MM-DD)"), nil
		}
		day, err := time.ParseInLocation(isoDate, dateStr, time.Local)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("date must be ISO 8601 YYYY-MM-DD: %v", err)), nil
		}
		timeStr := req.GetString("time", "18:00")
		hhmm, err := time.ParseInLocation("15:04", timeStr, time.Local)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("time must be HH:MM 24h: %v", err)), nil
		}
		when := time.Date(day.Year(), day.Month(), day.Day(),
			hhmm.Hour(), hhmm.Minute(), 0, 0, time.Local)
		// Format: YYYY-MM-DDTHH:MM±ZZZZ
		datePayload := when.Format("2006-01-02T15:04-0700")

		duration := int(req.GetFloat("duration_s", 0))
		if duration <= 0 {
			return mcpgo.NewToolResultError("duration_s is required and must be > 0"), nil
		}

		body := &gen.TrainingSessionCreate{
			Date:                         datePayload,
			Sport:                        int(req.GetFloat("sport_id", 1)),
			Duration:                     duration,
			Distance:                     int(req.GetFloat("distance_m", 0)),
			KiloCalories:                 int(req.GetFloat("kcal", 0)),
			Note:                         req.GetString("note", ""),
			TrainingSessionName:          name,
			InterpolatedHeartRateSamples: []jx.Raw{},
			SaveHeartRateSamples:         false,
		}
		// HR: integer-as-string convention, "" when unset.
		if v := int(req.GetFloat("hr_avg", 0)); v > 0 {
			body.HrAverage = fmt.Sprintf("%d", v)
		}
		if v := int(req.GetFloat("hr_max", 0)); v > 0 {
			body.HrMax = fmt.Sprintf("%d", v)
		}
		// SpeedAverage: nullable float (km/h). Omit by leaving NilFloat64 zero
		// (Null:true via SetToNull).
		body.SpeedAverage.SetToNull()
		if v := req.GetFloat("speed_kmh", 0); v > 0 {
			body.SpeedAverage.SetTo(v)
		}
		body.Feeling.SetToNull()

		if err := fc.CreateTrainingSession(ctx, body); err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		return mcpgo.NewToolResultText(fmt.Sprintf(
			"Created training session %q on %s (duration %ds). Note: this writes a real session to your diary and counts toward your training stats.",
			name, datePayload, duration)), nil
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
