// Package mcp: create_training_target tool handler (per MCP-02, MCP-03, MCP-06, MCP-07).
package mcp

import (
	"context"
	"encoding/json"
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

// labelToZone maps coaching intensity labels (per CONTEXT.md D-06) to Polar HR zones 1-5.
//
//nolint:gochecknoglobals
var labelToZone = map[string]int{
	"easy":      1,
	"aerobic":   2,
	"tempo":     3,
	"threshold": 4,
	"vo2max":    5,
}

// resolveZone returns the HR zone (1-5) given hrZone and label.
// hrZone takes priority when in range 1-5; otherwise label is mapped.
// Returns (0, false) if neither resolves.
func resolveZone(hrZone int, label string) (int, bool) {
	if hrZone >= 1 && hrZone <= 5 {
		return hrZone, true
	}
	if z, ok := labelToZone[strings.ToLower(strings.TrimSpace(label))]; ok {
		return z, true
	}
	return 0, false
}

// phaseInput represents a single element of the phases array passed by the caller.
type phaseInput struct {
	Type      string `json:"type"`
	DurationS int64  `json:"duration_s"`
	Reps      int    `json:"reps"`
	Goal      *struct {
		DistanceM float64 `json:"distance_m"`
		DurationS int64   `json:"duration_s"`
	} `json:"goal"`
	Intensity *struct {
		Label  string  `json:"label"`
		HRZone float64 `json:"hr_zone"` // float64 because JSON numbers from map[string]any are float64.
	} `json:"intensity"`
	Recovery *struct {
		DurationS int64 `json:"duration_s"`
	} `json:"recovery"`
}

// buildRepeatPhase converts a repeat phaseInput into a PhaseOrRepeat with nested work/recovery.
func buildRepeatPhase(i int, p phaseInput) (polar.PhaseOrRepeat, error) {
	if p.Reps < 1 {
		return polar.PhaseOrRepeat{}, fmt.Errorf("phase %d: repeat requires reps >= 1", i)
	}
	if p.Goal == nil {
		return polar.PhaseOrRepeat{}, fmt.Errorf("phase %d: repeat requires goal", i)
	}

	work := polar.PhaseOrRepeat{Name: "work", ChangeType: "AUTOMATIC"}
	switch {
	case p.Goal.DistanceM > 0:
		d := p.Goal.DistanceM
		work.Goal = polar.PhaseGoal{Type: "DISTANCE", Distance: &d}
	case p.Goal.DurationS > 0:
		ms := p.Goal.DurationS * 1000
		work.Goal = polar.PhaseGoal{Type: "DURATION", Duration: &ms}
	default:
		return polar.PhaseOrRepeat{}, fmt.Errorf("phase %d: repeat goal must specify distance_m or duration_s", i)
	}

	hrZone := 0
	label := ""
	if p.Intensity != nil {
		hrZone = int(p.Intensity.HRZone)
		label = p.Intensity.Label
	}
	if zone, ok := resolveZone(hrZone, label); ok {
		work.Intensity = &polar.PhaseIntensity{Type: "HEART_RATE_ZONES", LowerZone: &zone, UpperZone: &zone}
	} else {
		work.Intensity = &polar.PhaseIntensity{Type: "NONE"}
	}

	repeatChildren := []polar.PhaseOrRepeat{work}
	if p.Recovery != nil && p.Recovery.DurationS > 0 {
		rms := p.Recovery.DurationS * 1000
		repeatChildren = append(repeatChildren, polar.PhaseOrRepeat{
			Name:       "recovery",
			ChangeType: "AUTOMATIC",
			Goal:       polar.PhaseGoal{Type: "DURATION", Duration: &rms},
			Intensity:  &polar.PhaseIntensity{Type: "NONE"},
		})
	}
	reps := p.Reps
	return polar.PhaseOrRepeat{
		Name:          "intervals",
		ChangeType:    "AUTOMATIC",
		Goal:          polar.PhaseGoal{Type: "MANUAL"},
		RepeatCount:   &reps,
		PhaseOrRepeat: repeatChildren,
	}, nil
}

// flatToTree converts the flat phases array into Polar's nested PhaseOrRepeat tree.
func flatToTree(phases []phaseInput) ([]polar.PhaseOrRepeat, error) {
	children := make([]polar.PhaseOrRepeat, 0, len(phases))
	for i, p := range phases {
		phaseType := strings.ToLower(strings.TrimSpace(p.Type))
		switch phaseType {
		case "warmup":
			ms := p.DurationS * 1000
			children = append(children, polar.PhaseOrRepeat{
				Name:       "warmup",
				ChangeType: "AUTOMATIC",
				Goal:       polar.PhaseGoal{Type: "DURATION", Duration: &ms},
				Intensity:  &polar.PhaseIntensity{Type: "NONE"},
			})
		case "cooldown":
			ms := p.DurationS * 1000
			children = append(children, polar.PhaseOrRepeat{
				Name:       "cooldown",
				ChangeType: "AUTOMATIC",
				Goal:       polar.PhaseGoal{Type: "DURATION", Duration: &ms},
				Intensity:  &polar.PhaseIntensity{Type: "NONE"},
			})
		case "repeat":
			node, err := buildRepeatPhase(i, p)
			if err != nil {
				return nil, err
			}
			children = append(children, node)
		default:
			return nil, fmt.Errorf("phase %d: unknown type %q (expected warmup|repeat|cooldown)", i, p.Type)
		}
	}
	return children, nil
}

// parsePhasesFromRequest extracts and validates the phases array from the request arguments.
func parsePhasesFromRequest(req mcpgo.CallToolRequest) ([]phaseInput, error) {
	argsBytes, err := json.Marshal(req.GetArguments())
	if err != nil {
		return nil, fmt.Errorf("internal: marshal args: %w", err)
	}
	var parsedArgs struct {
		Phases []phaseInput `json:"phases"`
	}
	if err := json.Unmarshal(argsBytes, &parsedArgs); err != nil {
		return nil, fmt.Errorf("phases must be an array of objects: %w", err)
	}
	if len(parsedArgs.Phases) == 0 {
		return nil, fmt.Errorf("phases array must contain at least one phase")
	}
	if len(parsedArgs.Phases) > 50 {
		return nil, fmt.Errorf("phases array exceeds limit of 50")
	}
	return parsedArgs.Phases, nil
}

// CreateTrainingTargetHandler returns the handler closure for the create_training_target tool.
// Exported so tests can invoke it directly without needing a live MCP transport.
func CreateTrainingTargetHandler(st *store.Store, cipher *crypto.Cipher) func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
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

		name := req.GetString("name", "")
		if name == "" {
			return mcpgo.NewToolResultError("name is required"), nil
		}
		dateStr := req.GetString("date", "")
		if dateStr == "" {
			return mcpgo.NewToolResultError("date is required (ISO 8601, e.g. 2026-05-15)"), nil
		}
		timeStr := req.GetString("time", "18:00")
		// sport is currently informational; not sent as sportId per RESEARCH.md Open Question 2.
		_ = req.GetString("sport", "RUNNING")

		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return mcpgo.NewToolResultError("date must be ISO 8601 (YYYY-MM-DD): " + err.Error()), nil
		}
		parsedTime, err := time.Parse("15:04", timeStr)
		if err != nil {
			return mcpgo.NewToolResultError("time must be HH:MM (e.g. 18:00): " + err.Error()), nil
		}

		phases, err := parsePhasesFromRequest(req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}

		children, err := flatToTree(phases)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}

		body := polar.CreateTrainingTargetRequest{
			Session: polar.TrainingSessionTarget{
				Name: name,
				StartTime: polar.DateTime{
					Year:  parsedDate.Year(),
					Month: int(parsedDate.Month()),
					Day:   parsedDate.Day(),
					Hour:  parsedTime.Hour(),
					Min:   parsedTime.Minute(),
					Sec:   0,
				},
			},
			Exercise: []polar.ExerciseTarget{{
				Idx:           0,
				Type:          "PHASED",
				PhaseOrRepeat: children,
			}},
		}

		client := polar.NewClient(string(tokenBytes))
		id, err := client.CreateTrainingTarget(ctx, polarUserID, body)
		if err != nil {
			slog.Error("polar create training target failed", "identity", identity, "polar_user_id", polarUserID, "error", err)
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		slog.Info("training target created", "identity", identity, "polar_user_id", polarUserID, "target_id", id)
		return mcpgo.NewToolResultText(fmt.Sprintf(
			"Training target created. ID: %s. Scheduled: %s %s. Phases: %d.",
			id, dateStr, timeStr, len(phases),
		)), nil
	}
}
