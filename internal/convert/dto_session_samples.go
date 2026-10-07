package convert

import (
	"fmt"
	"math"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// MaxSamplePoints caps the rows get_training_session_samples returns: a
// coarser resolution is picked when a window would need more.
const MaxSamplePoints = 1000

// sampleMetric is one series get_training_session_samples can return: the
// SampleBlock field it reads, the output column (canonical unit suffix), its
// rounding, and how a bucket aggregates (cumulative distance keeps the last
// value, everything else averages).
type sampleMetric struct {
	column   string
	decimals int
	last     bool
	pick     func(gen.SampleBlock) gen.OptNilSampleSeries
}

var sampleMetrics = map[string]sampleMetric{
	"hr":          {column: "hr_bpm", pick: func(b gen.SampleBlock) gen.OptNilSampleSeries { return b.HEARTRATE }},
	"speed":       {column: "speed_kmh", decimals: 2, pick: func(b gen.SampleBlock) gen.OptNilSampleSeries { return b.SPEED }},
	"power":       {column: "power_w", pick: func(b gen.SampleBlock) gen.OptNilSampleSeries { return b.POWER }},
	"cadence":     {column: "cadence", pick: func(b gen.SampleBlock) gen.OptNilSampleSeries { return b.CADENCE }},
	"altitude":    {column: "altitude_m", decimals: 1, pick: func(b gen.SampleBlock) gen.OptNilSampleSeries { return b.ALTITUDE }},
	"distance":    {column: "distance_m", decimals: 1, last: true, pick: func(b gen.SampleBlock) gen.OptNilSampleSeries { return b.DISTANCE }},
	"temperature": {column: "temperature_c", decimals: 1, pick: func(b gen.SampleBlock) gen.OptNilSampleSeries { return b.TEMPERATURE }},
}

// SampleMetricNames are the accepted `metrics` values, in output order. "pace"
// is derived from the bucket's average speed (seconds per km).
var SampleMetricNames = []string{"hr", "speed", "pace", "power", "cadence", "altitude", "distance", "temperature"}

// AvailableSampleMetrics lists the SampleMetricNames recorded in a block.
func AvailableSampleMetrics(b gen.SampleBlock) []string {
	out := []string{}
	for _, name := range SampleMetricNames {
		key := name
		if name == "pace" {
			key = "speed"
		}
		if s, ok := sampleMetrics[key].pick(b).Get(); ok && len(s.Samples.Values) > 0 {
			out = append(out, name)
		}
	}
	return out
}

// SampleWindow selects what ResampleSession returns: [FromS, ToS) seconds from
// the exercise start (ToS ≤ 0 = to the end) at ResolutionS-second buckets.
type SampleWindow struct {
	FromS, ToS  float64
	ResolutionS int
}

// SessionSamples is the get_training_session_samples payload: equal-length
// columns, one row per bucket. t_s is the bucket start in seconds from the
// exercise start; a bucket with no reading is null.
type SessionSamples struct {
	ExerciseID  int64                 `json:"exercise_id"`
	FromS       int                   `json:"from_s"`
	ToS         int                   `json:"to_s"`
	ResolutionS int                   `json:"resolution_s"`
	Points      int                   `json:"points"`
	Columns     map[string][]*float64 `json:"columns"`
}

// ResampleSession averages the requested metrics of one exercise's sample
// block into fixed buckets. durationS bounds an open-ended window (the
// exercise duration; ≤ 0 = the longest series). Unknown metric names are an
// error; metrics the exercise did not record come back all-null. The
// resolution is raised when the window would exceed MaxSamplePoints rows.
func ResampleSession(b gen.SampleBlock, exerciseID int64, durationS float64, metrics []string, w SampleWindow) (SessionSamples, error) {
	for _, m := range metrics {
		if _, ok := sampleMetrics[m]; !ok && m != "pace" {
			return SessionSamples{}, fmt.Errorf("unknown metric %q (want one of %v)", m, SampleMetricNames)
		}
	}
	end, res, err := resolveWindow(b, durationS, &w)
	if err != nil {
		return SessionSamples{}, err
	}
	n := int(math.Ceil((end - w.FromS) / float64(res)))

	out := SessionSamples{
		ExerciseID:  exerciseID,
		FromS:       int(math.Round(w.FromS)),
		ToS:         int(math.Round(end)),
		ResolutionS: res,
		Points:      n,
		Columns:     map[string][]*float64{},
	}
	ts := make([]*float64, n)
	for i := range ts {
		t := math.Round(w.FromS) + float64(i*res)
		ts[i] = &t
	}
	out.Columns["t_s"] = ts
	for _, m := range metrics {
		if m == "pace" {
			out.Columns["pace_s_per_km"] = paceColumn(b, w.FromS, end, res, n)
			continue
		}
		sm := sampleMetrics[m]
		col := bucketize(sm.pick(b), w.FromS, end, res, n, sm.last)
		for _, v := range col {
			if v != nil {
				*v = RoundTo(*v, sm.decimals)
			}
		}
		out.Columns[sm.column] = col
	}
	return out, nil
}

// resolveWindow clamps the window to [0, end) — end being the exercise
// duration, the longest series when unknown, or ToS when earlier — and
// coarsens the resolution to stay within MaxSamplePoints buckets.
func resolveWindow(b gen.SampleBlock, durationS float64, w *SampleWindow) (end float64, res int, err error) {
	end = durationS
	if end <= 0 {
		end = seriesSpan(b)
	}
	if w.ToS > 0 && w.ToS < end {
		end = w.ToS
	}
	w.FromS = max(w.FromS, 0)
	if end <= w.FromS {
		return 0, 0, fmt.Errorf("empty window: from_s %.0f is at or past the end (%.0f s)", w.FromS, end)
	}
	res = max(w.ResolutionS, 1)
	if math.Ceil((end-w.FromS)/float64(res)) > MaxSamplePoints {
		res = int(math.Ceil((end - w.FromS) / MaxSamplePoints))
	}
	return end, res, nil
}

// paceColumn derives seconds per km from each bucket's average speed; a
// standstill bucket has no pace.
func paceColumn(b gen.SampleBlock, from, end float64, res, n int) []*float64 {
	speed := bucketize(sampleMetrics["speed"].pick(b), from, end, res, n, false)
	pace := make([]*float64, n)
	for i, v := range speed {
		if v == nil {
			continue
		}
		if p, ok := KmhToPaceSeconds(*v); ok {
			f := float64(p)
			pace[i] = &f
		}
	}
	return pace
}

// seriesSpan is the duration covered by the longest series of a block.
func seriesSpan(b gen.SampleBlock) float64 {
	var span float64
	for _, sm := range sampleMetrics {
		if s, ok := sm.pick(b).Get(); ok {
			step, _ := ISO8601DurationToSecondsF(s.Samples.Interval)
			span = max(span, step*float64(len(s.Samples.Values)))
		}
	}
	return span
}

// bucketize folds one series into n buckets of res seconds starting at from:
// the mean of the non-null values in each (the last one when last is set).
func bucketize(opt gen.OptNilSampleSeries, from, to float64, res, n int, last bool) []*float64 {
	out := make([]*float64, n)
	s, ok := opt.Get()
	if !ok {
		return out
	}
	step, ok := ISO8601DurationToSecondsF(s.Samples.Interval)
	if !ok || step <= 0 {
		return out
	}
	sums := make([]float64, n)
	counts := make([]int, n)
	for i, v := range s.Samples.Values {
		t := float64(i) * step
		if t < from {
			continue
		}
		if t >= to {
			break
		}
		f, ok := v.Get()
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
			continue
		}
		k := int((t - from) / float64(res))
		if k >= n {
			break
		}
		if last {
			sums[k], counts[k] = f, 1
		} else {
			sums[k] += f
			counts[k]++
		}
	}
	for k := range out {
		if counts[k] > 0 {
			v := sums[k] / float64(counts[k])
			out[k] = &v
		}
	}
	return out
}
