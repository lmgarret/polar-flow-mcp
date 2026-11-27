package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

type trackPayload struct {
	Type           string  `json:"type"`
	SessionID      int64   `json:"session_id"`
	ExerciseID     int64   `json:"exercise_id"`
	Start          string  `json:"start"`
	DurationS      *int    `json:"duration_s"`
	PointCount     int     `json:"point_count"`
	ReturnedPoints int     `json:"returned_points"`
	DistanceM      float64 `json:"distance_m"`
	Points         []struct {
		TS        int      `json:"t_s"`
		Lat       float64  `json:"lat"`
		Lon       float64  `json:"lon"`
		AltitudeM *float64 `json:"altitude_m"`
	} `json:"points"`
}

func callTrack(t *testing.T, ff *fakeFlow, args map[string]any) (trackPayload, string, bool) {
	t.Helper()
	res := callTool(t, GetSessionTrackHandler(ff.client()), args)
	text := resultText(res)
	var p trackPayload
	if !res.IsError && strings.HasPrefix(text, "{") {
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, text)
		}
	}
	return p, text, res.IsError
}

//nolint:gocyclo // one sequential scenario
func TestGetSessionTrack(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", detailsPath, 200, "application/json", detailsFixture(t))
	p, text, isErr := callTrack(t, ff, map[string]any{"session_id": float64(9000000001)})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	if p.Type != "session_track" || p.SessionID != 9000000001 || p.ExerciseID != 9000000101 {
		t.Fatalf("header = %+v, want session_track of exercise 9000000101", p)
	}
	if p.Start != "2026-01-15T17:20:00" || p.DurationS == nil || *p.DurationS != 3683 {
		t.Errorf("start/duration = %q/%v, want 2026-01-15T17:20:00 / 3683", p.Start, p.DurationS)
	}
	if p.PointCount != 121 || p.ReturnedPoints != 121 || len(p.Points) != 121 {
		t.Fatalf("counts = %d/%d, want all 121 fixes", p.PointCount, p.ReturnedPoints)
	}
	if p.Points[0].TS != 4 || p.Points[0].Lat != 48.838001 || p.Points[0].AltitudeM == nil {
		t.Errorf("point 0 = %+v, want the first fix at 4 s", p.Points[0])
	}
	if p.DistanceM <= 0 {
		t.Errorf("distance_m = %v, want a positive cumulation", p.DistanceM)
	}

	thin, _, isErr := callTrack(t, ff, map[string]any{"session_id": float64(9000000001), "max_points": float64(10)})
	if isErr || thin.PointCount != 121 || thin.ReturnedPoints != 10 || len(thin.Points) != 10 {
		t.Fatalf("max_points 10 = %d/%d (error %v), want 10 of 121", thin.ReturnedPoints, thin.PointCount, isErr)
	}
	if thin.Points[9].TS != 3658 {
		t.Errorf("last returned fix t = %d, want the last fix kept", thin.Points[9].TS)
	}
}

func TestGetSessionTrackNoGPS(t *testing.T) {
	manual := `{"id":5,"exercises":{"6":{"id":6,"sport":{"id":1},"duration":"PT30M"}},
		"samples":{"6":{"HEART_RATE":null,"COORDINATE":null}},"zones":{"6":{}},"exerciseResultTargetData":{}}`
	ff := newFakeFlow(t)
	ff.on("GET", "/api/training/analysis/5/details", 200, "application/json", manual)
	_, text, isErr := callTrack(t, ff, map[string]any{"session_id": float64(5)})
	if isErr || !strings.Contains(text, "no GPS track") {
		t.Fatalf("manual session = %q (error %v), want a no-GPS notice", text, isErr)
	}

	ff.on("GET", "/api/training/analysis/404/details", 404, "text/plain", "")
	if _, text, _ := callTrack(t, ff, map[string]any{"session_id": float64(404)}); !strings.Contains(text, "No session with id 404") {
		t.Errorf("unknown session = %q", text)
	}

	ff.on("GET", detailsPath, 200, "application/json", detailsFixture(t))
	_, text, isErr = callTrack(t, ff, map[string]any{"session_id": float64(9000000001), "exercise_id": float64(7)})
	if !isErr || !strings.Contains(text, "not part of session") {
		t.Errorf("unknown exercise = %q (error %v), want an error naming the session", text, isErr)
	}
}

func TestGetSessionTrackValidation(t *testing.T) {
	runValidationCases(t, func(c *flow.Client) toolHandler { return GetSessionTrackHandler(c) }, []validationCase{
		{"missing id", map[string]any{}, "session_id is required"},
		{"zero exercise", map[string]any{"session_id": float64(1), "exercise_id": float64(0)}, "exercise_id must be between 1"},
		{"fractional max_points", map[string]any{"session_id": float64(1), "max_points": 2.5}, "whole number"},
		{"one max_point", map[string]any{"session_id": float64(1), "max_points": float64(1)}, "max_points must be between 2 and 100000"},
	})
}
