package mcp

import (
	"context"
	"errors"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Training-target text limits, probed 2026-09-29 and counted in UTF-16 code
// units (convert.PolarTextLen). Flow answers an over-long name with the same
// opaque message as its name content filter, and an over-long *phase* name
// with a 400 that still stores a half-created, calendar-invisible target
// holding the time slot — so all three are checked before sending.
const (
	maxTargetNameLen = 45
	maxTargetDescLen = 500
	maxPhaseNameLen  = 45
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
			return targetWriteError(err, body.Datetime), nil
		}
		return widgetResultText(
			fmt.Sprintf("Created training target %d (%q at %s).", id, body.Name, body.Datetime),
			map[string]any{"type": "target_created", "id": id, "name": body.Name, "datetime": body.Datetime}), nil
	}
}

// targetWriteError renders a failed create/update. A time clash gets an
// actionable message: Flow refuses two targets on the same minute, and the
// occupant can be invisible (a half-created target left by an earlier create
// that Flow rejected for a phase name).
func targetWriteError(err error, datetime string) *mcpgo.CallToolResult {
	if errors.Is(err, flow.ErrTargetTimeClash) {
		return mcpgo.NewToolResultError(fmt.Sprintf("Polar already has a training target at %s and refuses two "+
			"at the same minute — pick another time (even a minute later works). If list_training_targets "+
			"shows nothing there, the slot is held by a hidden, half-created target from an earlier failed "+
			"create; it can only be removed by id.", datetime))
	}
	return mcpgo.NewToolResultError(err.Error())
}

// buildTrainingTargetCreate parses the user-facing create/update arguments into
// the wire-format TrainingTargetCreate body. On validation failure it returns
// an empty body and a non-empty error message suitable for direct surfacing
// via NewToolResultError.
func buildTrainingTargetCreate(req mcpgo.CallToolRequest) (*gen.TrainingTargetCreate, string) {
	name, hasName, err := textArg(req, "name", maxTargetNameLen, true)
	if err != nil {
		return nil, err.Error()
	}
	if !hasName {
		return nil, "name is required (1–45 characters)"
	}
	date := req.GetString("date", "")
	if date == "" {
		return nil, "date is required (YYYY-MM-DD)"
	}
	clock := req.GetString("time", "18:00")
	// Targets send a tz-less local datetime; the server applies the account's
	// timezone. Shared assembly path with create_training_session (which sends
	// the offset form) — see convert.ParseLocalDateTime.
	when, err := convert.ParseLocalDateTime(date, clock)
	if err != nil {
		return nil, err.Error()
	}
	description, _, err := textArg(req, "description", maxTargetDescLen, false)
	if err != nil {
		return nil, err.Error()
	}

	et, phased, errMsg := buildGoalExerciseTarget(req)
	if errMsg != "" {
		return nil, errMsg
	}
	targetType := gen.TrainingTargetCreateTypeVOLUME
	if phased {
		targetType = gen.TrainingTargetCreateTypePHASED
	}

	body := &gen.TrainingTargetCreate{
		Type:            targetType,
		Name:            name,
		Datetime:        convert.WireDateTimeTZLess(when),
		ExerciseTargets: []gen.ExerciseTarget{et},
	}
	if description != "" {
		body.Description.SetTo(description)
	}
	return body, ""
}

// buildGoalExerciseTarget parses the goal arguments shared by training
// targets and favorites — sport_id plus either a phases array (PHASED) or one
// of duration_s / distance_m (VOLUME) — into a single wire ExerciseTarget.
// phased reports which shape was built.
func buildGoalExerciseTarget(req mcpgo.CallToolRequest) (et gen.ExerciseTarget, phased bool, errMsg string) {
	args, _ := req.GetArguments()["phases"].([]any)
	phases, err := buildPhases(args)
	if err != nil {
		return et, false, err.Error()
	}
	et = gen.ExerciseTarget{
		SportId: argInt(req, "sport_id", 1),
		Phases:  phases,
	}
	if len(phases) > 0 {
		return et, true, ""
	}
	// VOLUME target: requires exactly one of duration_s or distance_m.
	volDur, hasDur := goalDuration(req.GetArguments(), "duration_s")
	volDist, _ := req.GetArguments()["distance_m"].(float64)
	switch {
	case hasDur:
		et.Duration.SetTo(volDur)
	case volDist > 0:
		et.Distance.SetTo(volDist)
	default:
		return et, false, "VOLUME targets (no phases) require duration_s or distance_m"
	}
	return et, false, ""
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
		if err := checkPhaseNames(out[len(out)-1]); err != nil {
			return nil, fmt.Errorf("phases[%d] (%s): %w", i, ptype, err)
		}
	}
	return out, nil
}

// checkPhaseNames enforces Flow's phase-name limit on a built phase (and a
// repeat's nested work/recovery leaves).
func checkPhaseNames(p gen.Phase) error {
	leaves := []gen.PhaseLeaf{p.PhaseLeaf}
	if p.Type == gen.PhaseRepeatPhase {
		leaves = p.PhaseRepeat.Phases
	}
	for _, l := range leaves {
		if n := convert.PolarTextLen(l.Name); n > maxPhaseNameLen {
			return fmt.Errorf("phase name %q is %d characters; Polar's limit is %d", l.Name, n, maxPhaseNameLen)
		}
	}
	return nil
}

// phaseName returns a caller-supplied phase name (p["name"]) when present and
// non-empty, otherwise the type-derived fallback (e.g. "Warm-up", "Work"). The
// Polar Flow API persists this string verbatim per phase.
func phaseName(p map[string]any, fallback string) string {
	if name, ok := p["name"].(string); ok && name != "" {
		return name
	}
	return fallback
}

// buildSimpleLeaf constructs a duration-goal warm-up / cool-down leaf. Polar's
// PhaseLeaf carries intensityType/lowerZone/upperZone regardless of phase
// role — a bare leaf (no REPEAT wrapper) is the idiomatic way to model a
// zoned continuous block — so intensity is applied here exactly like it is on
// a repeat's work leaf (buildWorkLeaf). Defaults to open intensity (NONE)
// when the caller omits intensity, same as before.
func buildSimpleLeaf(p map[string]any, name, changeType string) (gen.PhaseLeaf, error) {
	dur, ok := goalDuration(p, "duration_s")
	if !ok {
		return gen.PhaseLeaf{}, fmt.Errorf("duration_s is required and must be > 0")
	}
	leaf := gen.PhaseLeaf{
		PhaseType:       "PHASE",
		Name:            phaseName(p, name),
		PhaseChangeType: gen.PhaseLeafPhaseChangeType(changeType),
		GoalType:        "DURATION",
		IntensityType:   "NONE",
	}
	leaf.Duration.SetTo(dur)
	if intensity, ok := p["intensity"].(map[string]any); ok {
		applyIntensity(&leaf, intensity)
	}
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
				Name:            phaseName(rec, "Recovery"),
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
		Name:            phaseName(intensityCtx, "Work"),
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

// applyIntensity sets a phase leaf's intensityType and lowerZone/upperZone from
// the caller's intensity object. All three zone metrics (HR, power, speed) share
// the same wire representation — a 1-5 zone index in lowerZone/upperZone, not a
// raw bpm/watts/km-h threshold (see PhaseLeaf in the vendored spec) — so each
// case below just picks the discriminator (intensityType) and copies the zone
// number(s) through. Precedence when multiple keys are set: hr_zone, then
// power_zone, then speed_zone, then label — matching the pre-existing "hr_zone
// wins over label" rule extended to the two new metrics.
func applyIntensity(leaf *gen.PhaseLeaf, intensity map[string]any) {
	if zone, ok := intensity["hr_zone"].(float64); ok && zone >= 1 && zone <= 5 {
		leaf.IntensityType = "HEART_RATE_ZONES"
		leaf.LowerZone.SetTo(zone)
		leaf.UpperZone.SetTo(zone)
		return
	}
	if zone, ok := intensity["power_zone"].(float64); ok && zone >= 1 && zone <= 5 {
		leaf.IntensityType = "POWER_ZONES"
		leaf.LowerZone.SetTo(zone)
		leaf.UpperZone.SetTo(zone)
		return
	}
	if zone, ok := intensity["speed_zone"].(float64); ok && zone >= 1 && zone <= 5 {
		leaf.IntensityType = "SPEED_ZONES"
		leaf.LowerZone.SetTo(zone)
		leaf.UpperZone.SetTo(zone)
		return
	}
	if label, ok := intensity["label"].(string); ok {
		if lo, hi, ok := convert.HRZoneForLabel(label); ok {
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

// goalDuration extracts an int seconds value from the args map and formats it
// as the HH:MM:SS string the training-target endpoints want.
func goalDuration(m map[string]any, key string) (string, bool) {
	v, ok := m[key].(float64)
	if !ok || v <= 0 {
		return "", false
	}
	return convert.SecondsToClock(int(v)), true
}
