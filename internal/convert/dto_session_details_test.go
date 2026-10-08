package convert

import (
	"os"
	"reflect"
	"testing"

	"github.com/go-faster/jx"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// loadDetailsFixture decodes the anonymized details response of a
// device-recorded interval run (3 × 8 min, PHASED target). Ids, time, place,
// GPS and altitude are synthetic and every metric is masked, with the stats
// recomputed from the masked series; series thinned to PT5S.
func loadDetailsFixture(t *testing.T) *gen.SessionDetails {
	t.Helper()
	b, err := os.ReadFile("../flow/testdata/session-details-intervals.json")
	if err != nil {
		t.Fatal(err)
	}
	var d gen.SessionDetails
	if err := d.Decode(jx.DecodeBytes(b)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return &d
}

//nolint:gocyclo // one sequential scenario
func TestFromWireSessionDetails(t *testing.T) {
	got := FromWireSessionDetails(loadDetailsFixture(t))
	if got.ID != 9000000001 || len(got.Exercises) != 1 {
		t.Fatalf("id=%d exercises=%d, want 9000000001 and 1", got.ID, len(got.Exercises))
	}
	ex := got.Exercises[0]
	if ex.ExerciseID != 9000000101 || ex.SportCategory != "run" {
		t.Errorf("exercise = %d/%q, want 9000000101/run", ex.ExerciseID, ex.SportCategory)
	}
	if ex.DurationS == nil || *ex.DurationS != 3683 {
		t.Errorf("DurationS = %v, want 3683 (PT1H1M23.138S)", ex.DurationS)
	}
	if ex.HR == nil || *ex.HR.Avg != 155 || *ex.HR.Max != 184 {
		t.Errorf("HR = %+v, want avg 155 max 184", ex.HR)
	}
	if want := []string{"hr", "speed", "pace", "power", "cadence", "altitude", "distance"}; !reflect.DeepEqual(ex.Samples, want) {
		t.Errorf("Samples = %v, want %v (temperature is null in the fixture)", ex.Samples, want)
	}
	if ex.Laps != nil {
		t.Errorf("Laps = %+v, want nil (none recorded)", ex.Laps)
	}

	z := ex.Zones
	if z == nil || len(z.HR) != 5 || len(z.Speed) != 5 || len(z.Power) != 5 {
		t.Fatalf("Zones = %+v, want 5 HR / speed / power zones", z)
	}
	if z.HR[0].Zone != 1 || z.HR[0].TimeS != 27 || *z.HR[0].Min != 105 || *z.HR[0].Max != 123 {
		t.Errorf("HR zone 1 = %+v, want zone 1, 27 s, [105, 123)", z.HR[0])
	}
	if z.Speed[4].Max != nil || z.Power[4].Max != nil {
		t.Errorf("top speed/power zone max = %v/%v, want nil (399 / 2000 sentinels)", z.Speed[4].Max, z.Power[4].Max)
	}
	if z.Speed[0].DistanceM == nil || z.HR[0].DistanceM != nil {
		t.Errorf("distance_m: speed %v hr %v, want set on speed zones only", z.Speed[0].DistanceM, z.HR[0].DistanceM)
	}

	tg := ex.Target
	if tg == nil || tg.Type != "PHASED" || len(tg.Reps) != 8 {
		t.Fatalf("Target = %+v, want PHASED with 8 reps", tg)
	}
	warm := tg.Reps[0]
	if warm.Index != 1 || warm.Phase != "Warm-up" || warm.StartS != 0 || warm.EndS != 780 || warm.DurationS != 780 {
		t.Errorf("rep 1 = %+v, want Warm-up 0–780 s", warm)
	}
	work2 := tg.Reps[3]
	if work2.Phase != "Work 8 min HR Z4" || work2.PhaseIndex != 2 || work2.Repetition != 2 {
		t.Errorf("rep 4 = %+v, want second repetition of phase 2", work2)
	}
	// splitTime PT32M12.388S − duration PT8M.
	if work2.StartS != 1452 || work2.EndS != 1932 {
		t.Errorf("rep 4 window = %d–%d, want 1452–1932", work2.StartS, work2.EndS)
	}
	if work2.TargetZone == nil || *work2.TargetZone != (TargetZone{Type: "hr", Lower: 4, Upper: 4}) {
		t.Errorf("rep 4 target zone = %+v, want hr 4–4", work2.TargetZone)
	}
	if work2.PaceSPerKm == nil || *work2.PaceSPerKm != 329 { // 10.95 km/h
		t.Errorf("rep 4 pace = %v, want 329 s/km", work2.PaceSPerKm)
	}
	if work2.HRAvg == nil || *work2.HRAvg != 169 || work2.InZoneS == nil || *work2.InZoneS != 392 {
		t.Errorf("rep 4 hr_avg/in_zone = %v/%v, want 169/392", work2.HRAvg, work2.InZoneS)
	}
	if walk := tg.Reps[2]; walk.TargetZone != nil || walk.InZoneS != nil {
		t.Errorf("walk rep target zone/in zone = %+v/%v, want nil (zoneType NONE)", walk.TargetZone, walk.InZoneS)
	}

	if len(ex.Hills) != 9 {
		t.Fatalf("Hills = %d, want 9", len(ex.Hills))
	}
	h := ex.Hills[0]
	if h.Type != "downhill" || h.StartS != 450 || h.DurationS == nil || *h.DurationS != 440 || *h.ElevationM != 32.6 {
		t.Errorf("hill 1 = %+v, want downhill at 450 s, 440 s, 32.6 m", h)
	}
}

func TestFromWireSessionDetailsManual(t *testing.T) {
	var d gen.SessionDetails
	in := `{"id":1,"exercises":{"2":{"id":2,"sport":{"id":1},"duration":"PT30M","distance":5000.0,
		"trainingStatistic":{"HEART_RATE":{"max":0.0,"avg":0.0,"min":null}}}},
		"samples":{"2":{"HEART_RATE":null,"SPEED":null}},"zones":{"2":{}},
		"laps":{"2":{"automaticLaps":[],"manualLaps":[]}},"periodData":null,"exerciseResultTargetData":{}}`
	if err := d.Decode(jx.DecodeStr(in)); err != nil {
		t.Fatal(err)
	}
	got := FromWireSessionDetails(&d)
	ex := got.Exercises[0]
	if ex.HR != nil || ex.Zones != nil || ex.Target != nil || ex.Laps != nil || len(ex.Hills) != 0 || len(ex.Samples) != 0 {
		t.Errorf("manual session detail = %+v, want no HR/zones/target/laps/hills/samples", ex)
	}
	if ex.DurationS == nil || *ex.DurationS != 1800 || *ex.DistanceM != 5000 {
		t.Errorf("duration/distance = %v/%v, want 1800/5000", ex.DurationS, ex.DistanceM)
	}
}

func series(interval string, vals ...any) gen.OptNilSampleSeries {
	s := gen.SampleSeries{Samples: gen.SampleSeriesSamples{Interval: interval}}
	for _, v := range vals {
		var n gen.NilFloat64
		if f, ok := v.(float64); ok {
			n.SetTo(f)
		} else {
			n.SetToNull()
		}
		s.Samples.Values = append(s.Samples.Values, n)
	}
	var o gen.OptNilSampleSeries
	o.SetTo(s)
	return o
}

func floats(col []*float64) []any {
	out := make([]any, len(col))
	for i, v := range col {
		if v != nil {
			out[i] = *v
		}
	}
	return out
}

//nolint:gocyclo // one sequential scenario
func TestResampleSession(t *testing.T) {
	b := gen.SampleBlock{
		HEARTRATE: series("PT1S", 100.0, 102.0, 104.0, 106.0, nil, 110.0),
		SPEED:     series("PT1S", 12.0, 12.0, 6.0, 6.0, 0.0, 0.0),
		DISTANCE:  series("PT1S", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0),
	}
	got, err := ResampleSession(b, 7, 6, []string{"hr", "pace", "distance", "power"}, SampleWindow{ResolutionS: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.Points != 3 || got.ResolutionS != 2 || got.FromS != 0 || got.ToS != 6 {
		t.Fatalf("header = %+v, want 3 points of 2 s over 0–6", got)
	}
	want := map[string][]any{
		"t_s":           {0.0, 2.0, 4.0},
		"hr_bpm":        {101.0, 105.0, 110.0}, // null skipped
		"pace_s_per_km": {300.0, 600.0, nil},   // 0 km/h has no pace
		"distance_m":    {2.0, 4.0, 6.0},       // last value, not the mean
		"power_w":       {nil, nil, nil},       // not recorded
	}
	for col, w := range want {
		if g := floats(got.Columns[col]); !reflect.DeepEqual(g, w) {
			t.Errorf("%s = %v, want %v", col, g, w)
		}
	}

	win, err := ResampleSession(b, 7, 6, []string{"hr"}, SampleWindow{FromS: 2, ToS: 4, ResolutionS: 60})
	if err != nil {
		t.Fatal(err)
	}
	if g := floats(win.Columns["hr_bpm"]); !reflect.DeepEqual(g, []any{105.0}) || win.Columns["t_s"][0] == nil || *win.Columns["t_s"][0] != 2 {
		t.Errorf("window 2–4 hr = %v t=%v, want [105] at t=2", g, win.Columns["t_s"])
	}

	long, err := ResampleSession(b, 7, 5000, []string{"hr"}, SampleWindow{ResolutionS: 1})
	if err != nil {
		t.Fatal(err)
	}
	if long.ResolutionS != 5 || long.Points != MaxSamplePoints {
		t.Errorf("coarsened = %d s / %d points, want 5 s / %d", long.ResolutionS, long.Points, MaxSamplePoints)
	}

	if _, err := ResampleSession(b, 7, 6, []string{"vo2"}, SampleWindow{ResolutionS: 1}); err == nil {
		t.Error("unknown metric: want an error")
	}
	if _, err := ResampleSession(b, 7, 6, []string{"hr"}, SampleWindow{FromS: 10, ResolutionS: 1}); err == nil {
		t.Error("window past the end: want an error")
	}
	// No duration → the longest series bounds the window.
	if open, err := ResampleSession(b, 7, 0, []string{"hr"}, SampleWindow{ResolutionS: 3}); err != nil || open.ToS != 6 {
		t.Errorf("open window = %+v, %v; want to_s 6", open, err)
	}
}

func TestISO8601DurationToSecondsF(t *testing.T) {
	for in, want := range map[string]float64{"PT1H1M23.138S": 3683.138, "PT4.013S": 4.013, "PT8M": 480} {
		if got, ok := ISO8601DurationToSecondsF(in); !ok || got < want-1e-9 || got > want+1e-9 {
			t.Errorf("%s = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := ISO8601DurationToSecondsF("8 min"); ok {
		t.Error("garbage: want ok=false")
	}
}
