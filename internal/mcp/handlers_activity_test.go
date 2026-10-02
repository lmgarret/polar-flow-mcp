package mcp

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// emptyDay is a device-less day as captured live 2026-09-30.
const emptyDay = `{"dataPanelData":{"dailyActivityGoal":0,"activeTime":0,"distanceFromSteps":0.0,"stepCount":0,"kiloCalories":0,"sleepDuration":0,"inactivityAlertCount":0,"sleepPlus":false},"activityGraphData":{"lastSync":null,"activityTimelineSamples":[],"activityTimelineIcons":[],"activityZoneLimits":[0.0,4.61176872253418,9.22353744506836,13.835306167602539,19.600017070770264,26.517670154571533,40.35297632217407],"heartRateTimelineSamples":[],"heartRateSummary":{"dayMinimum":0,"dayMinimumDateTime":null,"dayMaximum":0,"dayMaximumDateTime":null,"nightMinimum":0,"nightMinimumDateTime":null},"highSessionTimelineList":[],"trainingTimelineList":[]},"activityScoreData":{"sleepDuration":0,"sedentaryDuration":0,"lightDuration":0,"moderateDuration":0,"vigorousDuration":0},"activityBenefitFeedbackData":{"mvpa":"NONE","sitting":"NONE","improves":[],"promotes":[],"aids":[]}}`

// syncedDay is a populated day shaped after the web UI's reading of the
// payload (flow-ui-mono, 2026-09-30) — durations in minutes, distance in
// metres, samples {time, value} with time the local clock as epoch ms. Not a
// capture: the test account has no device.
const syncedDay = `{"dataPanelData":{"dailyActivityGoal":112.5,"activeTime":245,"distanceFromSteps":8123.4,"stepCount":10432,"kiloCalories":2650,"sleepDuration":452,"inactivityAlertCount":1,"sleepPlus":true},"activityGraphData":{"lastSync":1790683200000,"activityTimelineSamples":[{"time":1790726400000,"value":0.9},{"time":1790755200000,"value":3.25},{"time":1790812800000,"value":5}],"activityTimelineIcons":[],"activityZoneLimits":[0.0,1.2,2.4,3.6,4.8,6.0,9.0],"heartRateTimelineSamples":[{"time":1790726400000,"value":52},{"time":1790730000000,"value":0},{"time":1790755200000,"value":118}],"heartRateSummary":{"dayMinimum":55,"dayMinimumDateTime":1790650000000,"dayMaximum":162,"dayMaximumDateTime":1790670000000,"nightMinimum":48,"nightMinimumDateTime":1790630000000},"highSessionTimelineList":[],"trainingTimelineList":[]},"activityScoreData":{"sleepDuration":452,"sedentaryDuration":480,"lightDuration":300,"moderateDuration":150,"vigorousDuration":58},"activityBenefitFeedbackData":{"mvpa":"GOOD","sitting":"NONE","improves":["AEROBIC_FITNESS"],"promotes":[],"aids":[]}}`

// fourDays wraps day bodies into a loadFour response keyed by date.
func fourDays(days map[string]string) string {
	parts := make([]string, 0, len(days))
	for k, v := range days {
		parts = append(parts, `"`+k+`":`+v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func widgetPayload(t *testing.T, res *mcpgo.CallToolResult, wantType string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(res.StructuredContent)
	var p map[string]any
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p["type"] != wantType {
		t.Fatalf("payload type = %v, want %s", p["type"], wantType)
	}
	return p
}

//nolint:gocyclo // one sequential scenario
func TestGetDailyActivity_RangeStepsFourDaysAtATime(t *testing.T) {
	ff := newFakeFlow(t)
	// The windows are fetched in parallel, so every call gets the same body
	// (the fake routes by path only); the handler keeps the days in range.
	ff.on("GET", "/api/activity-timeline/loadFour", 200, "application/json", fourDays(map[string]string{
		"2026-09-20": emptyDay, "2026-09-21": emptyDay, "2026-09-22": syncedDay, "2026-09-23": emptyDay,
		"2026-09-24": emptyDay, "2026-09-25": emptyDay, "2026-09-26": emptyDay, "2026-09-27": emptyDay,
	}))
	res := callTool(t, GetDailyActivityHandler(ff.client()), map[string]any{"from_date": "2026-09-21", "to_date": "2026-09-26"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	reqs := ff.requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %+v, want 2 loadFour calls", reqs)
	}
	got := map[string]bool{}
	for _, r := range reqs {
		q, _ := url.ParseQuery(r.Query)
		// A tiny maxSampleCount 500s on days with device data; always ask for the full curve.
		if q.Get("maxSampleCount") != "144" {
			t.Fatalf("query = %q, want maxSampleCount=144", r.Query)
		}
		got[q.Get("day")] = true
	}
	if !got["2026-09-23"] || !got["2026-09-27"] {
		t.Fatalf("windows = %v, want days 2026-09-23 and 2026-09-27", got)
	}
	p := widgetPayload(t, res, "daily_activity")
	days, _ := p["days"].([]any)
	if len(days) != 6 {
		t.Fatalf("days = %d, want 6 (20th and 27th are outside the range)", len(days))
	}
	first := obj(days[0])
	if first["date"] != "2026-09-21" || first["has_data"] != false || first["steps"] != nil {
		t.Fatalf("zero day must read as no data: %v", first)
	}
	d := obj(days[1])
	if d["date"] != "2026-09-22" || d["has_data"] != true || d["samples"] != nil {
		t.Fatalf("synced day = %v", d)
	}
	// Minutes → seconds, metres stay metres, percentage passes through.
	if d["active_time_s"] != 14700.0 || d["sleep_s"] != 27120.0 || d["step_distance_m"] != 8123.0 || d["activity_goal_pct"] != 112.5 {
		t.Fatalf("units: %v", d)
	}
	assertJSONEqual(t, mustJSON(t, d["intensity"]), `{"sleep_s":27120,"sedentary_s":28800,"light_s":18000,"moderate_s":9000,"vigorous_s":3480}`)
	assertJSONEqual(t, mustJSON(t, d["heart_rate"]), `{"day_min_bpm":55,"day_max_bpm":162,"night_min_bpm":48}`)
	assertJSONEqual(t, mustJSON(t, d["benefit"]), `{"mvpa":"good","improves":["AEROBIC_FITNESS"]}`)
	if d["last_sync"] != "2026-09-29T12:00:00Z" {
		t.Fatalf("last_sync = %v", d["last_sync"])
	}
	s := obj(p["summary"])
	if s["days_with_data"] != 1.0 || s["total_steps"] != 10432.0 {
		t.Fatalf("summary = %v", s)
	}
	if !strings.Contains(resultText(res), "2026-09-22: 10432 steps") {
		t.Fatalf("text = %q", resultText(res))
	}
}

func TestGetDailyActivity_SingleDayHasSamples(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/activity-timeline/loadFour", 200, "application/json", fourDays(map[string]string{
		"2026-09-28": emptyDay, "2026-09-29": emptyDay, "2026-09-30": syncedDay, "2026-10-01": `{"dataPanelData":null}`,
	}))
	res := callTool(t, GetDailyActivityHandler(ff.client()), map[string]any{"from_date": "2026-09-30", "to_date": "2026-09-30"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	q, _ := url.ParseQuery(ff.requests()[0].Query)
	if q.Get("day") != "2026-10-02" || q.Get("maxSampleCount") != "144" {
		t.Fatalf("query = %q", ff.requests()[0].Query)
	}
	days, _ := widgetPayload(t, res, "daily_activity")["days"].([]any)
	if len(days) != 1 {
		t.Fatalf("days = %v", days)
	}
	// 1790726400000 = 2026-09-30T00:00Z: local clock encoded as UTC. The zero
	// heart-rate point is a gap and is dropped.
	assertJSONEqual(t, mustJSON(t, obj(days[0])["samples"]), `{
		"activity":[{"t":"00:00","v":0.9},{"t":"08:00","v":3.25}],
		"heart_rate":[{"t":"00:00","v":52},{"t":"08:00","v":118}],
		"zone_limits":[0,1.2,2.4,3.6,4.8,6,9]}`)
}

func TestGetDailyActivity_Validation(t *testing.T) {
	runValidationCases(t, GetDailyActivityHandler, []validationCase{
		{"bad date", map[string]any{"from_date": "30.09.2026"}, "from_date must be ISO 8601"},
		{"reversed", map[string]any{"from_date": "2026-09-30", "to_date": "2026-09-01"}, "is after to_date"},
		{"too long", map[string]any{"from_date": "2026-08-01", "to_date": "2026-09-30"}, "at most 31"},
	})
}

// sleepNight is a night shaped after the web UI's sleep report (flow-ui-mono,
// 2026-09-30). Not a capture: the test account has no sleep-tracking device.
const sleepNight = `{"date":"2026-09-29","sleepStartTime":"2026-09-28T23:00:00.000+02:00","sleepEndTime":"2026-09-29T07:00:00.000+02:00","sleepStartOffset":600,"sleepEndOffset":-300,"sleepScore":78.46,"continuityClass":3,"sleepCycles":4,"continuityIndex":3.4,"sleepRating":"SLEPT_WELL","sleepWakeStates":[{"sleepWakeState":0,"offsetFromStart":0},{"sleepWakeState":2,"offsetFromStart":600},{"sleepWakeState":3,"offsetFromStart":3600},{"sleepWakeState":1,"offsetFromStart":7200},{"sleepWakeState":0,"offsetFromStart":10800,"longInterruption":true},{"sleepWakeState":2,"offsetFromStart":11400}]}`

//nolint:gocyclo // one sequential scenario
func TestGetSleep(t *testing.T) {
	ff := newFakeFlow(t)
	outside := strings.Replace(sleepNight, `"date":"2026-09-29"`, `"date":"2026-09-10"`, 1)
	ff.on("GET", "/api/sleep/report", 200, "application/json", "["+outside+","+sleepNight+"]")
	res := callTool(t, GetSleepHandler(ff.client()), map[string]any{"from_date": "2026-09-23", "to_date": "2026-09-29"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	r := ff.requests()[0]
	q, _ := url.ParseQuery(r.Query)
	// Flow needs ≥ 30 days: the window is widened backwards and filtered.
	if q.Get("from") != "2026-08-30" || q.Get("to") != "2026-09-29" || r.XRequestedWith != "XMLHttpRequest" {
		t.Fatalf("request = %+v", r)
	}
	p := widgetPayload(t, res, "sleep_report")
	nights, _ := p["nights"].([]any)
	if len(nights) != 1 {
		t.Fatalf("nights = %v (the 10th is outside the range)", nights)
	}
	n := obj(nights[0])
	// Fell asleep 23:00 + 600 s, woke 07:00 − 300 s: 7 h 45 min.
	if n["fell_asleep"] != "2026-09-28T23:10:00" || n["woke_up"] != "2026-09-29T06:55:00" || n["sleep_s"] != 27900.0 {
		t.Fatalf("times = %v", n)
	}
	if n["score"] != 78.5 || n["rating"] != "well" || n["sleep_cycles"] != 4.0 {
		t.Fatalf("scalars = %v", n)
	}
	// Segments shift by the 600 s start offset; the first (wake) segment ends
	// exactly at falling asleep and disappears.
	assertJSONEqual(t, mustJSON(t, n["hypnogram"]), `[
		{"stage":"light","start_s":0,"end_s":3000},
		{"stage":"deep","start_s":3000,"end_s":6600},
		{"stage":"rem","start_s":6600,"end_s":10200},
		{"stage":"wake","start_s":10200,"end_s":10800,"long":true},
		{"stage":"light","start_s":10800,"end_s":27900}]`)
	assertJSONEqual(t, mustJSON(t, n["stages"]), `{"light_s":20100,"deep_s":3600,"rem_s":3600,"unknown_s":0}`)
	if n["interruptions_s"] != 600.0 || n["long_interruptions_s"] != 600.0 {
		t.Fatalf("interruptions = %v", n)
	}
	avg := obj(p["averages"])
	if avg["nights"] != 1.0 || avg["fell_asleep"] != "23:10" || avg["woke_up"] != "06:55" {
		t.Fatalf("averages = %v", avg)
	}
}

func TestGetSleep_EmptyAndValidation(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/sleep/report", 200, "application/json", "[]")
	res := callTool(t, GetSleepHandler(ff.client()), map[string]any{"from_date": "2026-09-01", "to_date": "2026-09-30"})
	if res.IsError || !strings.Contains(resultText(res), "No sleep recorded") {
		t.Fatalf("result = %v %q", res.IsError, resultText(res))
	}
	q, _ := url.ParseQuery(ff.requests()[0].Query)
	if q.Get("from") != "2026-08-31" {
		t.Fatalf("a 30-day request must still be widened to Flow's 30-day minimum: %q", ff.requests()[0].Query)
	}
	runValidationCases(t, GetSleepHandler, []validationCase{
		{"reversed", map[string]any{"from_date": "2026-09-30", "to_date": "2026-09-01"}, "is after to_date"},
		{"too long", map[string]any{"from_date": "2025-01-01", "to_date": "2026-09-30"}, "at most 365"},
	})
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A real account's 365-day report held a night with "sleepScore": null, which
// used to fail the whole report's decode (seen live 2026-10-01). Null offsets
// count as 0, like moment.add(null) in the web UI.
func TestGetSleep_NullScoreAndOffsets(t *testing.T) {
	ff := newFakeFlow(t)
	night := strings.NewReplacer(`"sleepScore":78.46`, `"sleepScore":null`,
		`"sleepStartOffset":600`, `"sleepStartOffset":null`, `"sleepEndOffset":-300`, `"sleepEndOffset":null`).Replace(sleepNight)
	ff.on("GET", "/api/sleep/report", 200, "application/json", "["+night+","+sleepNight+"]")
	res := callTool(t, GetSleepHandler(ff.client()), map[string]any{"from_date": "2026-09-01", "to_date": "2026-09-29"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	nights, _ := widgetPayload(t, res, "sleep_report")["nights"].([]any)
	if len(nights) != 2 {
		t.Fatalf("nights = %v", nights)
	}
	n := obj(nights[0])
	if n["score"] != nil || n["fell_asleep"] != "2026-09-28T23:00:00" || n["sleep_s"] != 28800.0 {
		t.Fatalf("null-score night = %v", n)
	}
}

// On a synced account Flow stamps lastSync on future days too, with an
// all-zero panel (seen live 2026-10-02 for tomorrow): still no data.
func TestGetDailyActivity_ZeroDayWithLastSyncIsNoData(t *testing.T) {
	ff := newFakeFlow(t)
	tomorrow := strings.Replace(emptyDay, `"lastSync":null`, `"lastSync":1790683200000`, 1)
	ff.on("GET", "/api/activity-timeline/loadFour", 200, "application/json", fourDays(map[string]string{
		"2026-10-02": syncedDay, "2026-10-03": tomorrow,
	}))
	res := callTool(t, GetDailyActivityHandler(ff.client()), map[string]any{"from_date": "2026-10-02", "to_date": "2026-10-03"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	p := widgetPayload(t, res, "daily_activity")
	days, _ := p["days"].([]any)
	if d := obj(days[1]); d["has_data"] != false || d["steps"] != nil || d["sleep_plus"] != false {
		t.Fatalf("tomorrow = %v", d)
	}
	if s := obj(p["summary"]); s["days_with_data"] != 1.0 {
		t.Fatalf("summary = %v", s)
	}
}

// One failing window fails the call with Flow's error, not a partial range.
func TestGetDailyActivity_WindowErrorFailsCall(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/activity-timeline/loadFour", 500, "text/plain", "Failed to load activity timeline data")
	res := callTool(t, GetDailyActivityHandler(ff.client()), map[string]any{"from_date": "2026-09-01", "to_date": "2026-09-20"})
	if !res.IsError || !strings.Contains(resultText(res), "Failed to load activity timeline data") {
		t.Fatalf("result = %v %q", res.IsError, resultText(res))
	}
}
