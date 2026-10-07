package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// Defaults of get_training_session_samples.
const (
	defaultSampleResolutionS = 60
	maxSampleOffsetS         = 359999 // Flow's longest session, 99:59:59
)

var defaultSampleMetrics = []string{"hr", "speed"}

// sessionSamples is the get_training_session_samples payload.
type sessionSamples struct {
	Type      string `json:"type"`
	SessionID int64  `json:"session_id"`
	Rep       *int   `json:"rep,omitempty"`
	convert.SessionSamples
}

// sampleRequest is the validated input of get_training_session_samples.
type sampleRequest struct {
	sessionID  int64
	exerciseID int64 // 0 = the first exercise
	metrics    []string
	window     convert.SampleWindow
	rep        int // 0 = no rep window
}

// GetTrainingSessionSamplesHandler returns a session's recorded series (HR,
// speed/pace, power, …) averaged into fixed buckets, optionally limited to one
// rep of a planned target or a time window. It reads the same details
// endpoint as get_training_session_details and resamples server-side, so the
// result stays small whatever the session length.
func GetTrainingSessionSamplesHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		in, err := parseSampleRequest(req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		details, err := fc.GetTrainingSessionDetails(ctx, in.sessionID)
		if err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No session with id %d.", in.sessionID)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		view := convert.FromWireSessionDetails(details)
		ex, err := pickExercise(view, in.exerciseID)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		block, ok := details.Samples.Or(nil)[strconv.FormatInt(ex.ExerciseID, 10)]
		if !ok || len(ex.Samples) == 0 {
			return mcpgo.NewToolResultText(fmt.Sprintf(
				"Session %d has no recorded samples (manually entered sessions have none).", in.sessionID)), nil
		}
		window := in.window
		if in.rep > 0 {
			if ex.Target == nil || in.rep > len(ex.Target.Reps) {
				n := 0
				if ex.Target != nil {
					n = len(ex.Target.Reps)
				}
				return mcpgo.NewToolResultError(fmt.Sprintf(
					"rep %d does not exist: this session has %d rep(s) (get_training_session_details lists them)", in.rep, n)), nil
			}
			r := ex.Target.Reps[in.rep-1]
			window.FromS, window.ToS = float64(r.StartS), float64(r.EndS)
		}
		duration := 0.0
		if ex.DurationS != nil {
			duration = float64(*ex.DurationS)
		}
		samples, err := convert.ResampleSession(block, ex.ExerciseID, duration, in.metrics, window)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		payload := sessionSamples{Type: "session_samples", SessionID: in.sessionID, SessionSamples: samples}
		if in.rep > 0 {
			payload.Rep = &in.rep
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		result := mcpgo.NewToolResultText(string(b))
		result.StructuredContent = payload
		return result, nil
	}
}

//nolint:gocyclo // one straight-line check per argument
func parseSampleRequest(req mcpgo.CallToolRequest) (sampleRequest, error) {
	var in sampleRequest
	var err error
	if in.sessionID, err = idArg(req, "session_id"); err != nil {
		return in, err
	}
	if v, ok, err := intArgRange(req, "exercise_id", 1, math.MaxInt64>>11, ""); err != nil {
		return in, err
	} else if ok {
		in.exerciseID = v
	}
	if in.metrics, err = metricsArg(req); err != nil {
		return in, err
	}
	in.window.ResolutionS = defaultSampleResolutionS
	if v, ok, err := intArgRange(req, "resolution_s", 1, 3600, "seconds"); err != nil {
		return in, err
	} else if ok {
		in.window.ResolutionS = int(v)
	}
	from, fromOK, err := intArgRange(req, "from_s", 0, maxSampleOffsetS, "seconds")
	if err != nil {
		return in, err
	}
	to, toOK, err := intArgRange(req, "to_s", 1, maxSampleOffsetS, "seconds")
	if err != nil {
		return in, err
	}
	rep, repOK, err := intArgRange(req, "rep", 1, 1000, "")
	if err != nil {
		return in, err
	}
	switch {
	case repOK && (fromOK || toOK):
		return in, errors.New("give either rep or from_s/to_s, not both")
	case fromOK && toOK && to <= from:
		return in, fmt.Errorf("to_s (%d) must be after from_s (%d)", to, from)
	}
	in.window.FromS, in.window.ToS, in.rep = float64(from), float64(to), int(rep)
	return in, nil
}

// metricsArg reads the optional metrics list, defaulting to hr + speed.
func metricsArg(req mcpgo.CallToolRequest) ([]string, error) {
	raw, ok := rawArg(req, "metrics")
	if !ok {
		return defaultSampleMetrics, nil
	}
	list, isList := raw.([]any)
	if !isList {
		return nil, fmt.Errorf("metrics must be an array of strings, got %s", jsonTypeName(raw))
	}
	known := map[string]bool{}
	for _, m := range convert.SampleMetricNames {
		known[m] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, isStr := v.(string)
		if !isStr || !known[s] {
			return nil, fmt.Errorf("metrics: %v is not one of %v", v, convert.SampleMetricNames)
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("metrics must name at least one series")
	}
	return out, nil
}

// pickExercise returns the requested exercise, or the first one.
func pickExercise(view convert.SessionDetails, exerciseID int64) (convert.ExerciseDetail, error) {
	if len(view.Exercises) == 0 {
		return convert.ExerciseDetail{}, errors.New("the session has no exercises")
	}
	if exerciseID == 0 {
		return view.Exercises[0], nil
	}
	for _, e := range view.Exercises {
		if e.ExerciseID == exerciseID {
			return e, nil
		}
	}
	return convert.ExerciseDetail{}, fmt.Errorf("exercise_id %d is not part of session %d", exerciseID, view.ID)
}
