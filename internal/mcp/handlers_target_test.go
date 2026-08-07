package mcp

import (
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

func req(args map[string]any) mcpgo.CallToolRequest {
	return mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Arguments: args}}
}

// A PHASED target with a DISTANCE-goal repeat is the exact shape that the update
// round-trip mishandled. The wire body must key the goal off goalType: a DISTANCE
// phase carries distance only, never a (phantom) duration.
func TestBuildTrainingTargetCreate_DistancePhaseGoalDiscrimination(t *testing.T) {
	body, errMsg := buildTrainingTargetCreate(req(map[string]any{
		"name":     "5x1km Threshold",
		"date":     "2026-06-10",
		"time":     "09:00",
		"sport_id": float64(1),
		"phases": []any{
			map[string]any{"type": "warmup", "duration_s": float64(600)},
			map[string]any{
				"type": "repeat", "reps": float64(5),
				"goal":      map[string]any{"distance_m": float64(1000)},
				"intensity": map[string]any{"label": "threshold"},
				"recovery":  map[string]any{"duration_s": float64(120)},
			},
			map[string]any{"type": "cooldown", "duration_s": float64(600)},
		},
	}))
	if errMsg != "" {
		t.Fatalf("unexpected build error: %s", errMsg)
	}
	if body.Type != gen.TrainingTargetCreateTypePHASED {
		t.Fatalf("type = %q, want PHASED", body.Type)
	}
	if len(body.ExerciseTargets) != 1 {
		t.Fatalf("exerciseTargets = %d, want 1", len(body.ExerciseTargets))
	}
	phases := body.ExerciseTargets[0].Phases
	if len(phases) != 3 {
		t.Fatalf("phases = %d, want 3 (warmup, repeat, cooldown)", len(phases))
	}
	if phases[1].Type != gen.PhaseRepeatPhase {
		t.Fatalf("phases[1] type = %q, want REPEAT", phases[1].Type)
	}
	work := phases[1].PhaseRepeat.Phases[0]
	if string(work.GoalType) != "DISTANCE" {
		t.Fatalf("work goalType = %q, want DISTANCE", work.GoalType)
	}
	if d, ok := work.Distance.Get(); !ok || d != 1000 {
		t.Fatalf("work distance = (%v, ok=%v), want (1000, true)", d, ok)
	}
	if _, ok := work.Duration.Get(); ok {
		t.Fatalf("work duration must be unset on a DISTANCE phase, got it set")
	}
}

// Per-phase names must reach the wire body verbatim, with the type-derived
// label (Warm-up / Work / Recovery / Cool-down) used only as a fallback when
// the caller omits name.
func TestBuildTrainingTargetCreate_PhaseNames(t *testing.T) {
	body, errMsg := buildTrainingTargetCreate(req(map[string]any{
		"name":     "Named phases",
		"date":     "2026-06-10",
		"time":     "09:00",
		"sport_id": float64(1),
		"phases": []any{
			map[string]any{"type": "warmup", "name": "Easy jog", "duration_s": float64(600)},
			map[string]any{
				"type": "repeat", "reps": float64(5), "name": "Hard 1k",
				"goal":     map[string]any{"distance_m": float64(1000)},
				"recovery": map[string]any{"duration_s": float64(120), "name": "Float"},
			},
			// No name → falls back to the default "Cool-down".
			map[string]any{"type": "cooldown", "duration_s": float64(600)},
		},
	}))
	if errMsg != "" {
		t.Fatalf("unexpected build error: %s", errMsg)
	}
	phases := body.ExerciseTargets[0].Phases
	if got := phases[0].PhaseLeaf.Name; got != "Easy jog" {
		t.Fatalf("warmup name = %q, want %q", got, "Easy jog")
	}
	work := phases[1].PhaseRepeat.Phases[0]
	if work.Name != "Hard 1k" {
		t.Fatalf("work name = %q, want %q", work.Name, "Hard 1k")
	}
	recovery := phases[1].PhaseRepeat.Phases[1]
	if recovery.Name != "Float" {
		t.Fatalf("recovery name = %q, want %q", recovery.Name, "Float")
	}
	if got := phases[2].PhaseLeaf.Name; got != "Cool-down" {
		t.Fatalf("cooldown name = %q, want default %q", got, "Cool-down")
	}
}

// Power and speed intensity share the HR-zone wire representation
// (lowerZone/upperZone as a 1-5 zone index, not a raw watts/km-h threshold), so
// power_zone / speed_zone must set intensityType and both zone bounds exactly
// like hr_zone does, and precedence must run hr_zone > power_zone > speed_zone.
func TestBuildTrainingTargetCreate_PowerAndSpeedIntensity(t *testing.T) {
	body, errMsg := buildTrainingTargetCreate(req(map[string]any{
		"name":     "Bike intervals",
		"date":     "2026-06-10",
		"time":     "09:00",
		"sport_id": float64(2),
		"phases": []any{
			map[string]any{
				"type": "repeat", "reps": float64(4),
				"goal":      map[string]any{"duration_s": float64(180)},
				"intensity": map[string]any{"power_zone": float64(4)},
			},
			map[string]any{
				"type": "repeat", "reps": float64(3),
				"goal":      map[string]any{"duration_s": float64(120)},
				"intensity": map[string]any{"speed_zone": float64(5)},
			},
			map[string]any{
				"type": "repeat", "reps": float64(2),
				"goal":      map[string]any{"duration_s": float64(60)},
				"intensity": map[string]any{"hr_zone": float64(3), "power_zone": float64(5)},
			},
		},
	}))
	if errMsg != "" {
		t.Fatalf("unexpected build error: %s", errMsg)
	}
	phases := body.ExerciseTargets[0].Phases

	power := phases[0].PhaseRepeat.Phases[0]
	if string(power.IntensityType) != "POWER_ZONES" {
		t.Fatalf("power work intensityType = %q, want POWER_ZONES", power.IntensityType)
	}
	if lo, ok := power.LowerZone.Get(); !ok || lo != 4 {
		t.Fatalf("power work lowerZone = (%v, ok=%v), want (4, true)", lo, ok)
	}
	if hi, ok := power.UpperZone.Get(); !ok || hi != 4 {
		t.Fatalf("power work upperZone = (%v, ok=%v), want (4, true)", hi, ok)
	}

	speed := phases[1].PhaseRepeat.Phases[0]
	if string(speed.IntensityType) != "SPEED_ZONES" {
		t.Fatalf("speed work intensityType = %q, want SPEED_ZONES", speed.IntensityType)
	}
	if lo, ok := speed.LowerZone.Get(); !ok || lo != 5 {
		t.Fatalf("speed work lowerZone = (%v, ok=%v), want (5, true)", lo, ok)
	}

	precedence := phases[2].PhaseRepeat.Phases[0]
	if string(precedence.IntensityType) != "HEART_RATE_ZONES" {
		t.Fatalf("hr_zone+power_zone work intensityType = %q, want HEART_RATE_ZONES (hr_zone wins)", precedence.IntensityType)
	}
	if lo, ok := precedence.LowerZone.Get(); !ok || lo != 3 {
		t.Fatalf("hr_zone+power_zone work lowerZone = (%v, ok=%v), want (3, true)", lo, ok)
	}
}

// Regression test: intensity was previously only wired on a repeat's work
// leaf (buildWorkLeaf); a warmup/cooldown's intensity was silently dropped
// (buildSimpleLeaf always hardcoded IntensityType "NONE"), even though the
// call succeeded — the server just came back with intensityType NONE,
// lowerZone/upperZone 0. warmup/cooldown must map intensity exactly like a
// repeat's work leaf does, so a single zoned block doesn't need to be
// artificially split into a repeat.
func TestBuildTrainingTargetCreate_WarmupCooldownIntensity(t *testing.T) {
	body, errMsg := buildTrainingTargetCreate(req(map[string]any{
		"name":     "Zoned warmup and cooldown",
		"date":     "2026-06-10",
		"time":     "09:00",
		"sport_id": float64(1),
		"phases": []any{
			map[string]any{
				"type": "warmup", "duration_s": float64(600),
				"intensity": map[string]any{"hr_zone": float64(3)},
			},
			map[string]any{
				"type": "cooldown", "duration_s": float64(300),
				"intensity": map[string]any{"power_zone": float64(2)},
			},
		},
	}))
	if errMsg != "" {
		t.Fatalf("unexpected build error: %s", errMsg)
	}
	phases := body.ExerciseTargets[0].Phases

	warmup := phases[0].PhaseLeaf
	if string(warmup.IntensityType) != "HEART_RATE_ZONES" {
		t.Fatalf("warmup intensityType = %q, want HEART_RATE_ZONES", warmup.IntensityType)
	}
	if lo, ok := warmup.LowerZone.Get(); !ok || lo != 3 {
		t.Fatalf("warmup lowerZone = (%v, ok=%v), want (3, true)", lo, ok)
	}
	if hi, ok := warmup.UpperZone.Get(); !ok || hi != 3 {
		t.Fatalf("warmup upperZone = (%v, ok=%v), want (3, true)", hi, ok)
	}

	cooldown := phases[1].PhaseLeaf
	if string(cooldown.IntensityType) != "POWER_ZONES" {
		t.Fatalf("cooldown intensityType = %q, want POWER_ZONES", cooldown.IntensityType)
	}
	if lo, ok := cooldown.LowerZone.Get(); !ok || lo != 2 {
		t.Fatalf("cooldown lowerZone = (%v, ok=%v), want (2, true)", lo, ok)
	}
}

// Warmup/cooldown without an intensity object must still default to open
// intensity (NONE, null zones) — the fix must not force a zone onto every
// warmup/cooldown.
func TestBuildTrainingTargetCreate_WarmupWithoutIntensityStaysNone(t *testing.T) {
	body, errMsg := buildTrainingTargetCreate(req(map[string]any{
		"name":     "Easy warmup",
		"date":     "2026-06-10",
		"time":     "09:00",
		"sport_id": float64(1),
		"phases": []any{
			map[string]any{"type": "warmup", "duration_s": float64(600)},
		},
	}))
	if errMsg != "" {
		t.Fatalf("unexpected build error: %s", errMsg)
	}
	warmup := body.ExerciseTargets[0].Phases[0].PhaseLeaf
	if string(warmup.IntensityType) != "NONE" {
		t.Fatalf("warmup intensityType = %q, want NONE", warmup.IntensityType)
	}
	if _, ok := warmup.LowerZone.Get(); ok {
		t.Fatalf("warmup lowerZone should be unset when no intensity is given")
	}
}

func TestPreserveExerciseTargetIDs(t *testing.T) {
	var existingID gen.OptNilFloat64
	existingID.SetTo(1454144110)
	existing := []gen.ExerciseTarget{{ID: existingID}}

	body := &gen.TrainingTargetCreate{ExerciseTargets: []gen.ExerciseTarget{{SportId: 1}}}
	if _, ok := body.ExerciseTargets[0].ID.Get(); ok {
		t.Fatalf("precondition: rebuilt body should have no exerciseTarget id")
	}

	preserveExerciseTargetIDs(body, existing)

	got, ok := body.ExerciseTargets[0].ID.Get()
	if !ok || got != 1454144110 {
		t.Fatalf("exerciseTarget id = (%v, ok=%v), want (1454144110, true)", got, ok)
	}
}

func TestPreserveExerciseTargetIDs_NoExistingIsNoOp(t *testing.T) {
	body := &gen.TrainingTargetCreate{ExerciseTargets: []gen.ExerciseTarget{{SportId: 1}}}
	preserveExerciseTargetIDs(body, nil) // must not panic or set anything
	if _, ok := body.ExerciseTargets[0].ID.Get(); ok {
		t.Fatalf("id should remain unset when there is no live target")
	}
}
