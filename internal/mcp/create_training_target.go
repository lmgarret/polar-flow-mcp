package mcp

import (
	"context"
	"errors"
	"fmt"
	"math"

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
// PHASE/REPEAT sum-type list. Warmup, block and cooldown become single
// PhaseLeaf entries; repeat becomes a PhaseRepeat with nested
// PhaseLeaf{work, recovery}.
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
		case "warmup", "block", "cooldown":
			leaf, err := buildSingleLeaf(p, singlePhaseNames[ptype])
			if err != nil {
				return nil, fmt.Errorf("phases[%d] (%s): %w", i, ptype, err)
			}
			out = append(out, gen.Phase{Type: gen.PhaseLeafPhase, PhaseLeaf: leaf})
		case "repeat":
			repeat, err := buildRepeat(p)
			if err != nil {
				return nil, fmt.Errorf("phases[%d] (repeat): %w", i, err)
			}
			out = append(out, gen.Phase{Type: gen.PhaseRepeatPhase, PhaseRepeat: repeat})
		default:
			return nil, fmt.Errorf("phases[%d]: unknown type %q (want warmup|block|repeat|cooldown)", i, ptype)
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

// singlePhaseNames are the default names of the single-leaf phase types. Flow
// gives a leaf no role — warm-up / block / cool-down differ only in this
// fallback label — so any of them may sit anywhere in the sequence.
var singlePhaseNames = map[string]string{"warmup": "Warm-up", "block": "Block", "cooldown": "Cool-down"}

// buildSingleLeaf constructs a bare (non-repeated) leaf with a duration_s or
// distance_m goal. A bare leaf is Flow's idiomatic continuous block, so a
// sequence of them models e.g. a race by km segments ("5 km Z3, 10 km Z1–Z3,
// 6.1 km Z3–Z4" — captured from the web UI 2026-10-13). Intensity is applied
// exactly like on a repeat's work leaf (buildWorkLeaf); it defaults to open
// (NONE) when omitted.
func buildSingleLeaf(p map[string]any, name string) (gen.PhaseLeaf, error) {
	leaf, err := goalLeaf(p, phaseName(p, name))
	if err != nil {
		return leaf, err
	}
	if intensity, ok := p["intensity"].(map[string]any); ok {
		if err := applyIntensity(&leaf, intensity); err != nil {
			return leaf, err
		}
	}
	return leaf, nil
}

// goalLeaf builds an AUTOMATIC, open-intensity leaf from a goal object
// holding exactly one of distance_m (metres) or duration_s (seconds). The
// goal decides goalType, which is what Flow (and our read-back) keys on.
func goalLeaf(goal map[string]any, name string) (gen.PhaseLeaf, error) {
	leaf := gen.PhaseLeaf{
		PhaseType:       "PHASE",
		Name:            name,
		PhaseChangeType: "AUTOMATIC",
		IntensityType:   "NONE",
	}
	_, hasDist := goal["distance_m"]
	_, hasDur := goal["duration_s"]
	switch {
	case hasDist && hasDur:
		return leaf, fmt.Errorf("give only one of distance_m or duration_s")
	case hasDist:
		v, ok := goal["distance_m"].(float64)
		if !ok || int(v) <= 0 {
			return leaf, fmt.Errorf("distance_m must be a number of metres > 0")
		}
		leaf.GoalType = "DISTANCE"
		leaf.Distance.SetTo(float64(int(v)))
	case hasDur:
		dur, ok := goalDuration(goal, "duration_s")
		if !ok {
			return leaf, fmt.Errorf("duration_s must be a number of seconds > 0")
		}
		leaf.GoalType = "DURATION"
		leaf.Duration.SetTo(dur)
	default:
		return leaf, fmt.Errorf("needs a goal: distance_m or duration_s")
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
	if goal == nil {
		return gen.PhaseLeaf{}, fmt.Errorf("goal must specify distance_m or duration_s")
	}
	leaf, err := goalLeaf(goal, phaseName(intensityCtx, "Work"))
	if err != nil {
		return leaf, fmt.Errorf("goal: %w", err)
	}
	if intensity, ok := intensityCtx["intensity"].(map[string]any); ok {
		if err := applyIntensity(&leaf, intensity); err != nil {
			return leaf, err
		}
	}
	return leaf, nil
}

// zoneKeys are the numeric intensity keys in precedence order, with the
// intensityType each one selects.
var zoneKeys = []struct{ key, intensityType string }{
	{"hr_zone", "HEART_RATE_ZONES"},
	{"power_zone", "POWER_ZONES"},
	{"speed_zone", "SPEED_ZONES"},
}

// applyIntensity sets a phase leaf's intensityType and lowerZone/upperZone from
// the caller's intensity object. All three zone metrics (HR, power, speed) share
// the same wire representation — a 1-5 zone index in lowerZone/upperZone, not a
// raw bpm/watts/km-h threshold (see PhaseLeaf in the vendored spec) — so each
// case just picks the discriminator (intensityType) and copies the zone
// number(s) through. Precedence when multiple keys are set: hr_zone, then
// power_zone, then speed_zone, then label.
//
// The zone is the range's lower bound; zone_max (optional) is its upper bound,
// so {hr_zone: 1, zone_max: 3} is "Z1–Z3", as the web UI sends when its two
// zone pickers differ. Flow 400s zones outside 1–5 or fractional, but stores an
// inverted range as sent — hence the checks here.
func applyIntensity(leaf *gen.PhaseLeaf, intensity map[string]any) error {
	zoneMax, hasMax, err := zoneValue(intensity, "zone_max")
	if err != nil {
		return err
	}
	for _, z := range zoneKeys {
		zone, ok, err := zoneValue(intensity, z.key)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		upper := zone
		if hasMax {
			if zoneMax < zone {
				return fmt.Errorf("intensity.zone_max (%d) must be ≥ %s (%d)", zoneMax, z.key, zone)
			}
			upper = zoneMax
		}
		leaf.IntensityType = gen.PhaseLeafIntensityType(z.intensityType)
		leaf.LowerZone.SetTo(float64(zone))
		leaf.UpperZone.SetTo(float64(upper))
		return nil
	}
	if hasMax {
		return fmt.Errorf("intensity.zone_max needs hr_zone, power_zone or speed_zone as the lower bound")
	}
	if label, ok := intensity["label"].(string); ok {
		if lo, hi, ok := convert.HRZoneForLabel(label); ok {
			leaf.IntensityType = "HEART_RATE_ZONES"
			leaf.LowerZone.SetTo(float64(lo))
			leaf.UpperZone.SetTo(float64(hi))
		}
	}
	return nil
}

// zoneValue reads an optional intensity zone: a whole number 1–5.
func zoneValue(intensity map[string]any, key string) (int, bool, error) {
	raw, ok := intensity[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	f, isNum := raw.(float64)
	if !isNum || f != math.Trunc(f) || f < 1 || f > 5 {
		return 0, true, fmt.Errorf("intensity.%s must be a whole zone number 1–5, got %v", key, raw)
	}
	return int(f), true, nil
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
