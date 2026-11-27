package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// Defaults of get_session_track: a 1 s track is 3600 fixes for an hour.
const (
	defaultTrackMaxPoints = 1000
	maxTrackMaxPoints     = 100000
)

// sessionTrack is the get_session_track payload.
type sessionTrack struct {
	Type string `json:"type"`
	convert.SessionTrack
}

// trackRequest is the validated input of get_session_track.
type trackRequest struct {
	sessionID  int64
	exerciseID int64 // 0 = the first exercise
	maxPoints  int
}

func parseTrackRequest(req mcpgo.CallToolRequest) (trackRequest, error) {
	var in trackRequest
	var err error
	if in.sessionID, err = idArg(req, "session_id"); err != nil {
		return in, err
	}
	if v, ok, err := intArgRange(req, "exercise_id", 1, math.MaxInt64>>11, ""); err != nil {
		return in, err
	} else if ok {
		in.exerciseID = v
	}
	in.maxPoints = defaultTrackMaxPoints
	if v, ok, err := intArgRange(req, "max_points", 2, maxTrackMaxPoints, ""); err != nil {
		return in, err
	} else if ok {
		in.maxPoints = int(v)
	}
	return in, nil
}

// GetSessionTrackHandler returns the GPS track of one exercise of a completed
// session. The fixes ship in the same details response as the other series; the
// track is down-sampled like get_route so the result stays small.
func GetSessionTrackHandler(fc *flow.Client) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		in, err := parseTrackRequest(req)
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
		var start time.Time
		if raw, ok := details.Exercises.Or(nil)[strconv.FormatInt(ex.ExerciseID, 10)]; ok {
			start = convert.WireStartTime(raw.StartTime.Or(""))
		}
		block := details.Samples.Or(nil)[strconv.FormatInt(ex.ExerciseID, 10)]
		track, hasTrack := convert.FromWireSessionTrack(block, in.sessionID, ex.ExerciseID, start, ex.DurationS, in.maxPoints)
		if !hasTrack {
			return mcpgo.NewToolResultText(fmt.Sprintf(
				"Session %d has no GPS track for exercise %d (manually entered sessions and indoor sports have none).",
				in.sessionID, ex.ExerciseID)), nil
		}
		payload := sessionTrack{Type: "session_track", SessionTrack: track}
		b, err := json.Marshal(payload)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		result := mcpgo.NewToolResultText(string(b))
		result.StructuredContent = payload
		return result, nil
	}
}
