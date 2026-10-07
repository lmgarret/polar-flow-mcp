package convert

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/jx"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// SessionDetails is the compact, canonical shape of get_training_session_details,
// mapped from GET /api/training/analysis/{id}/details. That response is ~500 KB
// for a one-hour run, ~95% of it per-second sample series; this view keeps the
// headline figures, time in zones, the per-repetition splits of a planned
// target, laps and hill segments, and only names the sample series —
// get_training_session_samples serves those, resampled.
type SessionDetails struct {
	ID        int64            `json:"id"`
	Exercises []ExerciseDetail `json:"exercises"`
}

// ExerciseDetail is one exercise (one sport) of a session. Offsets (start_s,
// end_s) count seconds from the exercise start.
type ExerciseDetail struct {
	ExerciseID    int64    `json:"exercise_id"`
	SportID       *int     `json:"sport_id"`
	SportCategory string   `json:"sport_category,omitempty"`
	StartTime     string   `json:"start_time,omitempty"`
	DurationS     *int     `json:"duration_s"`
	DistanceM     *float64 `json:"distance_m"`
	Calories      *int     `json:"calories"`
	AscentM       *float64 `json:"ascent_m,omitempty"`
	DescentM      *float64 `json:"descent_m,omitempty"`
	RunningIndex  *int     `json:"running_index,omitempty"`
	RecoveryTimeS *int     `json:"recovery_time_s,omitempty"`
	HR            *Stat    `json:"hr_bpm,omitempty"`
	Speed         *Stat    `json:"speed_kmh,omitempty"`
	Power         *Stat    `json:"power_w,omitempty"`
	Cadence       *Stat    `json:"cadence,omitempty"`
	Zones         *Zones   `json:"zones,omitempty"`
	Target        *Target  `json:"target,omitempty"`
	Laps          *Laps    `json:"laps,omitempty"`
	Hills         []Hill   `json:"hills,omitempty"`
	// Samples names the recorded series get_training_session_samples can
	// return for this exercise (its `metrics` values); empty for a manual entry.
	Samples []string `json:"samples"`
}

// Stat is the average / maximum / minimum of one metric; absent values are omitted.
type Stat struct {
	Avg *float64 `json:"avg,omitempty"`
	Max *float64 `json:"max,omitempty"`
	Min *float64 `json:"min,omitempty"`
}

// Zones is the time spent in each zone, per metric, lowest zone first.
type Zones struct {
	HR    []ZoneTime `json:"hr,omitempty"`
	Speed []ZoneTime `json:"speed,omitempty"`
	Power []ZoneTime `json:"power,omitempty"`
}

// ZoneTime is one zone [min, max) in the list's unit (bpm, km/h, W) and the
// time spent in it. Max is null on an open-ended top zone (Flow's 399 km/h /
// 2000 W sentinels); DistanceM is set on speed zones only.
type ZoneTime struct {
	Zone      int      `json:"zone"`
	Min       *float64 `json:"min"`
	Max       *float64 `json:"max"`
	TimeS     int      `json:"time_s"`
	DistanceM *float64 `json:"distance_m,omitempty"`
}

// Target is how the exercise went against the planned training target it was
// started from.
type Target struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Reached *bool  `json:"reached,omitempty"`
	Reps    []Rep  `json:"reps"`
}

// Rep is one executed phase repetition of a planned target, in execution order
// — the numbered segments the Flow web UI draws over its analysis charts. A
// "3 × (8 min work + 3 min walk)" block yields six reps.
type Rep struct {
	Index       int         `json:"index"` // 1-based, execution order; get_training_session_samples' rep
	Phase       string      `json:"phase,omitempty"`
	PhaseIndex  int         `json:"phase_index"` // 1-based planned phase
	Repetition  int         `json:"repetition"`  // 1-based repetition of that phase
	StartS      int         `json:"start_s"`
	EndS        int         `json:"end_s"`
	DurationS   int         `json:"duration_s"`
	DistanceM   *float64    `json:"distance_m"`
	AvgSpeedKmh *float64    `json:"avg_speed_kmh,omitempty"`
	MaxSpeedKmh *float64    `json:"max_speed_kmh,omitempty"`
	PaceSPerKm  *int        `json:"pace_s_per_km,omitempty"` // from the average speed
	HRAvg       *int        `json:"hr_avg,omitempty"`
	HRMax       *int        `json:"hr_max,omitempty"`
	PowerAvg    *int        `json:"power_avg_w,omitempty"`
	PowerMax    *int        `json:"power_max_w,omitempty"`
	CadenceAvg  *int        `json:"cadence_avg,omitempty"`
	TargetZone  *TargetZone `json:"target_zone,omitempty"`
	InZoneS     *int        `json:"in_zone_s,omitempty"`
	Finished    bool        `json:"finished"`
}

// TargetZone is a planned phase's intensity: zones lower..upper (1–5) of the
// sport's hr / speed / power zones.
type TargetZone struct {
	Type  string `json:"type"`
	Lower int    `json:"lower"`
	Upper int    `json:"upper"`
}

// Laps passes recorded laps through unchanged: their element shape has not
// been captured yet (internal/flow/openapi.yaml, SessionDetails.laps).
type Laps struct {
	Automatic []json.RawMessage `json:"automatic,omitempty"`
	Manual    []json.RawMessage `json:"manual,omitempty"`
}

// Hill is one climb or descent Flow detected (the altitude chart's segments).
type Hill struct {
	Type          string   `json:"type"` // "uphill" | "downhill"
	StartS        int      `json:"start_s"`
	DurationS     *int     `json:"duration_s,omitempty"`
	DistanceM     *float64 `json:"distance_m,omitempty"`
	ElevationM    *float64 `json:"elevation_m,omitempty"` // ascent of an uphill, descent of a downhill
	AvgInclinePct *float64 `json:"avg_incline_pct,omitempty"`
	HRAvg         *int     `json:"hr_avg,omitempty"`
	AvgSpeedKmh   *float64 `json:"avg_speed_kmh,omitempty"`
	PowerAvg      *int     `json:"power_avg_w,omitempty"`
}

// FromWireSessionDetails maps the details response to the compact view.
// Exercises come out in start order.
func FromWireSessionDetails(d *gen.SessionDetails) SessionDetails {
	out := SessionDetails{Exercises: []ExerciseDetail{}}
	if d == nil {
		return out
	}
	out.ID = d.ID.Or(0)
	exercises := d.Exercises.Or(nil)
	hills := hillsByExercise(d)
	for key, ex := range exercises {
		e := exerciseDetail(key, ex)
		if z, ok := d.Zones.Or(nil)[key]; ok {
			e.Zones = zonesFromWire(z)
		}
		if t, ok := d.ExerciseResultTargetData.Or(nil)[key]; ok {
			e.Target = targetFromWire(t)
		}
		if l, ok := d.Laps.Or(nil)[key]; ok {
			e.Laps = lapsFromWire(l)
		}
		if b, ok := d.Samples.Or(nil)[key]; ok {
			e.Samples = AvailableSampleMetrics(b)
		}
		// The raw start keeps its milliseconds (StartTime is normalised to the second).
		if start, ok := parseLocal(ex.StartTime.Or("")); ok && len(exercises) == 1 {
			// Hill segments hang off the period tree, not an exercise id;
			// attribute them only when there is no ambiguity.
			e.Hills = hillsFrom(hills, start)
		}
		out.Exercises = append(out.Exercises, e)
	}
	sort.SliceStable(out.Exercises, func(i, j int) bool {
		return out.Exercises[i].StartTime < out.Exercises[j].StartTime
	})
	return out
}

func exerciseDetail(key string, ex gen.SessionExercise) ExerciseDetail {
	e := ExerciseDetail{Samples: []string{}}
	e.ExerciseID = ex.ID.Or(0)
	if e.ExerciseID == 0 {
		e.ExerciseID, _ = strconv.ParseInt(key, 10, 64)
	}
	if id, ok := ex.Sport.Or(gen.SessionExerciseSport{}).ID.Get(); ok {
		e.SportID = &id
	}
	sid := 0
	if e.SportID != nil {
		sid = *e.SportID
	}
	e.SportCategory = SportCategory(ex.SportParent.Or(""), sid)
	if v, ok := ex.StartTime.Get(); ok {
		e.StartTime = NormalizeWireDatetime(v)
	}
	if v, ok := ex.Duration.Get(); ok {
		if secs, parsed := ISO8601DurationToSeconds(v); parsed {
			e.DurationS = &secs
		}
	}
	e.DistanceM = roundPtr(ex.Distance, 1)
	if v, ok := ex.Calories.Get(); ok {
		e.Calories = &v
	}
	e.AscentM = roundPtr(ex.Ascent, 1)
	e.DescentM = roundPtr(ex.Descent, 1)
	if v, ok := ex.RunningIndex.Get(); ok {
		e.RunningIndex = &v
	}
	if v, ok := ex.RecoveryTime.Get(); ok {
		if secs, parsed := ISO8601DurationToSeconds(v); parsed {
			e.RecoveryTimeS = &secs
		}
	}
	stats := ex.TrainingStatistic.Or(nil)
	e.HR = statFromWire(stats["HEART_RATE"], 0)
	e.Speed = statFromWire(stats["SPEED"], 2)
	e.Power = statFromWire(stats["POWER"], 0)
	e.Cadence = statFromWire(stats["CADENCE"], 0)
	return e
}

func statFromWire(s gen.SessionExerciseTrainingStatisticItem, decimals int) *Stat {
	out := Stat{
		Avg: roundPtr(s.Avg, decimals),
		Max: roundPtr(s.Max, decimals),
		Min: roundPtr(s.Min, decimals),
	}
	// A manual entry reports 0.0 for unknown HR; an all-zero stat says nothing.
	if isZeroOrNil(out.Avg) && isZeroOrNil(out.Max) && isZeroOrNil(out.Min) {
		return nil
	}
	return &out
}

func isZeroOrNil(p *float64) bool { return p == nil || *p == 0 }

func zonesFromWire(z gen.SessionZones) *Zones {
	out := Zones{
		HR:    zoneTimes(z.HEARTRATE, 0),
		Speed: zoneTimes(z.SPEED, openSpeedKmh),
		Power: zoneTimes(z.POWER, openPowerW),
	}
	if out.HR == nil && out.Speed == nil && out.Power == nil {
		return nil
	}
	return &out
}

// zoneTimes maps one zone list; openTop is the "no ceiling" sentinel of the
// list's top zone (0 when the list has none).
func zoneTimes(in []gen.SessionZone, openTop float64) []ZoneTime {
	if len(in) == 0 {
		return nil
	}
	out := make([]ZoneTime, 0, len(in))
	for i, z := range in {
		zt := ZoneTime{Zone: z.ZoneIndex.Or(i + 1)}
		if v, ok := z.LowerLimit.Get(); ok {
			zt.Min = &v
		}
		if v, ok := z.HigherLimit.Get(); ok && (openTop == 0 || v < openTop) {
			zt.Max = &v
		}
		if v, ok := z.InZone.Get(); ok {
			zt.TimeS, _ = ISO8601DurationToSeconds(v)
		}
		zt.DistanceM = roundPtr(z.Distance, 1)
		out = append(out, zt)
	}
	return out
}

// targetZoneType maps PhaseResultModel.zoneType to the canonical intensity
// names used by the training-target tools.
var targetZoneType = map[string]string{
	"HEART_RATE_ZONES": "hr",
	"SPEED_ZONES":      "speed",
	"POWER_ZONES":      "power",
}

func targetFromWire(t gen.ExerciseTargetResult) *Target {
	out := Target{Reps: []Rep{}}
	out.Name = t.Name.Or("")
	out.Type = t.Type.Or("")
	if v, ok := t.TargetReached.Get(); ok {
		out.Reached = &v
	}
	phases := map[int]gen.PhaseResultModel{}
	for _, p := range t.ExercisePhaseModels.Or(nil) {
		phases[p.PhaseIndex.Or(0)] = p
	}
	for i, r := range t.ExercisePhaseRepetitionModels.Or(nil) {
		out.Reps = append(out.Reps, repFromWire(i+1, r, phases[r.PhaseIndex.Or(0)]))
	}
	return &out
}

func repFromWire(index int, r gen.PhaseRepetitionResult, phase gen.PhaseResultModel) Rep {
	rep := Rep{
		Index:      index,
		Phase:      phase.Name.Or(""),
		PhaseIndex: r.PhaseIndex.Or(0),
		Repetition: r.PhaseRepetitionIndex.Or(0),
		Finished:   r.Finished.Or(false),
		DistanceM:  roundPtr(r.Distance, 1),
	}
	dur, _ := ISO8601DurationToSecondsF(r.Duration.Or(""))
	end, ok := ISO8601DurationToSecondsF(r.SplitTime.Or(""))
	if !ok {
		end = dur
	}
	rep.DurationS = int(math.Round(dur))
	rep.EndS = int(math.Round(end))
	rep.StartS = max(int(math.Round(end-dur)), 0)
	speed := r.SpeedStatistic.Or(gen.SessionStatistic{})
	rep.AvgSpeedKmh = roundPtr(speed.Avg, 2)
	rep.MaxSpeedKmh = roundPtr(speed.Max, 2)
	if rep.AvgSpeedKmh != nil {
		if p, ok := KmhToPaceSeconds(*rep.AvgSpeedKmh); ok {
			rep.PaceSPerKm = &p
		}
	}
	hr := r.HeartRateStatistic.Or(gen.SessionStatistic{})
	rep.HRAvg, rep.HRMax = roundIntPtr(hr.Avg), roundIntPtr(hr.Max)
	pw := r.PowerStatistic.Or(gen.SessionStatistic{})
	rep.PowerAvg, rep.PowerMax = roundIntPtr(pw.Avg), roundIntPtr(pw.Max)
	rep.CadenceAvg = roundIntPtr(r.CadenceStatistic.Or(gen.SessionStatistic{}).Avg)
	if v, ok := r.InZone.Get(); ok {
		if secs, parsed := ISO8601DurationToSeconds(v); parsed {
			rep.InZoneS = &secs
		}
	}
	if typ, ok := targetZoneType[phase.ZoneType.Or("")]; ok {
		lo, loOK := phase.LowerZone.Get()
		hi, hiOK := phase.UpperZone.Get()
		if loOK && hiOK {
			rep.TargetZone = &TargetZone{Type: typ, Lower: lo, Upper: hi}
		}
	}
	return rep
}

func lapsFromWire(l gen.SessionDetailsLapsItem) *Laps {
	if len(l.AutomaticLaps) == 0 && len(l.ManualLaps) == 0 {
		return nil
	}
	raw := func(in []jx.Raw) []json.RawMessage {
		out := make([]json.RawMessage, 0, len(in))
		for _, r := range in {
			out = append(out, json.RawMessage(r))
		}
		return out
	}
	return &Laps{Automatic: raw(l.AutomaticLaps), Manual: raw(l.ManualLaps)}
}

// hillsByExercise returns the UPHILL / DOWNHILL nodes under the period tree's
// EXERCISE node(s).
func hillsByExercise(d *gen.SessionDetails) []gen.PeriodData {
	root, ok := d.PeriodData.Get()
	if !ok {
		return nil
	}
	var out []gen.PeriodData
	var walk func(p gen.PeriodData)
	walk = func(p gen.PeriodData) {
		switch p.Type.Or("") {
		case "UPHILL", "DOWNHILL":
			out = append(out, p)
			return
		}
		for _, c := range p.Subperiods {
			walk(c)
		}
	}
	walk(root)
	return out
}

func hillsFrom(nodes []gen.PeriodData, exerciseStart time.Time) []Hill {
	out := make([]Hill, 0, len(nodes))
	for _, p := range nodes {
		data := p.Data.Or(nil)
		num := func(k string) *float64 {
			v, ok := data[k]
			if !ok {
				return nil
			}
			return roundPtr(v.NumValue, 1)
		}
		h := Hill{Type: strings.ToLower(p.Type.Or(""))}
		if t, ok := parseLocal(p.StartTime.Or(gen.PeriodTime{}).LocalDateTime.Or("")); ok {
			h.StartS = max(int(math.Round(t.Sub(exerciseStart).Seconds())), 0)
		}
		if ms := num("DURATION"); ms != nil { // milliseconds, unlike everything around it
			s := int(math.Round(*ms / 1000))
			h.DurationS = &s
		}
		h.DistanceM = num("DISTANCE")
		if h.Type == "uphill" {
			h.ElevationM = num("ASCENT")
		} else {
			h.ElevationM = num("DESCENT")
		}
		h.AvgInclinePct = num("AVG_INCLINE")
		h.HRAvg = intPtrF(num("AVG_HEART_RATE"))
		if v, ok := data["AVG_SPEED"]; ok {
			h.AvgSpeedKmh = roundPtr(v.NumValue, 2)
		}
		h.PowerAvg = intPtrF(num("AVG_POWER"))
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartS < out[j].StartS })
	return out
}

// parseLocal reads a wall-clock timestamp. PeriodData stamps carry a trailing
// "Z" that is not UTC (the instant is local), so the zone is dropped.
func parseLocal(s string) (time.Time, bool) {
	s = strings.TrimSuffix(s, "Z")
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

type nilFloat interface{ Get() (float64, bool) }

func roundPtr(v nilFloat, decimals int) *float64 {
	f, ok := v.Get()
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	r := RoundTo(f, decimals)
	return &r
}

func roundIntPtr(v nilFloat) *int {
	f, ok := v.Get()
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	i := int(math.Round(f))
	return &i
}

func intPtrF(p *float64) *int {
	if p == nil {
		return nil
	}
	i := int(math.Round(*p))
	return &i
}
