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
