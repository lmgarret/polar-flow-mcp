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

	ets, kind, err := buildExerciseTargets(req.GetArguments(), true)
	if err != nil {
		return nil, err.Error()
	}

	body := &gen.TrainingTargetCreate{
		Type:            gen.TrainingTargetCreateType(kind),
		Name:            name,
		Datetime:        convert.WireDateTimeTZLess(when),
		ExerciseTargets: ets,
	}
	if description != "" {
		body.Description.SetTo(description)
	}
	return body, ""
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
