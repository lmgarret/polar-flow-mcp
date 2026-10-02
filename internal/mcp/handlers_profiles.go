package mcp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// Sport-profile writes. A profile is keyed by sport (its uuid is derived from
// the sport id — flow.SportProfileUUID), so every tool takes sport_id. Flow
// validates zone edits server-side but answers with one long plain-text 400;
// the rules are checked here first (see convert's zone-edit limits).

var (
	hrSources    = convert.ZoneSources{Free: flow.HRSourceFree, Default: flow.HRSourceDefault}
	speedSources = convert.ZoneSources{Free: flow.SpeedSourceFree, Default: flow.SpeedSourceDefault}
	powerSources = convert.ZoneSources{Free: flow.PowerSourceFree, Default: flow.PowerSourceDefault}
)

// sportLabel names a sport for messages: "RUNNING (sport_id 1)".
func sportLabel(ctx context.Context, fc *flow.Client, id int) string {
	if name := sportName(ctx, fc, id); name != "" {
		return fmt.Sprintf("%s (sport_id %d)", name, id)
	}
	return fmt.Sprintf("sport_id %d", id)
}

// sportIDArg reads the required sport_id and checks it against the catalogue.
func sportIDArg(ctx context.Context, fc *flow.Client, req mcpgo.CallToolRequest) (int, error) {
	id, present, err := intArgRange(req, "sport_id", 1, 100000, "")
	if err != nil {
		return 0, err
	}
	if !present {
		return 0, errors.New("sport_id is required (list_sports has the ids)")
	}
	if err := checkSport(ctx, fc, int(id)); err != nil {
		return 0, err
	}
	return int(id), nil
}

// zonesResult renders one sport's zones for the zones UI, with a leading
// sentence saying what happened.
func zonesResult(notice string, z convert.TrainingZones) *mcpgo.CallToolResult {
	zones := []convert.TrainingZones{z}
	return widgetResultText(notice+"\n\n"+zonesText(zones),
		map[string]any{"type": "training_zones", "notice": notice, "sports": zones})
}

// CreateSportProfileHandler creates a sport's profile with Polar's defaults.
func CreateSportProfileHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		sportID, err := sportIDArg(ctx, fc, req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		p, created, err := fc.CreateSportProfile(ctx, sportID)
		if errors.Is(err, flow.ErrUnknownSport) {
			return mcpgo.NewToolResultError(fmt.Sprintf("Polar refused sport_id %d.", sportID)), nil
		}
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		label := sportLabel(ctx, fc, sportID)
		notice := fmt.Sprintf("Created the %s sport profile with Polar's default zones. "+
			"A paired watch picks it up on its next sync.", label)
		if !created {
			notice = fmt.Sprintf("A %s sport profile already exists — nothing was changed.", label)
		}
		return zonesResult(notice, convert.FromWireTrainingZones(p, sportName(ctx, fc, sportID), convert.ZonesFromProfile)), nil
	}
}

// zoneEditArgs reads the heart_rate_bpm / speed_kmh / power_w / reset
// arguments of update_training_zones and validates them against Flow's rules.
func zoneEditArgs(req mcpgo.CallToolRequest) (hr, speed, power *convert.ZoneEdit, err error) {
	hrB, hrOK, err := boundsArg(req, "heart_rate_bpm", 6, 6, convert.MinHRBPM, convert.MaxHRBPM, true, convert.MinZoneSpanBPM, "bpm")
	if err != nil {
		return nil, nil, nil, err
	}
	spB, spOK, err := boundsArg(req, "speed_kmh", 5, 6, convert.MinSpeedKmh, convert.MaxSpeedKmh, false, 0, "km/h")
	if err != nil {
		return nil, nil, nil, err
	}
	pwB, pwOK, err := boundsArg(req, "power_w", 5, 6, 0, convert.MaxPowerW, true, convert.MinZoneSpanBPM, "W")
	if err != nil {
		return nil, nil, nil, err
	}
	resets, err := resetArg(req)
	if err != nil {
		return nil, nil, nil, err
	}
	pick := func(name string, bounds []float64, set bool) (*convert.ZoneEdit, error) {
		switch {
		case set && resets[name]:
			return nil, fmt.Errorf("%s is both set and listed in reset — pick one", zoneArgName[name])
		case set:
			return &convert.ZoneEdit{Bounds: bounds}, nil
		case resets[name]:
			return &convert.ZoneEdit{Reset: true}, nil
		}
		return nil, nil
	}
	if hr, err = pick("heart_rate", hrB, hrOK); err != nil {
		return nil, nil, nil, err
	}
	if speed, err = pick("speed", spB, spOK); err != nil {
		return nil, nil, nil, err
	}
	if power, err = pick("power", pwB, pwOK); err != nil {
		return nil, nil, nil, err
	}
	if hr == nil && speed == nil && power == nil {
		return nil, nil, nil, errors.New("nothing to change: set heart_rate_bpm, speed_kmh or power_w, or list zone types in reset")
	}
	return hr, speed, power, nil
}

var zoneArgName = map[string]string{"heart_rate": "heart_rate_bpm", "speed": "speed_kmh", "power": "power_w"}

// boundsArg reads an optional list of zone boundaries: minLen–maxLen strictly
// ascending numbers in [lo, hi], optionally whole, each step ≥ minStep.
func boundsArg(req mcpgo.CallToolRequest, name string, minLen, maxLen int, lo, hi float64, whole bool, minStep float64, unit string) ([]float64, bool, error) {
	raw, ok := rawArg(req, name)
	if !ok {
		return nil, false, nil
	}
	list, isList := raw.([]any)
	if !isList {
		return nil, true, fmt.Errorf("%s must be an array of numbers, got %s", name, jsonTypeName(raw))
	}
	if len(list) < minLen || len(list) > maxLen {
		want := fmt.Sprintf("%d", minLen)
		if maxLen != minLen {
			want = fmt.Sprintf("%d or %d", minLen, maxLen)
		}
		return nil, true, fmt.Errorf("%s must have %s values (zone boundaries, Z1 lower first), got %d", name, want, len(list))
	}
	out := make([]float64, len(list))
	for i, v := range list {
		f, err := boundAt(name, i, v, lo, hi, whole, unit)
		if err != nil {
			return nil, true, err
		}
		if i > 0 && f <= out[i-1] {
			return nil, true, fmt.Errorf("%s must be strictly ascending: %v follows %v", name, f, out[i-1])
		}
		if i > 0 && f-out[i-1] < minStep {
			return nil, true, fmt.Errorf("%s: every zone must span at least %v %s (Z%d is %v–%v)", name, minStep, unit, i, out[i-1], f)
		}
		out[i] = f
	}
	return out, true, nil
}

// boundAt checks one boundary value: a finite number in [lo, hi], whole when
// required.
func boundAt(name string, i int, v any, lo, hi float64, whole bool, unit string) (float64, error) {
	f, isNum := v.(float64)
	if !isNum || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%s[%d] must be a number, got %s", name, i, jsonTypeName(v))
	}
	if whole && f != math.Trunc(f) {
		return 0, fmt.Errorf("%s[%d] must be a whole number, got %v", name, i, f)
	}
	if f < lo || f > hi {
		return 0, fmt.Errorf("%s[%d] must be between %v and %v %s, got %v", name, i, lo, hi, unit, f)
	}
	return f, nil
}

// resetArg reads the optional reset list of zone types.
func resetArg(req mcpgo.CallToolRequest) (map[string]bool, error) {
	out := map[string]bool{}
	raw, ok := rawArg(req, "reset")
	if !ok {
		return out, nil
	}
	list, isList := raw.([]any)
	if !isList {
		return nil, fmt.Errorf("reset must be an array of zone types, got %s", jsonTypeName(raw))
	}
	for _, v := range list {
		s, _ := v.(string)
		if _, known := zoneArgName[s]; !known {
			return nil, fmt.Errorf("reset entries must be \"heart_rate\", \"speed\" or \"power\", got %v", v)
		}
		out[s] = true
	}
	return out, nil
}

// UpdateTrainingZonesHandler saves hand-entered zones (or resets them to
// Polar's defaults) on a stored sport profile.
func UpdateTrainingZonesHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		hr, speed, power, err := zoneEditArgs(req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		sportID, err := sportIDArg(ctx, fc, req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		label := sportLabel(ctx, fc, sportID)
		p, ok, err := fc.GetSportProfile(ctx, flow.SportProfileUUID(sportID))
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if !ok {
			return mcpgo.NewToolResultError(fmt.Sprintf("No %s sport profile is stored, so it has no zones to edit. "+
				"Call create_sport_profile first (it starts from Polar's defaults), then retry.", label)), nil
		}
		settings, _ := p.Profile.Settings.Get()
		stored, _ := settings.ZoneLimits.Get()
		if speed != nil && !speed.Reset && len(stored.SpeedZones) == 0 {
			return mcpgo.NewToolResultError(fmt.Sprintf("%s has no speed zones (Polar only keeps heart-rate zones for it).", label)), nil
		}
		if power != nil && !power.Reset && len(stored.PowerZones) == 0 {
			return mcpgo.NewToolResultError(fmt.Sprintf("%s has no power zones (Polar only keeps heart-rate zones for it).", label)), nil
		}
		// Flow requires the caller's own user id in the body.
		ui, err := fc.GetUserInfo(ctx)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		zl := convert.ApplyZoneEdits(stored, hr, speed, power, hrSources, speedSources, powerSources)
		if err := fc.UpdateSportProfileZones(ctx, ui.ID, p, zl); err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		z, ok, err := storedZones(ctx, fc, p.UUID, sportID)
		if err != nil || !ok {
			return mcpgo.NewToolResultText(fmt.Sprintf("Saved the %s zones, but reading them back failed: %v", label, err)), nil
		}
		notice := fmt.Sprintf("Saved the %s zones (%s). A paired watch picks them up on its next sync.", label, editSummary(hr, speed, power))
		return zonesResult(notice, z), nil
	}
}

// editSummary lists what an update changed: "heart rate set, power reset".
func editSummary(hr, speed, power *convert.ZoneEdit) string {
	var parts []string
	for _, e := range []struct {
		name string
		edit *convert.ZoneEdit
	}{{"heart rate", hr}, {"speed", speed}, {"power", power}} {
		switch {
		case e.edit == nil:
		case e.edit.Reset:
			parts = append(parts, e.name+" reset to Polar's defaults")
		default:
			parts = append(parts, e.name+" set")
		}
	}
	return strings.Join(parts, ", ")
}

// DeleteSportProfileHandler deletes a sport's profile, after confirmation.
func DeleteSportProfileHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		sportID, err := sportIDArg(ctx, fc, req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		label := sportLabel(ctx, fc, sportID)
		id := flow.SportProfileUUID(sportID)
		// Flow answers 200 for a profile that does not exist; check first.
		if _, ok, err := fc.GetSportProfile(ctx, id); err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		} else if !ok {
			return mcpgo.NewToolResultText(fmt.Sprintf("No %s sport profile is stored — nothing to delete.", label)), nil
		}
		if result, proceed := requireConfirm(ctx, req, "delete_sport_profile", func() string {
			return fmt.Sprintf("Delete the %s sport profile? Its training zones and watch settings for this "+
				"sport are removed (a paired watch drops them on its next sync).", label)
		}); !proceed {
			return result, nil
		}
		if err := fc.DeleteSportProfile(ctx, id); err != nil {
			if errors.Is(err, flow.ErrLastSportProfile) {
				return mcpgo.NewToolResultError(fmt.Sprintf("Polar refuses to delete the %s profile: it is the "+
					"account's last sport profile.", label)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		result := mcpgo.NewToolResultText(fmt.Sprintf("Deleted the %s sport profile.", label))
		result.StructuredContent = map[string]any{"type": "sport_profile_deleted", "data": map[string]any{"sport_id": sportID}}
		return result, nil
	}
}
