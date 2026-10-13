package mcp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// targetBody builds a create body from tool arguments and returns it as
// generic JSON, failing on a build error.
func targetBody(t *testing.T, args map[string]any) map[string]any {
	t.Helper()
	base := map[string]any{"name": "t", "date": "2026-12-08", "time": "07:00"}
	for k, v := range args {
		base[k] = v
	}
	body, errMsg := buildTrainingTargetCreate(req(base))
	if errMsg != "" {
		t.Fatalf("unexpected build error: %s", errMsg)
	}
	raw, err := body.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func firstET(m map[string]any) map[string]any { return objAt(m["exerciseTargets"], 0) }

// objAt returns element i of a decoded JSON array as an object (nil when the
// shape differs, which the caller's assertions then report).
func objAt(v any, i int) map[string]any {
	list, _ := v.([]any)
	if i >= len(list) {
		return nil
	}
	m, _ := list[i].(map[string]any)
	return m
}

// The goal arguments pick the target type the way Flow's editor does
// (probed 2026-10-13): nothing → FREE, one metric → VOLUME, duration +
// distance → STEADY_RACE_PACE, phases → PHASED; kcal rides along with the
// last two.
func TestBuildTrainingTargetCreate_GoalTypes(t *testing.T) {
	block := []any{map[string]any{"type": "block", "duration_s": 600.0}}
	cases := []struct {
		name string
		args map[string]any
		typ  string
		want map[string]any // expected exerciseTarget fields
	}{
		{"free", map[string]any{}, "FREE", map[string]any{"duration": nil, "distance": nil, "calories": nil}},
		{"kcal", map[string]any{"kcal": 500.0}, "VOLUME", map[string]any{"calories": 500.0, "duration": nil}},
		{"duration", map[string]any{"duration_s": 2100.0}, "VOLUME", map[string]any{"duration": "00:35:00"}},
		{"distance", map[string]any{"distance_m": 10000.0}, "VOLUME", map[string]any{"distance": 10000.0}},
		{"race pace", map[string]any{"duration_s": 2700.0, "distance_m": 10000.0, "kcal": 600.0}, "STEADY_RACE_PACE",
			map[string]any{"duration": "00:45:00", "distance": 10000.0, "calories": 600.0}},
		{"phased + kcal", map[string]any{"phases": block, "kcal": 400.0}, "PHASED", map[string]any{"calories": 400.0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := targetBody(t, c.args)
			if m["type"] != c.typ {
				t.Fatalf("type = %v, want %s", m["type"], c.typ)
			}
			et := firstET(m)
			for k, v := range c.want {
				if !reflect.DeepEqual(et[k], v) {
					t.Fatalf("%s = %#v, want %#v (exerciseTarget %v)", k, et[k], v, et)
				}
			}
		})
	}
}

// The phase tree the web editor saved for a nested ladder (captured from the
// UI's own update request 2026-10-13): manual phase changes, a repeat inside
// a repeat, mixed goals and intensities, and a zone-free phase.
func TestBuildTrainingTargetCreate_NestedTreeMatchesUI(t *testing.T) {
	m := targetBody(t, map[string]any{"phases": []any{
		map[string]any{"type": "warmup", "name": "WU", "duration_s": 300.0, "manual": true},
		map[string]any{"type": "repeat", "reps": 3.0, "phases": []any{
			map[string]any{"type": "block", "name": "A", "distance_m": 1000.0,
				"intensity": map[string]any{"hr_zone": 4.0, "zone_max": 5.0}},
			map[string]any{"type": "repeat", "reps": 2.0, "phases": []any{
				map[string]any{"type": "block", "name": "B", "duration_s": 30.0,
					"intensity": map[string]any{"speed_zone": 3.0, "zone_max": 4.0}},
				map[string]any{"type": "block", "name": "C", "distance_m": 200.0},
			}},
			map[string]any{"type": "block", "name": "D", "duration_s": 120.0, "manual": true,
				"intensity": map[string]any{"hr_zone": 1.0}},
		}},
		map[string]any{"type": "cooldown", "name": "CD", "distance_m": 1500.0,
			"intensity": map[string]any{"power_zone": 1.0, "zone_max": 2.0}},
	}})
	leaf := func(name, chg, goal string, v any, it string, lo, hi any) map[string]any {
		p := map[string]any{"phaseType": "PHASE", "name": name, "phaseChangeType": chg, "goalType": goal,
			"intensityType": it, "lowerZone": lo, "upperZone": hi}
		if goal == "DISTANCE" {
			p["distance"] = v
		} else {
			p["duration"] = v
		}
		return p
	}
	want := []any{
		leaf("WU", "MANUAL", "DURATION", "00:05:00", "NONE", nil, nil),
		map[string]any{"phaseType": "REPEAT", "repeatCount": 3.0, "phases": []any{
			leaf("A", "AUTOMATIC", "DISTANCE", 1000.0, "HEART_RATE_ZONES", 4.0, 5.0),
			map[string]any{"phaseType": "REPEAT", "repeatCount": 2.0, "phases": []any{
				leaf("B", "AUTOMATIC", "DURATION", "00:00:30", "SPEED_ZONES", 3.0, 4.0),
				leaf("C", "AUTOMATIC", "DISTANCE", 200.0, "NONE", nil, nil),
			}},
			leaf("D", "MANUAL", "DURATION", "00:02:00", "HEART_RATE_ZONES", 1.0, 1.0),
		}},
		leaf("CD", "AUTOMATIC", "DISTANCE", 1500.0, "POWER_ZONES", 1.0, 2.0),
	}
	got := firstET(m)["phases"]
	stripNulls(got)
	stripNulls(want)
	if !reflect.DeepEqual(got, want) {
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		t.Fatalf("phases =\n%s\nwant\n%s", g, w)
	}
}

// stripNulls drops null-valued keys from a decoded JSON tree in place, so a
// comparison is insensitive to whether an unset field is sent as null.
func stripNulls(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if e == nil {
				delete(x, k)
			} else {
				stripNulls(e)
			}
		}
	case []any:
		for _, e := range x {
			stripNulls(e)
		}
	}
}

// The repeat shorthand's recovery takes a distance, zones and a manual
// advance, like any phase in the web editor.
func TestBuildTrainingTargetCreate_RecoveryOptions(t *testing.T) {
	m := targetBody(t, map[string]any{"phases": []any{map[string]any{
		"type": "repeat", "reps": 6.0, "goal": map[string]any{"distance_m": 1000.0},
		"intensity": map[string]any{"hr_zone": 4.0}, "manual": true,
		"recovery": map[string]any{"distance_m": 400.0, "manual": true, "name": "Jog",
			"intensity": map[string]any{"hr_zone": 1.0, "zone_max": 2.0}},
	}}})
	rep := objAt(firstET(m)["phases"], 0)
	work := objAt(rep["phases"], 0)
	rec := objAt(rep["phases"], 1)
	if work["phaseChangeType"] != "MANUAL" || work["lowerZone"] != 4.0 {
		t.Fatalf("work = %v", work)
	}
	if rec["goalType"] != "DISTANCE" || rec["distance"] != 400.0 || rec["phaseChangeType"] != "MANUAL" ||
		rec["intensityType"] != "HEART_RATE_ZONES" || rec["lowerZone"] != 1.0 || rec["upperZone"] != 2.0 ||
		rec["name"] != "Jog" {
		t.Fatalf("recovery = %v", rec)
	}
}

// exercises builds one exercise target per sport, in order.
func TestBuildTrainingTargetCreate_MultiSport(t *testing.T) {
	m := targetBody(t, map[string]any{"exercises": []any{
		map[string]any{"sport_id": 1.0, "phases": []any{map[string]any{"type": "block", "duration_s": 1800.0}}},
		map[string]any{"sport_id": 2.0, "phases": []any{map[string]any{"type": "block", "distance_m": 20000.0,
			"intensity": map[string]any{"power_zone": 2.0, "zone_max": 3.0}}}},
	}})
	ets, _ := m["exerciseTargets"].([]any)
	if m["type"] != "PHASED" || len(ets) != 2 {
		t.Fatalf("body = %v", m)
	}
	if objAt(ets, 0)["sportId"] != 1.0 || objAt(ets, 1)["sportId"] != 2.0 {
		t.Fatalf("sports = %v", ets)
	}
}

// Every limit of the editor (and of Flow) is rejected before anything is sent.
func TestCreateTrainingTarget_GoalValidation(t *testing.T) {
	blk := func(extra map[string]any) map[string]any {
		p := map[string]any{"type": "block", "duration_s": 60.0}
		for k, v := range extra {
			p[k] = v
		}
		return p
	}
	rep := func(reps float64, phases ...any) map[string]any {
		return map[string]any{"type": "repeat", "reps": reps, "phases": phases}
	}
	args := func(extra map[string]any) map[string]any {
		m := map[string]any{"name": "ok", "date": "2026-12-01", "time": "07:00"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	ph := func(p ...any) map[string]any { return args(map[string]any{"phases": p}) }
	leaves := func(n int) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = blk(nil)
		}
		return out
	}
	runValidationCases(t, CreateTrainingTargetHandler, []validationCase{
		{"kcal with duration", args(map[string]any{"kcal": 300.0, "duration_s": 600.0}), "kcal combines only with"},
		{"kcal too high", args(map[string]any{"kcal": 65536.0}), "between 1 and 65535"},
		{"duration 100h", args(map[string]any{"duration_s": 360000.0}), "between 1 and 359999"},
		{"distance too far", args(map[string]any{"distance_m": 9999001.0}), "between 1 and 9.999e+06"},
		{"phases with duration", args(map[string]any{"duration_s": 600.0, "phases": []any{blk(nil)}}), "not both"},
		{"phase duration 100h", ph(blk(map[string]any{"duration_s": 360000.0})), "phases[0] (block): duration_s must be between 1 and 359999"},
		{"phase distance 0", ph(map[string]any{"type": "block", "distance_m": 0.0}), "distance_m must be between 1"},
		{"manual not bool", ph(blk(map[string]any{"manual": "yes"})), "manual must be true or false"},
		{"reps 100", ph(rep(100, blk(nil))), "reps must be between 2 and 99"},
		{"reps missing", ph(map[string]any{"type": "repeat", "phases": []any{blk(nil)}}), "reps is required"},
		{"empty repeat", ph(rep(3)), "non-empty array"},
		{"phases and shorthand", ph(map[string]any{"type": "repeat", "reps": 3.0, "phases": []any{blk(nil)},
			"goal": map[string]any{"duration_s": 60.0}}), "not both (goal)"},
		{"repeat without goal", ph(map[string]any{"type": "repeat", "reps": 3.0}), "needs phases, or a goal"},
		// Polar flattens a nested repeat that ends its parent, losing its reps.
		{"nested repeat last", ph(rep(3, blk(nil), rep(2, blk(nil), blk(nil)))), "can't be the last phase"},
		{"third repeat level", ph(rep(2, rep(2, rep(2, blk(nil)), blk(nil)), blk(nil))),
			"phases[0].phases[0].phases[0] (repeat): repeats nest at most 2 deep"},
		{"21 phases", ph(leaves(21)...), "has 21 phases"},
		{"21 phases counting repeats once", ph(append(leaves(19), rep(10, blk(nil), blk(nil)))...), "has 21 phases"},
		{"expands past 200", ph(rep(67, blk(nil), blk(nil), blk(nil), blk(nil))), "expands to 268 phases"},
		{"nested expands past 200", ph(rep(20, rep(5, blk(nil), blk(nil)), blk(nil))), "expands to 220 phases"},
		{"nested phase name", ph(rep(2, blk(map[string]any{"name": strings.Repeat("n", 46)}))),
			"phases[0].phases[0] (block): phase name"},
		{"exercises mixed kinds", args(map[string]any{"exercises": []any{
			map[string]any{"sport_id": 1.0, "duration_s": 600.0},
			map[string]any{"sport_id": 2.0, "phases": []any{blk(nil)}}}}), "exercises[1] is a PHASED goal but exercises[0] is VOLUME"},
		{"exercises with sport_id", args(map[string]any{"sport_id": 1.0, "exercises": []any{
			map[string]any{"sport_id": 1.0}}}), "give either exercises or the single-sport sport_id"},
		{"exercises empty", args(map[string]any{"exercises": []any{}}), "non-empty array"},
		{"exercise bad duration", args(map[string]any{"exercises": []any{
			map[string]any{"sport_id": 1.0, "duration_s": 0.5}}}), "exercises[0].duration_s must be between 1"},
	})
}

// A UI-made nested target reads back with a repeat inside a repeat; it used to
// fail decoding because a repeat's phases were typed as leaves only.
func TestGetTrainingTarget_NestedRepeatDecodes(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/trainingtarget/42", 200, "application/json", nestedTargetJSON)
	res := callTool(t, GetTrainingTargetHandler(ff.client()), map[string]any{"target_id": 42.0})
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(res))
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(raw), `"repeatCount":2`) || !strings.Contains(string(raw), `"name":"C"`) {
		t.Fatalf("nested repeat missing from result: %s", raw)
	}
}

// Polar's update keeps the sports it is not sent and ignores new ones, so a
// sport count that differs from the stored target is refused up front.
func TestUpdateTrainingTarget_SportCountMismatch(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/trainingtarget/42", 200, "application/json", multiSportTargetJSON)
	res := callTool(t, UpdateTrainingTargetHandler(ff.client()), map[string]any{
		"target_id": 42.0, "name": "brick", "date": "2026-12-03", "time": "08:00", "duration_s": 1800.0,
	})
	if !res.IsError || !strings.Contains(resultText(res), "has 2 sport(s) but the update gives 1") {
		t.Fatalf("result = %s", resultText(res))
	}
	if w := ff.writes(); len(w) != 0 {
		t.Fatalf("writes = %+v, want none", w)
	}
}

// With every sport given, the update carries each stored exercise-target id
// over by position.
func TestUpdateTrainingTarget_MultiSportKeepsIDs(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/trainingtarget/42", 200, "application/json", multiSportTargetJSON)
	ff.on("POST", "/api/trainingtarget/42", 200, textPlain, "")
	res := callTool(t, UpdateTrainingTargetHandler(ff.client()), map[string]any{
		"target_id": 42.0, "name": "brick", "date": "2026-12-03", "time": "08:00",
		"exercises": []any{
			map[string]any{"sport_id": 1.0, "duration_s": 1800.0},
			map[string]any{"sport_id": 2.0, "duration_s": 3600.0},
		},
	})
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(res))
	}
	var body struct {
		ExerciseTargets []struct {
			ID      float64 `json:"id"`
			SportID int     `json:"sportId"`
		} `json:"exerciseTargets"`
	}
	if err := json.Unmarshal([]byte(ff.onlyWrite().Body), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.ExerciseTargets) != 2 || body.ExerciseTargets[0].ID != 1540697225 || body.ExerciseTargets[1].ID != 1540697226 {
		t.Fatalf("exercise targets = %+v", body.ExerciseTargets)
	}
}

// nestedTargetJSON is GET /api/trainingtarget/{id} for the nested ladder saved
// by the web editor (2026-10-13).
const nestedTargetJSON = `{"datetime":"2026-12-08T07:00","sourceProgramType":"UNKNOWN","isUserEditable":true,` +
	`"name":"pUI nested","exerciseTargets":[{"id":1540697290,"duration":"00:12:00","distance":5700.0,"calories":null,` +
	`"index":0,"sportId":1,"phases":[` +
	`{"id":2931531538,"lowerZone":0,"upperZone":0,"intensityType":"NONE","phaseChangeType":"MANUAL","goalType":"DURATION","duration":"00:05:00","name":"WU","phaseType":"PHASE"},` +
	`{"repeatCount":3,"phases":[` +
	`{"id":2931531539,"lowerZone":4,"upperZone":5,"intensityType":"HEART_RATE_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DISTANCE","duration":"00:00:00","distance":1000.0,"name":"A","phaseType":"PHASE"},` +
	`{"repeatCount":2,"phases":[` +
	`{"id":2931531540,"lowerZone":3,"upperZone":4,"intensityType":"SPEED_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DURATION","duration":"00:00:30","name":"B","phaseType":"PHASE"},` +
	`{"id":2931531541,"lowerZone":0,"upperZone":0,"intensityType":"NONE","phaseChangeType":"AUTOMATIC","goalType":"DISTANCE","duration":"00:00:00","distance":200.0,"name":"C","phaseType":"PHASE"}` +
	`],"phaseType":"REPEAT"},` +
	`{"id":2931531542,"lowerZone":1,"upperZone":1,"intensityType":"HEART_RATE_ZONES","phaseChangeType":"MANUAL","goalType":"DURATION","duration":"00:02:00","name":"D","phaseType":"PHASE"}` +
	`],"phaseType":"REPEAT"},` +
	`{"id":2931531543,"lowerZone":1,"upperZone":2,"intensityType":"POWER_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DISTANCE","duration":"00:00:00","distance":1500.0,"name":"CD","phaseType":"PHASE"}` +
	`]}],"description":null,"type":"PHASED"}`

// multiSportTargetJSON is a two-sport VOLUME target read-back (2026-10-13).
const multiSportTargetJSON = `{"datetime":"2026-12-03T08:00","sourceProgramType":"UNKNOWN","isUserEditable":true,` +
	`"name":"brick","exerciseTargets":[` +
	`{"id":1540697225,"duration":"00:30:00","distance":null,"calories":null,"index":0,"sportId":1,"phases":[]},` +
	`{"id":1540697226,"duration":null,"distance":20000.0,"calories":null,"index":1,"sportId":2,"phases":[]}` +
	`],"description":null,"type":"VOLUME"}`
