package convert

import (
	"math"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Zone sentinels: the top speed and power zones have no ceiling, which Flow
// encodes as its validation maximum (399 km/h, 2000 W). They map to null.
const (
	openSpeedKmh = 399.0
	openPowerW   = 2000.0
)

// Zone sources reported on TrainingZones.Source.
const (
	ZonesFromProfile = "profile" // read from the sport profile stored on the account
	ZonesFromDefault = "default" // computed by Flow; no profile stored for the sport
)

// TrainingZones is the canonical zone set of one sport: what hr_zone /
// speed_zone / power_zone 1–5 on a training-target phase mean in bpm, km/h and
// watts. Every zone is the half-open range [min, max); zones are contiguous.
// A zone type the sport does not support (e.g. power for swimming) is omitted.
type TrainingZones struct {
	SportID       int             `json:"sport_id"`
	SportName     string          `json:"sport_name,omitempty"`
	SportCategory string          `json:"sport_category,omitempty"`
	Source        string          `json:"source"`
	HeartRate     *HRZoneSet      `json:"heart_rate,omitempty"`
	Speed         *SpeedZoneSet   `json:"speed,omitempty"`
	Power         *PowerZoneSet   `json:"power,omitempty"`
	Thresholds    *ZoneThresholds `json:"thresholds,omitempty"`
}

// HRZoneSet holds the five heart-rate zones. Setting is "default" (derived
// from max HR) or "free" (entered by hand).
type HRZoneSet struct {
	Setting string   `json:"setting,omitempty"`
	Zones   []HRZone `json:"zones"`
}

// HRZone is one heart-rate zone in bpm.
type HRZone struct {
	Zone   int `json:"zone"`
	MinBPM int `json:"min_bpm"`
	MaxBPM int `json:"max_bpm"`
}

// SpeedZoneSet holds the five speed zones. Method says how default zones were
// computed ("mas_based" = % of maximum aerobic speed, "sport_specific_predefined"
// = a fixed per-sport table). SpeedView is "pace" or "speed" — how the watch
// shows it — when the stored profile says so.
type SpeedZoneSet struct {
	Setting   string      `json:"setting,omitempty"`
	Method    string      `json:"method,omitempty"`
	SpeedView string      `json:"speed_view,omitempty"`
	Zones     []SpeedZone `json:"zones"`
}

// SpeedZone is one speed zone in km/h, with the matching pace range in seconds
// per km. MaxKmh and FastestPace are null on the open-ended top zone.
type SpeedZone struct {
	Zone              int      `json:"zone"`
	MinKmh            float64  `json:"min_kmh"`
	MaxKmh            *float64 `json:"max_kmh"`
	SlowestPaceSPerKm *int     `json:"slowest_pace_s_per_km"`
	FastestPaceSPerKm *int     `json:"fastest_pace_s_per_km"`
}

// PowerZoneSet holds the five power zones. Method is "ftp_based" or
// "map_based" (% of maximum aerobic power) for default zones.
type PowerZoneSet struct {
	Setting string      `json:"setting,omitempty"`
	Method  string      `json:"method,omitempty"`
	Zones   []PowerZone `json:"zones"`
}

// PowerZone is one power zone in watts. MaxW is null on the open-ended top zone.
type PowerZone struct {
	Zone int  `json:"zone"`
	MinW int  `json:"min_w"`
	MaxW *int `json:"max_w"`
}

// ZoneThresholds are the per-sport thresholds default zones derive from, each
// with its source ("estimated" when Flow estimated it from physical info).
type ZoneThresholds struct {
	MASKmh    *float64 `json:"mas_kmh,omitempty"`
	MASSource string   `json:"mas_source,omitempty"`
	MAPW      *int     `json:"map_w,omitempty"`
	MAPSource string   `json:"map_source,omitempty"`
	FTPW      *int     `json:"ftp_w,omitempty"`
	FTPSource string   `json:"ftp_source,omitempty"`
}

// FromWireTrainingZones maps a sport profile (stored, or the recalculate
// output) to TrainingZones. sportName may be "" when unknown; source is
// ZonesFromProfile or ZonesFromDefault.
func FromWireTrainingZones(p *gen.SportProfile, sportName, source string) TrainingZones {
	body := p.Profile
	out := TrainingZones{
		SportID:       body.SportId,
		SportName:     sportName,
		SportCategory: SportCategory(sportName, body.SportId),
		Source:        source,
	}
	settings, _ := body.Settings.Get()
	if zl, ok := settings.ZoneLimits.Get(); ok {
		out.HeartRate = hrZoneSet(zl)
		out.Speed = speedZoneSet(zl, settings.SpeedView.Or(""))
		out.Power = powerZoneSet(zl)
	}
	out.Thresholds = zoneThresholds(body)
	return out
}

func hrZoneSet(zl gen.SportProfileZoneLimits) *HRZoneSet {
	if len(zl.HeartRateZones) == 0 {
		return nil
	}
	set := &HRZoneSet{Setting: PolarEnumTail(zl.HeartRateSettingSource.Or(""), "_SOURCE_")}
	for i, z := range zl.HeartRateZones {
		set.Zones = append(set.Zones, HRZone{Zone: i + 1, MinBPM: roundInt(z.LowerLimit), MaxBPM: roundInt(z.HigherLimit)})
	}
	return set
}

func speedZoneSet(zl gen.SportProfileZoneLimits, speedView string) *SpeedZoneSet {
	if len(zl.SpeedZones) == 0 {
		return nil
	}
	set := &SpeedZoneSet{
		Setting: PolarEnumTail(zl.SpeedSettingSource.Or(""), "_SOURCE_"),
		Method:  PolarEnumTail(zl.SpeedZoneCalculationMethod.Or(""), "_METHOD_"),
	}
	switch speedView {
	case "SPEED_VIEW_PACE":
		set.SpeedView = "pace"
	case "SPEED_VIEW_SPEED":
		set.SpeedView = "speed"
	}
	last := len(zl.SpeedZones) - 1
	for i, z := range zl.SpeedZones {
		sz := SpeedZone{Zone: i + 1, MinKmh: round2(z.LowerLimit)}
		if p, ok := KmhToPaceSeconds(z.LowerLimit); ok {
			sz.SlowestPaceSPerKm = &p
		}
		if i != last || z.HigherLimit < openSpeedKmh {
			hi := round2(z.HigherLimit)
			sz.MaxKmh = &hi
			if p, ok := KmhToPaceSeconds(z.HigherLimit); ok {
				sz.FastestPaceSPerKm = &p
			}
		}
		set.Zones = append(set.Zones, sz)
	}
	return set
}

func powerZoneSet(zl gen.SportProfileZoneLimits) *PowerZoneSet {
	if len(zl.PowerZones) == 0 {
		return nil
	}
	set := &PowerZoneSet{
		Setting: PolarEnumTail(zl.PowerSettingSource.Or(""), "_SOURCE_"),
		Method:  PolarEnumTail(zl.PowerZoneCalculationMethod.Or(""), "_METHOD_"),
	}
	last := len(zl.PowerZones) - 1
	for i, z := range zl.PowerZones {
		pz := PowerZone{Zone: i + 1, MinW: roundInt(z.LowerLimit)}
		if i != last || z.HigherLimit < openPowerW {
			hi := roundInt(z.HigherLimit)
			pz.MaxW = &hi
		}
		set.Zones = append(set.Zones, pz)
	}
	return set
}

func zoneThresholds(body gen.SportProfileBody) *ZoneThresholds {
	var th ZoneThresholds
	if t, ok := body.MaximumAerobicSpeed.Get(); ok {
		if v, ok := t.Speed.Get(); ok {
			v = round2(v)
			th.MASKmh, th.MASSource = &v, PolarEnumTail(t.Source.Or(""), "_SOURCE_")
		}
	}
	if t, ok := body.MaximumAerobicPower.Get(); ok {
		if v, ok := t.Power.Get(); ok {
			th.MAPW, th.MAPSource = &v, PolarEnumTail(t.Source.Or(""), "_SOURCE_")
		}
	}
	if t, ok := body.FunctionalThresholdPower.Get(); ok {
		if v, ok := t.Power.Get(); ok {
			th.FTPW, th.FTPSource = &v, PolarEnumTail(t.Source.Or(""), "_SOURCE_")
		}
	}
	if th == (ZoneThresholds{}) {
		return nil
	}
	return &th
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func roundInt(v float64) int { return int(math.Round(v)) }
