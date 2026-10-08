package mcp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

const detailsPath = "/api/training/analysis/9000000001/details"

// detailsFixture is the anonymized details response of a device-recorded
// interval run (8 reps; series thinned to PT5S).
func detailsFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../flow/testdata/session-details-intervals.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGetTrainingSessionDetailsCompact(t *testing.T) {
	raw := detailsFixture(t)
	ff := newFakeFlow(t)
	ff.on("GET", detailsPath, 200, "application/json", raw)
	res := callTool(t, GetTrainingSessionDetailsHandler(ff.client()), map[string]any{"session_id": float64(9000000001)})
	text := resultText(res)
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	var got struct {
		Type    string `json:"type"`
		Details struct {
			Exercises []struct {
				Samples []string `json:"samples"`
				Target  struct {
					Reps []struct {
						Phase  string `json:"phase"`
						StartS int    `json:"start_s"`
					} `json:"reps"`
				} `json:"target"`
			} `json:"exercises"`
		} `json:"details"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got.Type != "session_details" || len(got.Details.Exercises) != 1 || len(got.Details.Exercises[0].Target.Reps) != 8 {
		t.Fatalf("payload = %+v, want session_details with 8 reps", got)
	}
	if strings.Contains(text, `"values"`) || strings.Contains(text, "latitude") {
		t.Error("the compact view must not carry sample values or GPS")
	}
	// The real response is ~500 KB; the view must stay a few KB.
	if len(text) > len(raw)/4 || len(text) > 16000 {
		t.Errorf("view is %d bytes (fixture %d), want a compact payload", len(text), len(raw))
	}
}

type samplesPayload struct {
	Type        string                `json:"type"`
	SessionID   int64                 `json:"session_id"`
	Rep         *int                  `json:"rep"`
	FromS       int                   `json:"from_s"`
	ToS         int                   `json:"to_s"`
	ResolutionS int                   `json:"resolution_s"`
	Points      int                   `json:"points"`
	Columns     map[string][]*float64 `json:"columns"`
}

func callSamples(t *testing.T, ff *fakeFlow, args map[string]any) (samplesPayload, string, bool) {
	t.Helper()
	res := callTool(t, GetTrainingSessionSamplesHandler(ff.client()), args)
	text := resultText(res)
	var p samplesPayload
	if !res.IsError && strings.HasPrefix(text, "{") {
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, text)
		}
	}
	return p, text, res.IsError
}

func TestGetTrainingSessionSamplesDefaults(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", detailsPath, 200, "application/json", detailsFixture(t))
	p, text, isErr := callSamples(t, ff, map[string]any{"session_id": float64(9000000001)})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	// 3683 s at the default 60 s.
	if p.Type != "session_samples" || p.ResolutionS != 60 || p.Points != 62 || p.ToS != 3683 {
		t.Fatalf("header = %+v, want 62 points of 60 s over 0–3683", p)
	}
	for _, col := range []string{"t_s", "hr_bpm", "speed_kmh"} {
		if len(p.Columns[col]) != 62 {
			t.Errorf("%s has %d rows, want 62", col, len(p.Columns[col]))
		}
	}
	if len(p.Columns) != 3 {
		t.Errorf("columns = %v, want t_s + the default hr and speed", p.Columns)
	}
}

//nolint:gocyclo // one sequential scenario
func TestGetTrainingSessionSamplesRep(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", detailsPath, 200, "application/json", detailsFixture(t))
	p, text, isErr := callSamples(t, ff, map[string]any{
		"session_id": float64(9000000001), "rep": float64(4),
		"metrics": []any{"hr", "pace", "power"}, "resolution_s": float64(15),
	})
	if isErr {
		t.Fatalf("error: %s", text)
	}
	if p.Rep == nil || *p.Rep != 4 || p.FromS != 1452 || p.ToS != 1932 || p.Points != 32 {
		t.Fatalf("header = %+v, want rep 4 over 1452–1932 in 32 buckets", p)
	}
	for i, hr := range p.Columns["hr_bpm"] {
		if hr == nil || *hr < 120 || *hr > 200 {
			t.Fatalf("hr_bpm[%d] = %v, want a plausible work-rep HR", i, hr)
		}
	}
	// The rep has a slow patch mid-way (on the chart too); most buckets are running pace.
	running := 0
	for _, v := range p.Columns["pace_s_per_km"] {
		if v != nil && *v > 200 && *v < 360 {
			running++
		}
	}
	if running < 24 {
		t.Errorf("%d of 32 pace buckets at running pace, want most of them", running)
	}
}

func TestGetTrainingSessionSamplesNoData(t *testing.T) {
	manual := `{"id":5,"exercises":{"6":{"id":6,"sport":{"id":1},"duration":"PT30M"}},
		"samples":{"6":{"HEART_RATE":null,"SPEED":null}},"zones":{"6":{}},"exerciseResultTargetData":{}}`
	ff := newFakeFlow(t)
	ff.on("GET", "/api/training/analysis/5/details", 200, "application/json", manual)
	_, text, isErr := callSamples(t, ff, map[string]any{"session_id": float64(5)})
	if isErr || !strings.Contains(text, "no recorded samples") {
		t.Fatalf("manual session = %q (error %v), want a no-samples notice", text, isErr)
	}

	ff.on("GET", "/api/training/analysis/404/details", 404, "text/plain", "")
	if _, text, _ := callSamples(t, ff, map[string]any{"session_id": float64(404)}); !strings.Contains(text, "No session with id 404") {
		t.Errorf("unknown session = %q", text)
	}

	ff.on("GET", detailsPath, 200, "application/json", detailsFixture(t))
	_, text, isErr = callSamples(t, ff, map[string]any{"session_id": float64(9000000001), "rep": float64(9)})
	if !isErr || !strings.Contains(text, "8 rep(s)") {
		t.Errorf("rep out of range = %q (error %v), want an error naming the 8 reps", text, isErr)
	}
}

func TestGetTrainingSessionSamplesValidation(t *testing.T) {
	mk := func(c *flow.Client) toolHandler { return GetTrainingSessionSamplesHandler(c) }
	id := float64(9000000001)
	runValidationCases(t, mk, []validationCase{
		{"missing id", map[string]any{}, "session_id is required"},
		{"rep and window", map[string]any{"session_id": id, "rep": float64(1), "from_s": float64(0)}, "either rep or from_s/to_s"},
		{"reversed window", map[string]any{"session_id": id, "from_s": float64(600), "to_s": float64(60)}, "must be after from_s"},
		{"zero resolution", map[string]any{"session_id": id, "resolution_s": float64(0)}, "resolution_s must be between 1 and 3600"},
		{"fractional resolution", map[string]any{"session_id": id, "resolution_s": 2.5}, "whole number"},
		{"unknown metric", map[string]any{"session_id": id, "metrics": []any{"hr", "vo2"}}, "vo2 is not one of"},
		{"metrics not a list", map[string]any{"session_id": id, "metrics": "hr"}, "metrics must be an array"},
		{"empty metrics", map[string]any{"session_id": id, "metrics": []any{}}, "at least one"},
	})
}
