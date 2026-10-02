package mcp

import (
	"encoding/json"
	"testing"
)

// weekSummaryCapture is the live getCalendarWeekSummary body for 25–31 May
// 2026 holding one manual 30-min / 250 kcal run (captured 2026-10-02).
const weekSummaryCapture = `[{"distance":0.0,"calories":250,"durationMillis":1800000,"exeCount":0,"traininigSessionsCount":1,"activeTime":0,"inActiveTime":0,"sleepTime":0,"sleepQuality":0.0,"stepCount":0,"distanceFromSteps":0.0,"inactivityAlertCount":0,"sleepPlus":false,"week":22}]`

func TestGetCalendarWeekSummary_MapsCapturedWeek(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("POST", "/training/getCalendarWeekSummary", 200, "application/json", weekSummaryCapture)
	res := callTool(t, GetCalendarWeekSummaryHandler(ff.client()), map[string]any{"from_date": "2026-05-20", "to_date": "2026-06-10"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	reqs := ff.requests()
	if len(reqs) != 1 || reqs[0].XRequestedWith != "XMLHttpRequest" {
		t.Fatalf("requests = %+v", reqs)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(reqs[0].Body), &body); err != nil || body["from"] != "20.5.2026" || body["to"] != "10.6.2026" {
		t.Fatalf("request body = %s (%v)", reqs[0].Body, err)
	}
	weeks, _ := widgetPayload(t, res, "week_summary")["weeks"].([]any)
	if len(weeks) != 1 {
		t.Fatalf("weeks = %v", weeks)
	}
	w, ok := weeks[0].(map[string]any)
	if !ok {
		t.Fatalf("week = %T", weeks[0])
	}
	want := map[string]any{"week": 22.0, "week_start": "2026-05-25", "number_of_sessions": 1.0, "total_duration_s": 1800.0, "total_distance_m": 0.0, "total_kcal": 250.0}
	for k, v := range want {
		if w[k] != v {
			t.Errorf("%s = %v, want %v", k, w[k], v)
		}
	}
	if _, ok := w["step_count"]; ok {
		t.Errorf("step_count should be omitted when 0: %v", w)
	}
}
