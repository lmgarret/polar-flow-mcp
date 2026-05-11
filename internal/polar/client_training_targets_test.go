//go:build polartest

package polar_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lm/polar-flow-mcp/internal/polar"
)

func TestCreateTrainingTarget_Success(t *testing.T) {
	var capturedMethod string
	var capturedPath string
	var capturedAuth string
	var capturedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"99"}`))
	}))
	defer ts.Close()

	restore := polar.SetTrainingTargetsBaseURL(ts.URL)
	t.Cleanup(restore)

	client := polar.NewClient("tok")
	durationMs := int64(600 * 1000)
	cooldownMs := int64(300 * 1000)
	recoveryMs := int64(120 * 1000)
	distM := float64(1000)
	reps := 5
	zone := 4
	req := polar.CreateTrainingTargetRequest{
		Session: polar.TrainingSessionTarget{
			Name: "5x1km Threshold",
			StartTime: polar.DateTime{
				Year:  2026,
				Month: 5,
				Day:   15,
				Hour:  18,
				Min:   0,
				Sec:   0,
			},
		},
		Exercise: []polar.ExerciseTarget{
			{
				Idx:  0,
				Type: "PHASED",
				PhaseOrRepeat: []polar.PhaseOrRepeat{
					{
						Name:       "warmup",
						ChangeType: "AUTOMATIC",
						Goal:       polar.PhaseGoal{Type: "DURATION", Duration: &durationMs},
						Intensity:  &polar.PhaseIntensity{Type: "NONE"},
					},
					{
						Name:          "intervals",
						ChangeType:    "AUTOMATIC",
						Goal:          polar.PhaseGoal{Type: "MANUAL"},
						RepeatCount:   &reps,
						PhaseOrRepeat: []polar.PhaseOrRepeat{
							{
								Name:       "work",
								ChangeType: "AUTOMATIC",
								Goal:       polar.PhaseGoal{Type: "DISTANCE", Distance: &distM},
								Intensity:  &polar.PhaseIntensity{Type: "HEART_RATE_ZONES", LowerZone: &zone, UpperZone: &zone},
							},
							{
								Name:       "recovery",
								ChangeType: "AUTOMATIC",
								Goal:       polar.PhaseGoal{Type: "DURATION", Duration: &recoveryMs},
								Intensity:  &polar.PhaseIntensity{Type: "NONE"},
							},
						},
					},
					{
						Name:       "cooldown",
						ChangeType: "AUTOMATIC",
						Goal:       polar.PhaseGoal{Type: "DURATION", Duration: &cooldownMs},
						Intensity:  &polar.PhaseIntensity{Type: "NONE"},
					},
				},
			},
		},
	}

	id, err := client.CreateTrainingTarget(t.Context(), "12345", req)
	if err != nil {
		t.Fatalf("CreateTrainingTarget returned error: %v", err)
	}
	if id != "99" {
		t.Errorf("id = %q, want %q", id, "99")
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	if capturedAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want 'Bearer tok'", capturedAuth)
	}
	if capturedPath != "/12345/training-targets" {
		t.Errorf("path = %q, want /12345/training-targets", capturedPath)
	}

	// Verify body contains expected fields.
	var parsed map[string]any
	if err := json.Unmarshal(capturedBody, &parsed); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	bodyStr := string(capturedBody)
	for _, want := range []string{`"duration":600000`, `"distance":1000`, `"repeatCount":5`, `"lowerZone":4`, `"upperZone":4`, `"duration":120000`} {
		if !containsSubstring(bodyStr, want) {
			t.Errorf("body missing %q; body = %s", want, bodyStr)
		}
	}
}

func TestCreateTrainingTarget_ErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid"))
	}))
	defer ts.Close()

	restore := polar.SetTrainingTargetsBaseURL(ts.URL)
	t.Cleanup(restore)

	client := polar.NewClient("tok")
	_, err := client.CreateTrainingTarget(t.Context(), "12345", polar.CreateTrainingTargetRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsSubstring(err.Error(), "status 400") {
		t.Errorf("error %q should contain 'status 400'", err.Error())
	}
	if !containsSubstring(err.Error(), "invalid") {
		t.Errorf("error %q should contain 'invalid'", err.Error())
	}
}

func TestCreateTrainingTarget_AcceptsBoth200And201(t *testing.T) {
	for _, statusCode := range []int{http.StatusOK, http.StatusCreated} {
		statusCode := statusCode
		t.Run("status_"+http.StatusText(statusCode), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(`{"id":"42"}`))
			}))
			defer ts.Close()

			restore := polar.SetTrainingTargetsBaseURL(ts.URL)
			t.Cleanup(restore)

			client := polar.NewClient("tok")
			id, err := client.CreateTrainingTarget(t.Context(), "12345", polar.CreateTrainingTargetRequest{})
			if err != nil {
				t.Fatalf("status %d: expected no error, got: %v", statusCode, err)
			}
			if id != "42" {
				t.Errorf("status %d: id = %q, want %q", statusCode, id, "42")
			}
		})
	}
}

func TestCreateTrainingTarget_NoIdInBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		// empty body — no id returned
	}))
	defer ts.Close()

	restore := polar.SetTrainingTargetsBaseURL(ts.URL)
	t.Cleanup(restore)

	client := polar.NewClient("tok")
	id, err := client.CreateTrainingTarget(t.Context(), "12345", polar.CreateTrainingTargetRequest{})
	if err != nil {
		t.Fatalf("expected no error for empty body, got: %v", err)
	}
	if id != "(id not returned)" {
		t.Errorf("id = %q, want %q", id, "(id not returned)")
	}
}

// containsSubstring is a local helper (strings.Contains is not available without import in test helper scope).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}())
}
