package mcp

import (
	"context"
	"fmt"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/flow"
	"github.com/lm/polar-flow-mcp/internal/flow/gen"
)

// CreateTrainingTargetHandler builds a TrainingTargetCreate from the Claude-
// facing phase array and POSTs it to /api/trainingtarget.
func CreateTrainingTargetHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		body, errMsg := buildTrainingTargetCreate(req)
		if errMsg != "" {
			return mcpgo.NewToolResultError(errMsg), nil
		}
		id, err := fc.CreateTrainingTarget(ctx, body)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		result := mcpgo.NewToolResultText(fmt.Sprintf("Created training target %d (%q at %s).", id, body.Name, body.Datetime))
		result.StructuredContent = map[string]any{"type": "target_created", "id": id, "name": body.Name, "datetime": body.Datetime}
		return result, nil
	}
}

// buildTrainingTargetCreate parses the user-facing create/update arguments into
// the wire-format TrainingTargetCreate body. On validation failure it returns
// an empty body and a non-empty error message suitable for direct surfacing
// via NewToolResultError.
func buildTrainingTargetCreate(req mcpgo.CallToolRequest) (*gen.TrainingTargetCreate, string) {
	name := req.GetString("name", "")
	if name == "" {
		return nil, "name is required"
	}
	date := req.GetString("date", "")
	if date == "" {
		return nil, "date is required (YYYY-MM-DD)"
	}
	if _, err := time.Parse(isoDate, date); err != nil {
		return nil, "date must be ISO 8601 YYYY-MM-DD: " + err.Error()
	}
	clock := req.GetString("time", "18:00")
	if _, err := time.Parse("15:04", clock); err != nil {
		return nil, "time must be HH:MM (24h): " + err.Error()
	}
	sportID := req.GetInt("sport_id", 1)
	description := req.GetString("description", "")

	args, _ := req.GetArguments()["phases"].([]any)
	phases, err := buildPhases(args)
	if err != nil {
		return nil, err.Error()
	}

	targetType := gen.TrainingTargetCreateTypeVOLUME
	if len(phases) > 0 {
		targetType = gen.TrainingTargetCreateTypePHASED
	}
	body := &gen.TrainingTargetCreate{
		Type:     targetType,
		Name:     name,
		Datetime: date + "T" + clock,
		ExerciseTargets: []gen.ExerciseTarget{{
			SportId: sportID,
			Phases:  phases,
		}},
	}
	if description != "" {
		body.Description.SetTo(description)
	}
	return body, ""
}

// hrZoneForLabel maps an effort label to a (lower, upper) Polar HR zone pair.
// Conservative defaults — Claude can override by passing hr_zone explicitly.
func hrZoneForLabel(label string) (int, int, bool) {
	switch label {
	case "easy":
		return 1, 2, true
	case "aerobic":
		return 2, 2, true
	case "tempo":
		return 3, 3, true
	case "threshold":
		return 4, 4, true
	case "vo2max":
		return 5, 5, true
	default:
		return 0, 0, false
	}
}

// buildPhases turns the user-facing flat phase array into Polar Flow's
// PHASE/REPEAT sum-type list. Warmup and cooldown become single PhaseLeaf
// entries; repeat becomes a PhaseRepeat with nested PhaseLeaf{work, recovery}.
//
//nolint:gocyclo // straight-line builder over a small enum
func buildPhases(args []any) ([]gen.Phase, error) {
	out := make([]gen.Phase, 0, len(args))
	for i, raw := range args {
		p, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("phases[%d]: not an object", i)
		}
		ptype, _ := p["type"].(string)
		switch ptype {
		case "warmup":
			leaf, err := buildSimpleLeaf(p, "Warm-up", "AUTOMATIC")
			if err != nil {
				return nil, fmt.Errorf("phases[%d] (warmup): %w", i, err)
			}
			out = append(out, gen.Phase{Type: gen.PhaseLeafPhase, PhaseLeaf: leaf})
		case "cooldown":
			leaf, err := buildSimpleLeaf(p, "Cool-down", "AUTOMATIC")
			if err != nil {
				return nil, fmt.Errorf("phases[%d] (cooldown): %w", i, err)
			}
			out = append(out, gen.Phase{Type: gen.PhaseLeafPhase, PhaseLeaf: leaf})
		case "repeat":
			repeat, err := buildRepeat(p)
			if err != nil {
				return nil, fmt.Errorf("phases[%d] (repeat): %w", i, err)
			}
			out = append(out, gen.Phase{Type: gen.PhaseRepeatPhase, PhaseRepeat: repeat})
		default:
			return nil, fmt.Errorf("phases[%d]: unknown type %q (want warmup|repeat|cooldown)", i, ptype)
		}
	}
	return out, nil
}

// buildSimpleLeaf constructs a duration-goal warm-up / cool-down leaf.
func buildSimpleLeaf(p map[string]any, name, changeType string) (gen.PhaseLeaf, error) {
	dur, ok := goalDuration(p, "duration_s")
	if !ok {
		return gen.PhaseLeaf{}, fmt.Errorf("duration_s is required and must be > 0")
	}
	leaf := gen.PhaseLeaf{
		PhaseType:       "PHASE",
		Name:            name,
		PhaseChangeType: gen.PhaseLeafPhaseChangeType(changeType),
		GoalType:        "DURATION",
		IntensityType:   "NONE",
	}
	leaf.Duration.SetTo(dur)
	// Distance / LowerZone / UpperZone left at null defaults.
	return leaf, nil
}

// buildRepeat constructs a repeat group with a work leaf and optional recovery leaf.
func buildRepeat(p map[string]any) (gen.PhaseRepeat, error) {
	reps, ok := p["reps"].(float64)
	if !ok || reps < 2 {
		return gen.PhaseRepeat{}, fmt.Errorf("reps is required and must be >= 2")
	}
	goal, _ := p["goal"].(map[string]any)
	work, err := buildWorkLeaf(goal, p)
	if err != nil {
		return gen.PhaseRepeat{}, fmt.Errorf("work: %w", err)
	}
	leaves := []gen.PhaseLeaf{work}
	if rec, ok := p["recovery"].(map[string]any); ok {
		if dur, ok := goalDuration(rec, "duration_s"); ok {
			recLeaf := gen.PhaseLeaf{
				PhaseType:       "PHASE",
				Name:            "Recovery",
				PhaseChangeType: "AUTOMATIC",
				GoalType:        "DURATION",
				IntensityType:   "NONE",
			}
			recLeaf.Duration.SetTo(dur)
			leaves = append(leaves, recLeaf)
		}
	}
	return gen.PhaseRepeat{
		PhaseType:   "REPEAT",
		RepeatCount: int(reps),
		Phases:      leaves,
	}, nil
}

// buildWorkLeaf constructs the leaf that does the actual interval work.
func buildWorkLeaf(goal, intensityCtx map[string]any) (gen.PhaseLeaf, error) {
	leaf := gen.PhaseLeaf{
		PhaseType:       "PHASE",
		Name:            "Work",
		PhaseChangeType: "AUTOMATIC",
		IntensityType:   "NONE",
	}
	switch {
	case goal != nil && goalHasFloat(goal, "distance_m"):
		v, _ := goal["distance_m"].(float64)
		dist := int(v)
		if dist <= 0 {
			return leaf, fmt.Errorf("goal.distance_m must be > 0")
		}
		leaf.GoalType = "DISTANCE"
		leaf.Distance.SetTo(float64(dist))
	case goal != nil && goalHasFloat(goal, "duration_s"):
		dur, _ := goalDuration(goal, "duration_s")
		leaf.GoalType = "DURATION"
		leaf.Duration.SetTo(dur)
	default:
		return leaf, fmt.Errorf("goal must specify distance_m or duration_s")
	}
	if intensity, ok := intensityCtx["intensity"].(map[string]any); ok {
		applyIntensity(&leaf, intensity)
	}
	return leaf, nil
}

func applyIntensity(leaf *gen.PhaseLeaf, intensity map[string]any) {
	if zone, ok := intensity["hr_zone"].(float64); ok && zone >= 1 && zone <= 5 {
		leaf.IntensityType = "HEART_RATE_ZONES"
		leaf.LowerZone.SetTo(zone)
		leaf.UpperZone.SetTo(zone)
		return
	}
	if label, ok := intensity["label"].(string); ok {
		if lo, hi, ok := hrZoneForLabel(label); ok {
			leaf.IntensityType = "HEART_RATE_ZONES"
			leaf.LowerZone.SetTo(float64(lo))
			leaf.UpperZone.SetTo(float64(hi))
		}
	}
}

func goalHasFloat(m map[string]any, key string) bool {
	v, ok := m[key].(float64)
	return ok && v > 0
}

// goalDuration extracts an int seconds value and formats it as the HH:MM:SS
// string the API wants.
func goalDuration(m map[string]any, key string) (string, bool) {
	v, ok := m[key].(float64)
	if !ok || v <= 0 {
		return "", false
	}
	secs := int(v)
	h := secs / 3600
	min := (secs % 3600) / 60
	s := secs % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, min, s), true
}
