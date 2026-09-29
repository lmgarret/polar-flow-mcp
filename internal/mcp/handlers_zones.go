package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// GetTrainingZonesHandler returns the heart-rate / speed / power zones for one
// sport (sport_id) or for every sport profile stored on the account.
//
// A stored profile is read as-is (it may carry hand-entered "free" zones). For
// a sport with no stored profile, Flow's recalculate endpoint computes the
// defaults without persisting anything — the zones the watch would use once a
// profile is created.
func GetTrainingZonesHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		sportID, onlyOne, err := intArgRange(req, "sport_id", 1, 100000, "")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if onlyOne {
			if err := checkSport(ctx, fc, int(sportID)); err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
		}
		profiles, err := fc.ListSportProfiles(ctx)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		var zones []convert.TrainingZones
		for _, p := range profiles {
			if onlyOne && p.Profile.SportId != int(sportID) {
				continue
			}
			z, ok, err := storedZones(ctx, fc, p.UUID, p.Profile.SportId)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			if ok {
				zones = append(zones, z)
			}
		}
		if onlyOne && len(zones) == 0 {
			z, err := defaultZones(ctx, fc, int(sportID))
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			zones = append(zones, z)
		}
		if zones == nil {
			zones = []convert.TrainingZones{}
		}
		return widgetResultText(zonesText(zones), map[string]any{"type": "training_zones", "sports": zones}), nil
	}
}

// storedZones reads one stored profile's zones. ok=false when the profile was
// deleted between the list and the read.
func storedZones(ctx context.Context, fc *flow.Client, id string, sportID int) (convert.TrainingZones, bool, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return convert.TrainingZones{}, false, fmt.Errorf("sport profile for sport %d has an unexpected id %q", sportID, id)
	}
	p, ok, err := fc.GetSportProfile(ctx, u)
	if err != nil || !ok {
		return convert.TrainingZones{}, false, err
	}
	return convert.FromWireTrainingZones(p, sportName(ctx, fc, p.Profile.SportId), convert.ZonesFromProfile), true, nil
}

func defaultZones(ctx context.Context, fc *flow.Client, sportID int) (convert.TrainingZones, error) {
	// Flow requires the caller's own user id in the body.
	ui, err := fc.GetUserInfo(ctx)
	if err != nil {
		return convert.TrainingZones{}, err
	}
	p, err := fc.DefaultSportZones(ctx, ui.ID, sportID)
	if err != nil {
		return convert.TrainingZones{}, err
	}
	return convert.FromWireTrainingZones(p, sportName(ctx, fc, sportID), convert.ZonesFromDefault), nil
}

// sportName resolves a sport id through the cached catalogue; "" if unknown.
func sportName(ctx context.Context, fc *flow.Client, id int) string {
	name, _, _ := fc.SportName(ctx, id)
	return name
}

// zonesText renders the zones as compact lines for the model.
func zonesText(zones []convert.TrainingZones) string {
	if len(zones) == 0 {
		return "No sport profiles are stored on this account. Pass sport_id to get the " +
			"default zones Polar computes for that sport."
	}
	var b strings.Builder
	for i, z := range zones {
		if i > 0 {
			b.WriteString("\n")
		}
		name := z.SportName
		if name == "" {
			name = fmt.Sprintf("sport %d", z.SportID)
		}
		origin := "stored sport profile"
		if z.Source == convert.ZonesFromDefault {
			origin = "Polar defaults — no profile stored for this sport"
		}
		fmt.Fprintf(&b, "%s (sport_id %d, %s):\n", name, z.SportID, origin)
		if z.HeartRate != nil {
			fmt.Fprintf(&b, "  Heart rate (bpm%s): %s\n", setting(z.HeartRate.Setting), hrText(z.HeartRate))
		}
		if z.Speed != nil {
			fmt.Fprintf(&b, "  Speed (km/h%s): %s\n", setting(z.Speed.Setting), speedText(z.Speed))
		}
		if z.Power != nil {
			fmt.Fprintf(&b, "  Power (W%s): %s\n", setting(z.Power.Setting), powerText(z.Power))
		}
		if th := thresholdsText(z.Thresholds); th != "" {
			fmt.Fprintf(&b, "  Thresholds: %s\n", th)
		}
	}
	return b.String()
}

func hrText(set *convert.HRZoneSet) string {
	parts := make([]string, len(set.Zones))
	for i, z := range set.Zones {
		parts[i] = fmt.Sprintf("Z%d %d–%d", z.Zone, z.MinBPM, z.MaxBPM)
	}
	return strings.Join(parts, " · ")
}

func speedText(set *convert.SpeedZoneSet) string {
	parts := make([]string, len(set.Zones))
	for i, z := range set.Zones {
		if z.MaxKmh == nil {
			parts[i] = fmt.Sprintf("Z%d ≥%g", z.Zone, z.MinKmh)
		} else {
			parts[i] = fmt.Sprintf("Z%d %g–%g", z.Zone, z.MinKmh, *z.MaxKmh)
		}
		if pace := paceRange(z); pace != "" && showPace(set) {
			parts[i] += " (" + pace + ")"
		}
	}
	return strings.Join(parts, " · ")
}

func powerText(set *convert.PowerZoneSet) string {
	parts := make([]string, len(set.Zones))
	for i, z := range set.Zones {
		if z.MaxW == nil {
			parts[i] = fmt.Sprintf("Z%d ≥%d", z.Zone, z.MinW)
		} else {
			parts[i] = fmt.Sprintf("Z%d %d–%d", z.Zone, z.MinW, *z.MaxW)
		}
	}
	return strings.Join(parts, " · ")
}

func thresholdsText(t *convert.ZoneThresholds) string {
	if t == nil {
		return ""
	}
	var th []string
	if t.MASKmh != nil {
		th = append(th, fmt.Sprintf("MAS %g km/h%s", *t.MASKmh, paren(t.MASSource)))
	}
	if t.MAPW != nil {
		th = append(th, fmt.Sprintf("MAP %d W%s", *t.MAPW, paren(t.MAPSource)))
	}
	if t.FTPW != nil {
		th = append(th, fmt.Sprintf("FTP %d W%s", *t.FTPW, paren(t.FTPSource)))
	}
	return strings.Join(th, ", ")
}

func setting(s string) string {
	if s == "" {
		return ""
	}
	return ", " + s
}

func paren(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

// showPace reports whether a sport reads its speed as pace: the stored
// profile's speed view, else MAS-based defaults (running-type sports only).
// The payload always carries both; this only trims the model text.
func showPace(set *convert.SpeedZoneSet) bool {
	if set.SpeedView != "" {
		return set.SpeedView == "pace"
	}
	return set.Method == "mas_based"
}

// paceRange formats a speed zone as a min/km pace range, slow end first.
func paceRange(z convert.SpeedZone) string {
	switch {
	case z.SlowestPaceSPerKm == nil:
		return ""
	case z.FastestPaceSPerKm == nil:
		return "faster than " + convert.PaceClock(*z.SlowestPaceSPerKm) + "/km"
	default:
		return convert.PaceClock(*z.SlowestPaceSPerKm) + "–" + convert.PaceClock(*z.FastestPaceSPerKm) + "/km"
	}
}
