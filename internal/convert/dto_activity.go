package convert

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/go-faster/jx"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Units of the activity timeline, read from the Flow web UI's own rendering
// (flow-ui-mono chunk 9450, 2026-09-30) — the spec's first draft had them
// wrong: activeTime, sleepDuration and every activityScoreData duration are
// MINUTES, distanceFromSteps is METRES, dailyActivityGoal is a PERCENTAGE of
// the daily goal. Intraday samples are {time, value} with time an epoch-ms
// that encodes the local wall clock as if it were UTC (the UI shifts lastSync,
// a real UTC instant, by the browser's offset before comparing).

// DailyActivity is one day of 24/7 activity tracking. Every metric is null
// when HasData is false: Flow answers zeros for days with no synced device
// data and a null panel for days after the last sync, and both mean "no data"
// rather than "a day of zero steps".
type DailyActivity struct {
	Date             string             `json:"date"`
	HasData          bool               `json:"has_data"`
	GoalPct          *float64           `json:"activity_goal_pct"`
	Steps            *int               `json:"steps"`
	StepDistanceM    *int               `json:"step_distance_m"`
	ActiveTimeS      *int               `json:"active_time_s"`
	KCal             *int               `json:"kcal"`
	SleepS           *int               `json:"sleep_s"`
	InactivityAlerts *int               `json:"inactivity_alerts"`
	SleepPlus        bool               `json:"sleep_plus,omitempty"`
	LastSync         *string            `json:"last_sync"`
	Intensity        *ActivityIntensity `json:"intensity"`
	HeartRate        *DailyHeartRate    `json:"heart_rate"`
	Benefit          *ActivityBenefit   `json:"benefit"`
	Samples          *ActivitySamples   `json:"samples,omitempty"`
}

// ActivityIntensity is the day's time per activity-intensity bucket, seconds
// (Polar's "activity score" bar: rest/sleep, sitting, light, moderate,
// vigorous).
type ActivityIntensity struct {
	SleepS     int `json:"sleep_s"`
	SedentaryS int `json:"sedentary_s"`
	LightS     int `json:"light_s"`
	ModerateS  int `json:"moderate_s"`
	VigorousS  int `json:"vigorous_s"`
}

// DailyHeartRate is the day's 24/7 heart-rate summary in bpm; null fields
// had no reading.
type DailyHeartRate struct {
	DayMinBPM   *int `json:"day_min_bpm"`
	DayMaxBPM   *int `json:"day_max_bpm"`
	NightMinBPM *int `json:"night_min_bpm"`
}

// ActivityBenefit is Polar's qualitative "activity benefit" feedback: mvpa
// and sitting are lowercased Polar levels ("none" when nothing to say), the
// lists name the health benefits reached.
type ActivityBenefit struct {
	MVPA     string   `json:"mvpa,omitempty"`
	Sitting  string   `json:"sitting,omitempty"`
	Improves []string `json:"improves,omitempty"`
	Promotes []string `json:"promotes,omitempty"`
	Aids     []string `json:"aids,omitempty"`
}

// ActivitySamples are the day's downsampled intraday series. Clock is the
// local wall clock "HH:MM". Activity values are Polar's activity-intensity
// level; ZoneLimits are the level boundaries of its intensity bands (a value
// under ZoneLimits[1] reads as non-wear in the UI).
type ActivitySamples struct {
	Activity   []ActivitySample `json:"activity"`
	HeartRate  []ActivitySample `json:"heart_rate"`
	ZoneLimits []float64        `json:"zone_limits,omitempty"`
}

// ActivitySample is one intraday point.
type ActivitySample struct {
	Clock string  `json:"t"`
	Value float64 `json:"v"`
}

// FromWireActivityDay maps one day of /api/activity-timeline/load[Four].
// withSamples keeps the intraday series (sized by the request's
// maxSampleCount).
func FromWireActivityDay(date time.Time, d gen.ActivityTimelineDay, withSamples bool) DailyActivity {
	out := DailyActivity{Date: date.Format(ISODate)}
	panel, ok := d.DataPanelData.Get()
	graph, _ := d.ActivityGraphData.Get()
	lastSync, synced := graph.LastSync.Get()
	if !ok || (!synced && panelIsEmpty(panel)) {
		return out
	}
	out.HasData = true
	out.GoalPct = optFloat(panel.DailyActivityGoal)
	out.Steps = optInt(panel.StepCount)
	if v, ok := panel.DistanceFromSteps.Get(); ok {
		m := int(math.Round(float64(v)))
		out.StepDistanceM = &m
	}
	out.ActiveTimeS = minutesPtr(panel.ActiveTime)
	out.KCal = optInt(panel.KiloCalories)
	out.SleepS = minutesPtr(panel.SleepDuration)
	out.InactivityAlerts = optInt(panel.InactivityAlertCount)
	out.SleepPlus = panel.SleepPlus.Or(false)
	if synced {
		s := time.UnixMilli(int64(lastSync)).UTC().Format(time.RFC3339)
		out.LastSync = &s
	}
	if sc, ok := d.ActivityScoreData.Get(); ok {
		out.Intensity = &ActivityIntensity{
			SleepS:     sc.SleepDuration.Or(0) * 60,
			SedentaryS: sc.SedentaryDuration.Or(0) * 60,
			LightS:     sc.LightDuration.Or(0) * 60,
			ModerateS:  sc.ModerateDuration.Or(0) * 60,
			VigorousS:  sc.VigorousDuration.Or(0) * 60,
		}
	}
	if hr, ok := graph.HeartRateSummary.Get(); ok {
		out.HeartRate = &DailyHeartRate{
			DayMinBPM:   positive(hr.DayMinimum),
			DayMaxBPM:   positive(hr.DayMaximum),
			NightMinBPM: positive(hr.NightMinimum),
		}
		if out.HeartRate.DayMinBPM == nil && out.HeartRate.DayMaxBPM == nil && out.HeartRate.NightMinBPM == nil {
			out.HeartRate = nil
		}
	}
	if b, ok := d.ActivityBenefitFeedbackData.Get(); ok {
		out.Benefit = activityBenefit(b)
	}
	if withSamples {
		dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
		s := &ActivitySamples{
			Activity:  intradaySamples(graph.ActivityTimelineSamples, dayStart, false),
			HeartRate: intradaySamples(graph.HeartRateTimelineSamples, dayStart, true),
		}
		for _, z := range graph.ActivityZoneLimits {
			s.ZoneLimits = append(s.ZoneLimits, math.Round(float64(z)*100)/100)
		}
		out.Samples = s
	}
	return out
}

// activityBenefit maps the feedback block; nil when it says nothing (levels
// "NONE", no benefits listed).
func activityBenefit(b gen.ActivityTimelineDayActivityBenefitFeedbackData) *ActivityBenefit {
	level := func(v gen.OptString) string {
		if l := strings.ToLower(v.Or("")); l != "none" {
			return l
		}
		return ""
	}
	out := &ActivityBenefit{MVPA: level(b.Mvpa), Sitting: level(b.Sitting), Improves: b.Improves, Promotes: b.Promotes, Aids: b.Aids}
	if out.MVPA == "" && out.Sitting == "" && len(out.Improves)+len(out.Promotes)+len(out.Aids) == 0 {
		return nil
	}
	return out
}

func panelIsEmpty(p gen.ActivityTimelineDayDataPanelData) bool {
	return p.StepCount.Or(0) == 0 && p.ActiveTime.Or(0) == 0 && p.KiloCalories.Or(0) == 0 &&
		p.SleepDuration.Or(0) == 0 && p.DailyActivityGoal.Or(0) == 0
}

// intradaySamples decodes {time, value} points (element shape read from the
// web UI, never observed populated — so decoding is tolerant: an element that
// does not fit is skipped rather than failing the day). dropZero drops
// value-0 points, which the UI treats as heart-rate gaps.
func intradaySamples(raw []jx.Raw, dayStartMs int64, dropZero bool) []ActivitySample {
	out := []ActivitySample{}
	for _, r := range raw {
		var p struct {
			Time  *float64 `json:"time"`
			Value *float64 `json:"value"`
		}
		if json.Unmarshal(r, &p) != nil || p.Time == nil || p.Value == nil {
			continue
		}
		if dropZero && *p.Value == 0 {
			continue
		}
		min := (int64(*p.Time) - dayStartMs) / 60000
		if min < 0 || min >= 24*60 {
			continue
		}
		out = append(out, ActivitySample{
			Clock: fmt.Sprintf("%02d:%02d", min/60, min%60),
			Value: math.Round(*p.Value*100) / 100,
		})
	}
	return out
}

func optInt(v gen.OptInt) *int {
	if x, ok := v.Get(); ok {
		return &x
	}
	return nil
}

func optFloat(v gen.OptFloat64) *float64 {
	if x, ok := v.Get(); ok {
		return &x
	}
	return nil
}

func minutesPtr(v gen.OptInt) *int {
	if x, ok := v.Get(); ok {
		s := x * 60
		return &s
	}
	return nil
}

func positive(v gen.OptInt) *int {
	if x, ok := v.Get(); ok && x > 0 {
		return &x
	}
	return nil
}

// ActivitySummary totals the days of a range that have data; averages are
// per day with data (sleep: per day with recorded sleep).
type ActivitySummary struct {
	DaysWithData int  `json:"days_with_data"`
	TotalSteps   int  `json:"total_steps"`
	AvgSteps     *int `json:"avg_steps"`
	AvgActiveS   *int `json:"avg_active_time_s"`
	AvgKCal      *int `json:"avg_kcal"`
	AvgSleepS    *int `json:"avg_sleep_s"`
}

// SummarizeActivity totals and averages the days that have data.
func SummarizeActivity(days []DailyActivity) ActivitySummary {
	var s ActivitySummary
	var active, kcal, sleep, sleepN int
	for _, d := range days {
		if !d.HasData {
			continue
		}
		s.DaysWithData++
		s.TotalSteps += derefInt(d.Steps)
		active += derefInt(d.ActiveTimeS)
		kcal += derefInt(d.KCal)
		if d.SleepS != nil && *d.SleepS > 0 {
			sleep += *d.SleepS
			sleepN++
		}
	}
	if n := s.DaysWithData; n > 0 {
		s.AvgSteps, s.AvgActiveS, s.AvgKCal = intPtr(s.TotalSteps/n), intPtr(active/n), intPtr(kcal/n)
	}
	if sleepN > 0 {
		s.AvgSleepS = intPtr(sleep / sleepN)
	}
	return s
}

func intPtr(v int) *int { return &v }
