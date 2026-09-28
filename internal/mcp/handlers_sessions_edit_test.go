package mcp

import (
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// Captured summary of a manual session (anonymised).
const manualSummary = `{"id":8429796771,"nextTrainingId":-1,"previousTrainingId":-1,"userId":11111111,"feeling":null,"note":"probe","latitude":null,"longitude":null,"startDate":"2026-09-20T08:00:00","stopTime":"2026-09-20T08:30:00","duration":"PT30M","distance":null,"kiloCalories":300,"trainingLoad":null,"trainingBenefit":null,"carboPercentage":null,"fatPercentage":null,"proteinPercentage":null,"hrMax":0,"hrAverage":140,"timezoneOffset":null,"trainingLoadPro":null,"exercises":{"8459583326":{"id":8459583326,"sport":{"id":1},"startTime":"2026-09-20T08:00:00","stopTime":"2026-09-20T08:30:00","duration":"PT30M","distance":5000.0,"calories":300,"trainingStatistic":{"HEART_RATE":{"max":0.0,"avg":140.0,"min":null}},"sportParent":"RUNNING","defaultSpeedViewSettings":"PACE"}},"periodDataUuid":null,"deviceName":"","trainingSessionName":"probe-edit-session"}`

const sessionPath = "/api/training/analysis/8429796771/summary"

func TestDeleteTrainingSession(t *testing.T) {
	t.Run("checks, deletes with trailing slash, verifies", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", sessionPath, 200, "application/json", manualSummary)
		ff.on("GET", sessionPath, 404, "", "")
		ff.on("DELETE", "/api/training/deleteTrainingSession/8429796771/", 200, "", "")
		res := callTool(t, DeleteTrainingSessionHandler(ff.client()), map[string]any{"session_id": 8429796771.0})
		if res.IsError || !strings.Contains(resultText(res), "Deleted training session 8429796771") {
			t.Fatalf("got %q", resultText(res))
		}
		if w := ff.onlyWrite(); w.XRequestedWith != "XMLHttpRequest" {
			t.Fatalf("missing X-Requested-With")
		}
	})
	t.Run("unknown id is not deleted (Flow would answer 200 anyway)", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/training/analysis/1/summary", 404, "", "")
		res := callTool(t, DeleteTrainingSessionHandler(ff.client()), map[string]any{"session_id": 1.0})
		if res.IsError || !strings.Contains(resultText(res), "No session with id 1") || len(ff.writes()) != 0 {
			t.Fatalf("got %q writes=%v", resultText(res), ff.writes())
		}
	})
	t.Run("another account's session", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/training/analysis/8348897000/summary", 403, "", "")
		res := callTool(t, DeleteTrainingSessionHandler(ff.client()), map[string]any{"session_id": 8348897000.0})
		if !res.IsError || !strings.Contains(resultText(res), "another Polar account") || len(ff.writes()) != 0 {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("still there after delete", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", sessionPath, 200, "application/json", manualSummary)
		ff.on("DELETE", "/api/training/deleteTrainingSession/8429796771/", 200, "", "")
		res := callTool(t, DeleteTrainingSessionHandler(ff.client()), map[string]any{"session_id": 8429796771.0})
		if !res.IsError || !strings.Contains(resultText(res), "still exists") {
			t.Fatalf("got %q", resultText(res))
		}
	})
	runValidationCases(t, DeleteTrainingSessionHandler, []validationCase{
		{"missing", map[string]any{}, "session_id is required"},
		{"negative", map[string]any{"session_id": -5.0}, "positive integer id"},
		{"string", map[string]any{"session_id": "abc"}, "got a string"},
		{"fractional", map[string]any{"session_id": 12.5}, "whole number"},
	})
}

func TestEditTrainingSession_Validation(t *testing.T) {
	id := 8429796771.0
	with := func(k string, v any) map[string]any { return map[string]any{"session_id": id, k: v} }
	runValidationCases(t, EditTrainingSessionHandler, []validationCase{
		{"nothing to change", map[string]any{"session_id": id}, "nothing to change"},
		{"session_id missing", map[string]any{"note": "x"}, "session_id is required"},
		{"name blank", with("name", "  "), "name must not be empty"},
		{"name 101", with("name", strings.Repeat("n", 101)), "101 characters; Polar's limit is 100"},
		{"note 10001", with("note", strings.Repeat("z", 10001)), "limit is 10000"},
		{"note wrong type", with("note", 5.0), "note must be a string"},
		{"feeling 0", with("feeling", 0.0), "feeling must be between 1 and 5"},
		{"feeling 6", with("feeling", 6.0), "feeling must be between 1 and 5"},
		{"feeling wire value", with("feeling", 0.19), "whole number"},
		{"feeling label", with("feeling", "great"), "feeling must be a number"},
		{"duration 0", with("duration_s", 0.0), "duration_s must be between 1 and 359999"},
		{"duration 100h", with("duration_s", 360000.0), "between 1 and 359999"},
		{"duration clock string", with("duration_s", "00:30:00"), "must be a number"},
		{"distance negative", with("distance_m", -1.0), "distance_m must be between 0"},
		{"distance too far", with("distance_m", 9999001.0), "distance_m must be between 0"},
		{"hr_avg 241", with("hr_avg", 241.0), "hr_avg must be between 0 and 240"},
		{"hr_avg negative", with("hr_avg", -1.0), "hr_avg must be between 0 and 240"},
		{"hr_max 241", with("hr_max", 241.0), "hr_max must be between 0 and 240"},
		{"kcal 65536", with("kcal", 65536.0), "kcal must be between 0 and 65535"},
		{"speed 400", with("speed_kmh", 400.0), "speed_kmh must be between 0 and 399"},
		{"speed negative", with("speed_kmh", -1.0), "speed_kmh must be between 0 and 399"},
		{"sport 0", with("sport_id", 0.0), "sport_id must be between 1"},
	})
}

func TestEditTrainingSession_UnknownSport(t *testing.T) {
	ff := newFakeFlow(t)
	res := callTool(t, EditTrainingSessionHandler(ff.client()), map[string]any{"session_id": 1.0, "sport_id": 21.0})
	if !res.IsError || !strings.Contains(resultText(res), "sport_id 21 is not a Polar sport") || len(ff.requests()) != 0 {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestEditTrainingSession_NoteAndFeelingUsePartialEndpoint(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", sessionPath, 200, "application/json", manualSummary)
	ff.on("PUT", "/api/training/analysis/updateTrainingData/8429796771", 200, "", "")
	res := callTool(t, EditTrainingSessionHandler(ff.client()), map[string]any{
		"session_id": 8429796771.0, "note": "inline note edit", "feeling": 5.0,
	})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	w := ff.onlyWrite()
	if w.Method != "PUT" || w.XRequestedWith != "XMLHttpRequest" {
		t.Fatalf("write = %+v", w)
	}
	assertJSONEqual(t, w.Body, `{"note":"inline note edit","feeling":"0.19"}`)
}

// A full edit sends every field, starting from the live values — exactly the
// form the web UI submits (minus its synthesized hrSamples).
func TestEditTrainingSession_FullFormBody(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "rename + note (the captured UI edit)",
			args: map[string]any{"name": "probe-edit-renamed", "note": "edited note", "distance_m": 5000.0},
			want: `{"sport":1,"duration":1800,"distance":5000,"hrAverage":140,"note":"edited note","trainingSessionName":"probe-edit-renamed","hrMax":0,"kiloCalories":300,"editedExerciseId":null}`,
		},
		{
			name: "every field",
			args: map[string]any{"sport_id": 2.0, "duration_s": 3600.0, "distance_m": 30000.5, "hr_avg": 150.0, "hr_max": 175.0,
				"kcal": 900.0, "speed_kmh": 30.0, "feeling": 1.0},
			want: `{"sport":2,"duration":3600,"distance":30000.5,"hrAverage":150,"hrMax":175,"kiloCalories":900,"speedAverage":30,"feeling":"0.99","note":"probe","trainingSessionName":"probe-edit-session","editedExerciseId":null}`,
		},
		{
			name: "clear heart rates",
			args: map[string]any{"hr_avg": 0.0},
			want: `{"sport":1,"duration":1800,"distance":5000,"hrAverage":0,"hrMax":0,"kiloCalories":300,"note":"probe","trainingSessionName":"probe-edit-session","editedExerciseId":null}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			ff.on("GET", sessionPath, 200, "application/json", manualSummary)
			ff.on("PUT", "/api/training/editTraining/8429796771", 200, "", "")
			args := map[string]any{"session_id": 8429796771.0}
			for k, v := range tt.args {
				args[k] = v
			}
			res := callTool(t, EditTrainingSessionHandler(ff.client()), args)
			if res.IsError {
				t.Fatalf("error: %s", resultText(res))
			}
			assertJSONEqual(t, ff.onlyWrite().Body, tt.want)
		})
	}
}

func TestEditTrainingSession_Refusals(t *testing.T) {
	tests := []struct {
		name, summary string
		args          map[string]any
		wantErr       string
	}{
		{
			name:    "device-recorded session: full edit refused",
			summary: strings.Replace(manualSummary, `"deviceName":""`, `"deviceName":"Polar Pacer Pro"`, 1),
			args:    map[string]any{"duration_s": 1900.0},
			wantErr: "only single-exercise, manually entered sessions",
		},
		{
			name:    "hr_max below the (live) hr_avg",
			summary: manualSummary,
			args:    map[string]any{"hr_max": 120.0},
			wantErr: "hr_max (120 bpm) is below hr_avg (140 bpm)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			ff.on("GET", sessionPath, 200, "application/json", tt.summary)
			args := map[string]any{"session_id": 8429796771.0}
			for k, v := range tt.args {
				args[k] = v
			}
			res := callTool(t, EditTrainingSessionHandler(ff.client()), args)
			if !res.IsError || !strings.Contains(resultText(res), tt.wantErr) || len(ff.writes()) != 0 {
				t.Fatalf("got %q writes=%v", resultText(res), ff.writes())
			}
		})
	}
	t.Run("device-recorded session: note still allowed", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", sessionPath, 200, "application/json", strings.Replace(manualSummary, `"deviceName":""`, `"deviceName":"Polar Pacer Pro"`, 1))
		ff.on("PUT", "/api/training/analysis/updateTrainingData/8429796771", 200, "", "")
		res := callTool(t, EditTrainingSessionHandler(ff.client()), map[string]any{"session_id": 8429796771.0, "note": ""})
		if res.IsError {
			t.Fatalf("error: %s", resultText(res))
		}
		assertJSONEqual(t, ff.onlyWrite().Body, `{"note":""}`)
	})
	t.Run("opaque 500 explained", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", sessionPath, 200, "application/json", manualSummary)
		ff.on("PUT", "/api/training/editTraining/8429796771", 500, "", "")
		res := callTool(t, EditTrainingSessionHandler(ff.client()), map[string]any{"session_id": 8429796771.0, "kcal": 10.0})
		if !res.IsError || !strings.Contains(resultText(res), "rejected a field value") {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("unknown session", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/training/analysis/1/summary", 404, "", "")
		res := callTool(t, EditTrainingSessionHandler(ff.client()), map[string]any{"session_id": 1.0, "note": "x"})
		if res.IsError || !strings.Contains(resultText(res), "No session with id 1") || len(ff.writes()) != 0 {
			t.Fatalf("got %q", resultText(res))
		}
	})
}

// With a client that can answer, the delete tools stop and ask before
// deleting, naming what would be deleted; declining leaves it in place.
func TestDeleteTools_AskFirst(t *testing.T) {
	tests := []struct {
		name      string
		mk        func(*fakeFlow) toolHandler
		args      map[string]any
		wantInMsg string
	}{
		{
			name: "delete_training_session",
			mk: func(ff *fakeFlow) toolHandler {
				ff.on("GET", sessionPath, 200, "application/json", manualSummary)
				return DeleteTrainingSessionHandler(ff.client())
			},
			args:      map[string]any{"session_id": 8429796771.0},
			wantInMsg: `"probe-edit-session" on 2026-09-20T08:00:00 (30 min) (id 8429796771)`,
		},
		{
			name: "delete_favorite",
			mk: func(ff *fakeFlow) toolHandler {
				ff.on("GET", "/api/favoritetarget/5", 200, "application/json", favRouteJSON)
				return DeleteFavoriteHandler(ff.client())
			},
			args:      map[string]any{"favorite_id": 5.0},
			wantInMsg: `route favorite "probe-route" (id 5)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			res, err := tt.mk(ff)(legacyCtx(true), req(tt.args))
			if err != nil || res == nil || !res.NeedsInput() {
				t.Fatalf("want an input request, got %+v (err %v)", res, err)
			}
			if m := res.InputRequests[confirmID].Elicitation.Message; !strings.Contains(m, tt.wantInMsg) {
				t.Fatalf("prompt = %q, want it to mention %q", m, tt.wantInMsg)
			}
			if w := ff.writes(); len(w) != 0 {
				t.Fatalf("deleted before confirmation: %+v", w)
			}

			declined := req(tt.args)
			declined.Params.InputResponses = mcpgo.InputResponses{
				confirmID: mcpgo.NewElicitationInputResponse(mcpgo.ElicitationResult{
					ElicitationResponse: mcpgo.ElicitationResponse{Action: mcpgo.ElicitationResponseActionDecline}}),
			}
			res, _ = tt.mk(ff)(legacyCtx(true), declined)
			if !strings.Contains(resultText(res), "did not confirm") || len(ff.writes()) != 0 {
				t.Fatalf("decline: %q writes=%v", resultText(res), ff.writes())
			}
		})
	}
}
