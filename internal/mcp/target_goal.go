package mcp

import (
	"errors"
	"fmt"
	"math"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Goal and phase-tree limits shared by training targets and favorites: the web
// editor's own rules (bundle constants) cross-checked against the server
// (probed 2026-10-13). Where the server is laxer (a 0 m phase, a "MM:SS"
// duration) or murky (its expanded-phase count), the editor's rule wins, so a
// target built here is always one the web UI can open and save.
const (
	maxRepeatDepth     = 2       // a repeat may hold one more level of repeat; a 3rd → 400
	minRepeatCount     = 2       // 1 and 0 are unwrapped by Flow
	maxRepeatCount     = 99      // 100 → 400
	maxPhaseLeaves     = 20      // leaves as written; 21 → 400
	maxExpandedPhases  = 200     // the editor's counter, repeats expanded
	maxGoalDurationS   = 359999  // 99:59:59; 100:00:00 → 400
	maxGoalDistanceM   = 9999000 // more → 400
	maxGoalKcal        = 65535   // 65536 → 400
	defaultTargetSport = 1
)

// Wire target types. Every exercise target of one target shares the type.
const (
	kindFree     = "FREE"
	kindVolume   = "VOLUME"
	kindRacePace = "STEADY_RACE_PACE"
	kindPhased   = "PHASED"
)

// buildExerciseTargets parses the goal arguments of a target or favorite into
// wire exercise targets and the target type they imply. args are the tool
// arguments; with allowMulti, an exercises array (multi-sport) replaces the
// single-sport sport_id / phases / duration_s / distance_m / kcal arguments.
func buildExerciseTargets(args map[string]any, allowMulti bool) ([]gen.ExerciseTarget, string, error) {
	raw, multi := args["exercises"]
	if !multi || raw == nil {
		et, kind, err := buildExercise(args, "")
		if err != nil {
			return nil, "", err
		}
		return []gen.ExerciseTarget{et}, kind, nil
	}
	if !allowMulti {
		return nil, "", errors.New("exercises (multi-sport) is not supported here; give one sport's goal")
	}
	for _, k := range []string{"sport_id", "phases", "duration_s", "distance_m", "kcal"} {
		if v, ok := args[k]; ok && v != nil {
			return nil, "", fmt.Errorf("give either exercises or the single-sport %s, not both", k)
		}
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, "", errors.New("exercises must be a non-empty array of {sport_id, goal} objects")
	}
	ets := make([]gen.ExerciseTarget, 0, len(list))
	kind := ""
	for i, r := range list {
		m, ok := r.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("exercises[%d] must be an object", i)
		}
		path := fmt.Sprintf("exercises[%d].", i)
		et, k, err := buildExercise(m, path)
		if err != nil {
			return nil, "", err
		}
		if kind != "" && k != kind {
			return nil, "", fmt.Errorf("exercises[%d] is a %s goal but exercises[0] is %s: Polar applies one goal "+
				"type to every sport of a target, so give them all phases, all the same volume metric, "+
				"all a race pace, or all no goal", i, k, kind)
		}
		kind = k
		ets = append(ets, et)
	}
	return ets, kind, nil
}

// buildExercise parses one sport's goal — sport_id plus one of: nothing
// (FREE), one of duration_s / distance_m / kcal (VOLUME), duration_s +
// distance_m (STEADY_RACE_PACE, kcal optional) or a phases array (PHASED, kcal
// optional). path prefixes argument names in errors ("exercises[1].").
func buildExercise(m map[string]any, path string) (gen.ExerciseTarget, string, error) {
	et := gen.ExerciseTarget{SportId: defaultTargetSport, Phases: []gen.Phase{}}
	sport, hasSport, err := mapInt(m, "sport_id", 1, 100000, "", path)
	if err != nil {
		return et, "", err
	}
	if hasSport {
		et.SportId = int(sport)
	}
	v, err := parseVolume(m, path)
	if err != nil {
		return et, "", err
	}
	if v.hasKcal {
		et.Calories.SetTo(float64(v.kcal))
	}

	if rawPhases, ok := m["phases"]; ok && rawPhases != nil {
		list, isList := rawPhases.([]any)
		if !isList {
			return et, "", fmt.Errorf("%sphases must be an array, got %s", path, jsonTypeName(rawPhases))
		}
		if len(list) > 0 {
			if v.hasDur || v.hasDist {
				return et, "", fmt.Errorf("give either %sphases or %sduration_s / %sdistance_m, not both — "+
					"a phased goal's totals come from its phases", path, path, path)
			}
			phases, err := buildPhaseTree(list, path+"phases")
			if err != nil {
				return et, "", err
			}
			et.Phases = phases
			return et, kindPhased, nil
		}
	}
	return v.apply(&et, path)
}

// volume holds the volume goal arguments of one sport.
type volume struct {
	dur, kcal                int64
	dist                     float64
	hasDur, hasDist, hasKcal bool
}

// parseVolume reads duration_s, distance_m and kcal, each in Flow's range.
func parseVolume(m map[string]any, path string) (volume, error) {
	var v volume
	var err error
	if v.dur, v.hasDur, err = mapInt(m, "duration_s", 1, maxGoalDurationS, "seconds (99:59:59)", path); err != nil {
		return v, err
	}
	if v.dist, v.hasDist, err = mapFloat(m, "distance_m", 1, maxGoalDistanceM, "metres", path); err != nil {
		return v, err
	}
	if v.kcal, v.hasKcal, err = mapInt(m, "kcal", 1, maxGoalKcal, "kcal", path); err != nil {
		return v, err
	}
	return v, nil
}

// apply sets a non-phased goal on et and returns the target type it makes:
// both duration and distance are a race pace, one metric a VOLUME goal, and
// nothing a FREE target. Flow 400s a VOLUME goal with two metrics.
func (v volume) apply(et *gen.ExerciseTarget, path string) (gen.ExerciseTarget, string, error) {
	if v.hasDur {
		et.Duration.SetTo(convert.SecondsToClock(int(v.dur)))
	}
	if v.hasDist {
		et.Distance.SetTo(v.dist)
	}
	switch {
	case v.hasDur && v.hasDist:
		return *et, kindRacePace, nil
	case v.hasKcal && (v.hasDur || v.hasDist):
		return *et, "", fmt.Errorf("%skcal combines only with duration_s + distance_m (a race pace) or with "+
			"phases; a single-metric goal takes exactly one of duration_s, distance_m or kcal", path)
	case v.hasDur || v.hasDist || v.hasKcal:
		return *et, kindVolume, nil
	}
	return *et, kindFree, nil
}

// phaseTree tallies a phase tree while it is built, for the editor's
// whole-tree limits.
type phaseTree struct {
	leaves   int
	expanded int
}

// buildPhaseTree turns the user-facing phase array into Polar Flow's
// PHASE/REPEAT tree and checks the whole-tree limits.
func buildPhaseTree(list []any, path string) ([]gen.Phase, error) {
	var t phaseTree
	phases, err := t.build(list, path, 0, 1)
	if err != nil {
		return nil, err
	}
	if t.leaves > maxPhaseLeaves {
		return nil, fmt.Errorf("%s has %d phases (repeats not expanded); Polar allows at most %d — "+
			"use a repeat instead of writing the same phase out several times", path, t.leaves, maxPhaseLeaves)
	}
	if t.expanded > maxExpandedPhases {
		return nil, fmt.Errorf("%s expands to %d phases once repeats are unrolled; Polar's editor allows "+
			"at most %d — lower some reps", path, t.expanded, maxExpandedPhases)
	}
	return phases, nil
}

// singlePhaseNames are the default names of the single-leaf phase types. Flow
// gives a leaf no role — warm-up / block / cool-down differ only in this
// fallback label — so any of them may sit anywhere in the sequence.
var singlePhaseNames = map[string]string{"warmup": "Warm-up", "block": "Block", "cooldown": "Cool-down"}

// build converts one level of phases. depth is the number of enclosing
// repeats; mult the product of their rep counts.
func (t *phaseTree) build(list []any, path string, depth, mult int) ([]gen.Phase, error) {
	out := make([]gen.Phase, 0, len(list))
	for i, raw := range list {
		p, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: not an object", path, i)
		}
		ptype, _ := p["type"].(string)
		where := fmt.Sprintf("%s[%d] (%s)", path, i, ptype)
		switch ptype {
		case "warmup", "block", "cooldown":
			leaf, err := buildLeaf(p, p, phaseName(p, singlePhaseNames[ptype]))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", where, err)
			}
			t.leaves++
			t.expanded += mult
			out = append(out, gen.Phase{Type: gen.PhaseLeafPhase, PhaseLeaf: leaf})
		case "repeat":
			repeat, err := t.buildRepeat(p, fmt.Sprintf("%s[%d]", path, i), depth+1, mult)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", where, err)
			}
			out = append(out, gen.Phase{Type: gen.PhaseRepeatPhase, PhaseRepeat: repeat})
		default:
			return nil, fmt.Errorf("%s[%d]: unknown type %q (want warmup|block|repeat|cooldown)", path, i, ptype)
		}
	}
	return out, nil
}

// buildRepeat constructs a repeat group, either from its own phases array
// (any leaves, and one more level of repeat) or from the work/recovery
// shorthand (goal + intensity for the work leaf, optional recovery).
func (t *phaseTree) buildRepeat(p map[string]any, path string, depth, mult int) (gen.PhaseRepeat, error) {
	if depth > maxRepeatDepth {
		return gen.PhaseRepeat{}, fmt.Errorf("repeats nest at most %d deep (a repeat inside a repeat); "+
			"Polar rejects a third level", maxRepeatDepth)
	}
	reps, err := wholeNumber(p, "reps", minRepeatCount, maxRepeatCount)
	if err != nil {
		return gen.PhaseRepeat{}, err
	}
	repeat := gen.PhaseRepeat{PhaseType: "REPEAT", RepeatCount: reps}
	inner, hasInner := p["phases"]
	if hasInner && inner != nil {
		for _, k := range []string{"goal", "recovery", "intensity", "manual"} {
			if _, ok := p[k]; ok {
				return repeat, fmt.Errorf("give either phases or the goal/intensity/recovery shorthand, not both (%s)", k)
			}
		}
		list, ok := inner.([]any)
		if !ok || len(list) == 0 {
			return repeat, errors.New("phases must be a non-empty array of the phases to repeat")
		}
		phases, err := t.build(list, path+".phases", depth, mult*reps)
		if err != nil {
			return repeat, err
		}
		if last := phases[len(phases)-1]; last.Type == gen.PhaseRepeatPhase {
			return repeat, errors.New("a nested repeat can't be the last phase of its repeat — Polar " +
				"flattens it and loses its reps; add a phase after it")
		}
		repeat.Phases = phases
		return repeat, nil
	}

	goal, _ := p["goal"].(map[string]any)
	if goal == nil {
		return repeat, errors.New("needs phases, or a goal (distance_m or duration_s) for the work interval")
	}
	work, err := buildLeaf(goal, p, phaseName(p, "Work"))
	if err != nil {
		return repeat, fmt.Errorf("work: %w", err)
	}
	repeat.Phases = []gen.Phase{{Type: gen.PhaseLeafPhase, PhaseLeaf: work}}
	if rec, ok := p["recovery"].(map[string]any); ok {
		leaf, err := buildLeaf(rec, rec, phaseName(rec, "Recovery"))
		if err != nil {
			return repeat, fmt.Errorf("recovery: %w", err)
		}
		repeat.Phases = append(repeat.Phases, gen.Phase{Type: gen.PhaseLeafPhase, PhaseLeaf: leaf})
	}
	t.leaves += len(repeat.Phases)
	t.expanded += mult * reps * len(repeat.Phases)
	return repeat, nil
}

// buildLeaf builds one phase leaf: its goal from goal (exactly one of
// distance_m metres or duration_s seconds — the goal decides goalType, which
// is what Flow and the read-back key on), and its name, intensity and manual
// advance from opts.
func buildLeaf(goal, opts map[string]any, name string) (gen.PhaseLeaf, error) {
	leaf := gen.PhaseLeaf{
		PhaseType:       "PHASE",
		Name:            name,
		PhaseChangeType: "AUTOMATIC",
		IntensityType:   "NONE",
	}
	if n := convert.PolarTextLen(name); n > maxPhaseNameLen {
		return leaf, fmt.Errorf("phase name %q is %d characters; Polar's limit is %d", name, n, maxPhaseNameLen)
	}
	dur, hasDur, err := mapInt(goal, "duration_s", 1, maxGoalDurationS, "seconds (99:59:59)", "")
	if err != nil {
		return leaf, err
	}
	dist, hasDist, err := mapFloat(goal, "distance_m", 1, maxGoalDistanceM, "metres", "")
	if err != nil {
		return leaf, err
	}
	switch {
	case hasDist && hasDur:
		return leaf, errors.New("give only one of distance_m or duration_s")
	case hasDist:
		leaf.GoalType = "DISTANCE"
		leaf.Distance.SetTo(dist)
	case hasDur:
		leaf.GoalType = "DURATION"
		leaf.Duration.SetTo(convert.SecondsToClock(int(dur)))
	default:
		return leaf, errors.New("needs a goal: distance_m or duration_s")
	}
	if raw, ok := opts["manual"]; ok && raw != nil {
		manual, isBool := raw.(bool)
		if !isBool {
			return leaf, fmt.Errorf("manual must be true or false, got %s", jsonTypeName(raw))
		}
		if manual {
			leaf.PhaseChangeType = "MANUAL"
		}
	}
	if intensity, ok := opts["intensity"].(map[string]any); ok {
		if err := applyIntensity(&leaf, intensity); err != nil {
			return leaf, err
		}
	}
	return leaf, nil
}

// phaseName returns a caller-supplied phase name (p["name"]) when present and
// non-empty, otherwise the type-derived fallback (e.g. "Warm-up", "Work"). The
// Polar Flow API persists this string verbatim per phase.
func phaseName(p map[string]any, fallback string) string {
	if name, ok := p["name"].(string); ok && name != "" {
		return name
	}
	return fallback
}

// wholeNumber reads a required integer in [lo, hi] from a phase object.
func wholeNumber(m map[string]any, key string, lo, hi int) (int, error) {
	v, ok, err := mapInt(m, key, int64(lo), int64(hi), "", "")
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("%s is required (%d–%d)", key, lo, hi)
	}
	return int(v), nil
}

// mapFloat reads an optional number in [lo, hi] from a nested argument object
// (a phase, goal or exercises entry); path prefixes the name in errors.
func mapFloat(m map[string]any, key string, lo, hi float64, unit, path string) (float64, bool, error) {
	raw, ok := m[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	f, isNum := raw.(float64)
	if !isNum {
		return 0, true, fmt.Errorf("%s%s must be a number, got %s", path, key, jsonTypeName(raw))
	}
	if math.IsNaN(f) || f < lo || f > hi {
		return 0, true, fmt.Errorf("%s%s must be between %v and %v%s, got %v", path, key, lo, hi, unitSuffix(unit), f)
	}
	return f, true, nil
}

// mapInt is mapFloat for whole numbers.
func mapInt(m map[string]any, key string, lo, hi int64, unit, path string) (int64, bool, error) {
	f, ok, err := mapFloat(m, key, float64(lo), float64(hi), unit, path)
	if err != nil || !ok {
		return 0, ok, err
	}
	if f != math.Trunc(f) {
		return 0, true, fmt.Errorf("%s%s must be a whole number, got %v", path, key, f)
	}
	return int64(f), true, nil
}
