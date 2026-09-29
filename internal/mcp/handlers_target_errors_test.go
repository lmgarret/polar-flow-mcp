package mcp

import (
	"context"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// Probed limits (2026-09-29): name ≤ 45, description ≤ 500, phase name ≤ 45,
// all in UTF-16 code units. An over-long phase name is the dangerous one — Flow
// 400s but still stores a half-created target that holds the time slot — so
// none of these may reach the server.
func TestCreateTrainingTarget_Validation(t *testing.T) {
	base := func(extra map[string]any) map[string]any {
		m := map[string]any{"name": "ok", "date": "2026-12-01", "time": "07:00", "duration_s": 1800.0}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	long46 := strings.Repeat("n", 46)
	emoji45 := strings.Repeat("a", 44) + "🏃" // 45 runes, 46 UTF-16 units
	phases := func(p map[string]any) []any { return []any{p} }
	runValidationCases(t, CreateTrainingTargetHandler, []validationCase{
		{"name missing", map[string]any{"date": "2026-12-01", "duration_s": 1800.0}, "name is required"},
		{"name blank", base(map[string]any{"name": "  "}), "must not be empty"},
		{"name 46", base(map[string]any{"name": long46}), "limit is 45"},
		{"name emoji counts 2", base(map[string]any{"name": emoji45}), "is 46 characters"},
		{"description 501", base(map[string]any{"description": strings.Repeat("d", 501)}), "limit is 500"},
		{"warmup name 46", base(map[string]any{"phases": phases(map[string]any{"type": "warmup", "name": long46, "duration_s": 600.0})}),
			"phases[0] (warmup): phase name"},
		{"recovery name 46", base(map[string]any{"phases": phases(map[string]any{
			"type": "repeat", "reps": 3.0, "goal": map[string]any{"duration_s": 60.0},
			"recovery": map[string]any{"duration_s": 60.0, "name": long46}})}), "phases[0] (repeat): phase name"},
	})
}

const clashBody = `{"time": ["error.trainingTarget.twoTargetsForSameTime"]}`

// Flow serves the time clash as text/plain; it used to fail decoding.
func TestCreateTrainingTarget_TimeClash(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("POST", "/api/trainingtarget", 400, textPlain, clashBody)
	res := callTool(t, CreateTrainingTargetHandler(ff.client()), map[string]any{
		"name": "ok", "date": "2026-12-01", "time": "07:00", "duration_s": 1800.0,
	})
	if text := resultText(res); !res.IsError || !strings.Contains(text, "already has a training target at 2026-12-01T07:00") {
		t.Fatalf("got %q", text)
	}
}

func TestUpdateTrainingTarget_TimeClash(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/trainingtarget/5", 200, "application/json", targetJSON)
	ff.on("POST", "/api/trainingtarget/5", 400, textPlain, clashBody)
	res := callTool(t, UpdateTrainingTargetHandler(ff.client()), map[string]any{
		"target_id": 5.0, "name": "ok", "date": "2026-12-01", "time": "07:00", "duration_s": 1800.0,
	})
	if text := resultText(res); !res.IsError || !strings.Contains(text, "already has a training target") {
		t.Fatalf("got %q", text)
	}
}

// Flow's delete answers a missing and a foreign id alike (400 text/plain), so
// the handler reads first and never sends the delete for either.
func TestDeleteTrainingTarget(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/trainingtarget/7", 404, textPlain, "trainingTarget.targetNotFound")
		res := callTool(t, DeleteTrainingTargetHandler(ff.client()), map[string]any{"target_id": 7.0})
		if res.IsError || resultText(res) != "No target with id 7." || len(ff.writes()) != 0 {
			t.Fatalf("got %q, writes %v", resultText(res), ff.writes())
		}
	})
	t.Run("foreign", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/trainingtarget/7", 403, textPlain, "error.invalid.target")
		res := callTool(t, DeleteTrainingTargetHandler(ff.client()), map[string]any{"target_id": 7.0})
		if !res.IsError || !strings.Contains(resultText(res), "another Polar account") || len(ff.writes()) != 0 {
			t.Fatalf("got %q, writes %v", resultText(res), ff.writes())
		}
	})
	t.Run("deleted", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/trainingtarget/7", 200, "application/json", targetJSON)
		ff.on("DELETE", "/training/target/7", 200, "", "")
		res := callTool(t, DeleteTrainingTargetHandler(ff.client()), map[string]any{"target_id": 7.0})
		if res.IsError || resultText(res) != "Deleted target 7." {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("gone between read and delete", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/trainingtarget/7", 200, "application/json", targetJSON)
		ff.on("DELETE", "/training/target/7", 400, textPlain, "deleteError")
		res := callTool(t, DeleteTrainingTargetHandler(ff.client()), map[string]any{"target_id": 7.0})
		if res.IsError || resultText(res) != "No target with id 7." {
			t.Fatalf("got %q", resultText(res))
		}
	})
}

// A device-paired account does not send supportedDeviceIds as an int array;
// the field is carried opaquely so the listing (and import_route, which lists
// before and after the upload) keeps working whatever its shape.
func TestListFavorites_SupportedDeviceIdsShapes(t *testing.T) {
	for _, ids := range []string{`[]`, `["A1B2C3D4"]`, `"A1B2C3D4"`, `[12345]`} {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favorites", 200, "application/json", strings.Replace(favoritesBefore, `"supportedDeviceIds":[]`, `"supportedDeviceIds":`+ids, 1))
		res := callTool(t, ListFavoritesHandler(ff.client()), map[string]any{})
		if res.IsError {
			t.Fatalf("supportedDeviceIds=%s: %s", ids, resultText(res))
		}
	}
}

// UI-bound tools advertise an outputSchema, so every non-error result must
// carry structuredContent — including plain statements like "No target".
func TestWithLogging_BackfillsStructuredContent(t *testing.T) {
	h := withLogging("t", func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return mcpgo.NewToolResultText("No target with id 7."), nil
	})
	res, _ := h(context.Background(), req(nil))
	sc, _ := res.StructuredContent.(map[string]any)
	if sc["type"] != "notice" || sc["kind"] != "notice" || sc["message"] != "No target with id 7." {
		t.Fatalf("structuredContent = %#v", res.StructuredContent)
	}

	errH := withLogging("t", func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return mcpgo.NewToolResultError("boom"), nil
	})
	if res, _ := errH(context.Background(), req(nil)); res.StructuredContent != nil {
		t.Fatalf("error result got structuredContent %#v", res.StructuredContent)
	}
}
