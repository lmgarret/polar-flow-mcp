package convert

import (
	"math"
	"strings"
	"time"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Sleep semantics, read from the Flow web UI's sleep report
// (flow-ui-mono chunk 1608, 2026-09-30); never observed on a populated
// account, so this mirrors the UI rather than a capture:
//
//   - sleepStartTime / sleepEndTime carry an offset; the UI uses their local
//     wall clock. Fell asleep = sleepStartTime + sleepStartOffset seconds,
//     woke up = sleepEndTime + sleepEndOffset seconds.
//   - sleepWakeStates is a transition list: each entry starts a segment of
//     its state at offsetFromStart seconds after sleepStartTime, lasting until
//     the next entry (the last one until wake-up). States: 0 wake
//     (interruption, flagged longInterruption), 1 REM, 2 light, 3 deep,
//     4 unknown. Deep only counts on a "Sleep Plus Stages" night (one with
//     sleep cycles or any REM/light state).
//   - sleepScore 0 means "no score".

// Sleep stage names used in SleepSegment.Stage.
const (
	StageWake    = "wake"
	StageREM     = "rem"
	StageLight   = "light"
	StageDeep    = "deep"
	StageUnknown = "unknown"
	StageSleep   = "sleep" // state 3 on a night without Sleep Plus Stages
)

// SleepNight is one recorded night, keyed by the date the user woke up.
// Times are the local wall clock, ISO 8601 without offset.
type SleepNight struct {
	Date            string         `json:"date"`
	FellAsleep      string         `json:"fell_asleep"`
	WokeUp          string         `json:"woke_up"`
	SleepS          int            `json:"sleep_s"`
	Score           *float64       `json:"score"`
	ContinuityIndex *float64       `json:"continuity_index"`
	ContinuityClass *int           `json:"continuity_class"`
	Cycles          *int           `json:"sleep_cycles"`
	Rating          *string        `json:"rating"`
	Stages          *SleepStages   `json:"stages"`
	InterruptionsS  int            `json:"interruptions_s"`
	LongInterrupts  int            `json:"long_interruptions_s"`
	Hypnogram       []SleepSegment `json:"hypnogram"`
}

// SleepStages totals a Sleep Plus Stages night's time per stage, seconds.
type SleepStages struct {
	LightS   int `json:"light_s"`
	DeepS    int `json:"deep_s"`
	REMS     int `json:"rem_s"`
	UnknownS int `json:"unknown_s"`
}

// SleepSegment is one stretch of a single state, in seconds after falling
// asleep.
type SleepSegment struct {
	Stage  string `json:"stage"`
	StartS int    `json:"start_s"`
	EndS   int    `json:"end_s"`
	Long   bool   `json:"long,omitempty"`
}

// sleepClockLayout is the canonical local wall-clock datetime.
const sleepClockLayout = "2006-01-02T15:04:05"

// FromWireSleepNight maps one night of /api/sleep/report. ok=false when its
// start/end times do not parse (the night is then skipped rather than failing
// the report).
func FromWireSleepNight(n gen.SleepNight) (SleepNight, bool) {
	start, ok1 := wallClock(n.SleepStartTime)
	end, ok2 := wallClock(n.SleepEndTime)
	if !ok1 || !ok2 {
		return SleepNight{}, false
	}
	// A null offset counts as 0, as moment.add(null) does in the web UI.
	asleep := start.Add(time.Duration(n.SleepStartOffset.Or(0)) * time.Second)
	woke := end.Add(time.Duration(n.SleepEndOffset.Or(0)) * time.Second)
	out := SleepNight{
		Date:       n.Date.Format(ISODate),
		FellAsleep: asleep.Format(sleepClockLayout),
		WokeUp:     woke.Format(sleepClockLayout),
		SleepS:     max(0, int(woke.Sub(asleep).Seconds())),
	}
	sleepScalars(n, &out)
	stagesNight := out.Cycles != nil && *out.Cycles > 0
	for _, s := range n.SleepWakeStates {
		if st := s.SleepWakeState.Or(-1); st == 1 || st == 2 {
			stagesNight = true
		}
	}
	stages := sleepHypnogram(n, int(woke.Sub(start).Seconds()), stagesNight, &out)
	if stagesNight {
		out.Stages = &stages
	}
	return out, true
}

// sleepScalars copies the per-night scores; a zero Sleep Score is "no score".
func sleepScalars(n gen.SleepNight, out *SleepNight) {
	if v, ok := n.SleepScore.Get(); ok && v > 0 {
		v = math.Round(v*10) / 10
		out.Score = &v
	}
	if v, ok := n.ContinuityIndex.Get(); ok {
		v = math.Round(v*10) / 10
		out.ContinuityIndex = &v
	}
	if v, ok := n.ContinuityClass.Get(); ok {
		out.ContinuityClass = &v
	}
	if v, ok := n.SleepCycles.Get(); ok {
		out.Cycles = &v
	}
	if v, ok := n.SleepRating.Get(); ok && v != "" {
		r := strings.ToLower(strings.TrimPrefix(v, "SLEPT_"))
		out.Rating = &r
	}
}

// sleepHypnogram turns the state transitions into segments — in seconds
// after sleepStartTime, then shifted to "after falling asleep" and clipped to
// the sleep period — filling out.Hypnogram and the interruption totals and
// returning the per-stage totals. wokeRaw is wake-up in seconds after
// sleepStartTime.
func sleepHypnogram(n gen.SleepNight, wokeRaw int, stagesNight bool, out *SleepNight) SleepStages {
	var stages SleepStages
	out.Hypnogram = []SleepSegment{}
	startOff := n.SleepStartOffset.Or(0)
	for i, s := range n.SleepWakeStates {
		segEnd := wokeRaw
		if i+1 < len(n.SleepWakeStates) {
			segEnd = n.SleepWakeStates[i+1].OffsetFromStart.Or(wokeRaw)
		}
		a := max(s.OffsetFromStart.Or(0), startOff) - startOff
		b := min(segEnd, wokeRaw) - startOff
		if b <= a {
			continue
		}
		seg := SleepSegment{StartS: a, EndS: b}
		d := b - a
		switch s.SleepWakeState.Or(-1) {
		case 0:
			seg.Stage, seg.Long = StageWake, s.LongInterruption.Or(false)
			out.InterruptionsS += d
			if seg.Long {
				out.LongInterrupts += d
			}
		case 1:
			seg.Stage, stages.REMS = StageREM, stages.REMS+d
		case 2:
			seg.Stage, stages.LightS = StageLight, stages.LightS+d
		case 3:
			if stagesNight {
				seg.Stage, stages.DeepS = StageDeep, stages.DeepS+d
			} else {
				seg.Stage = StageSleep
			}
		default:
			seg.Stage, stages.UnknownS = StageUnknown, stages.UnknownS+d
		}
		out.Hypnogram = append(out.Hypnogram, seg)
	}
	return stages
}

// wallClock parses a Flow datetime (with or without an offset) and returns
// its local wall clock as a zone-less time.
func wallClock(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC), true
		}
	}
	return time.Time{}, false
}

// SleepAverages summarizes nights the way Polar's sleep report does: averages
// over the nights that have the value (score > 0, stages on stage nights).
type SleepAverages struct {
	Nights     int      `json:"nights"`
	SleepS     *int     `json:"sleep_s"`
	Score      *float64 `json:"score"`
	LightS     *int     `json:"light_s"`
	DeepS      *int     `json:"deep_s"`
	REMS       *int     `json:"rem_s"`
	FellAsleep *string  `json:"fell_asleep"`
	WokeUp     *string  `json:"woke_up"`
}

// AverageSleep averages the nights. Bedtimes average as clock times around
// midnight ("23:40"), wake times as clock times.
func AverageSleep(nights []SleepNight) SleepAverages {
	out := SleepAverages{Nights: len(nights)}
	if len(nights) == 0 {
		return out
	}
	var sleep, light, deep, rem, fell, woke []float64
	var score []float64
	for _, n := range nights {
		sleep = append(sleep, float64(n.SleepS))
		if n.Score != nil {
			score = append(score, *n.Score)
		}
		if n.Stages != nil {
			light = append(light, float64(n.Stages.LightS))
			deep = append(deep, float64(n.Stages.DeepS))
			rem = append(rem, float64(n.Stages.REMS))
		}
		if t, err := time.Parse(sleepClockLayout, n.FellAsleep); err == nil {
			// Seconds from the previous noon, so 23:30 and 00:30 average to 00:00.
			sec := float64(t.Hour()*3600+t.Minute()*60+t.Second()) - 12*3600
			if sec < 0 {
				sec += 24 * 3600
			}
			fell = append(fell, sec)
		}
		if t, err := time.Parse(sleepClockLayout, n.WokeUp); err == nil {
			woke = append(woke, float64(t.Hour()*3600+t.Minute()*60+t.Second()))
		}
	}
	out.SleepS = avgInt(sleep)
	if len(score) > 0 {
		v := math.Round(mean(score)*10) / 10
		out.Score = &v
	}
	out.LightS, out.DeepS, out.REMS = avgInt(light), avgInt(deep), avgInt(rem)
	if len(fell) > 0 {
		c := clockOf(int(mean(fell)) + 12*3600)
		out.FellAsleep = &c
	}
	if len(woke) > 0 {
		c := clockOf(int(mean(woke)))
		out.WokeUp = &c
	}
	return out
}

func mean(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func avgInt(v []float64) *int {
	if len(v) == 0 {
		return nil
	}
	x := int(math.Round(mean(v)))
	return &x
}

// clockOf formats seconds since midnight (mod 24 h) as "HH:MM".
func clockOf(sec int) string {
	sec = ((sec % 86400) + 86400) % 86400
	return time.Date(0, 1, 1, 0, 0, sec, 0, time.UTC).Format(clock24)
}
