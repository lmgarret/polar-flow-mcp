package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// sessionErrorResult maps the session sentinel errors to the tool answer.
func sessionErrorResult(err error, id int64) *mcpgo.CallToolResult {
	switch {
	case errors.Is(err, flow.ErrSessionNotFound):
		return mcpgo.NewToolResultText(fmt.Sprintf("No session with id %d.", id))
	case errors.Is(err, flow.ErrNotOwned):
		return mcpgo.NewToolResultError(fmt.Sprintf("session %d belongs to another Polar account.", id))
	default:
		return mcpgo.NewToolResultError(err.Error())
	}
}

// describeSession renders a session as something a user can agree to delete:
// name, start and duration rather than a bare id.
func describeSession(id int64, s *gen.SessionSummary) string {
	dto := convert.FromWireSessionSummary(s)
	parts := []string{}
	if dto.Name != "" {
		parts = append(parts, fmt.Sprintf("%q", dto.Name))
	}
	if dto.StartTime != "" {
		parts = append(parts, "on "+dto.StartTime)
	}
	if dto.SessionDurationS != nil {
		parts = append(parts, "("+convert.HumanDuration(*dto.SessionDurationS)+")")
	}
	if len(parts) == 0 {
		return fmt.Sprintf("the training session with id %d", id)
	}
	return fmt.Sprintf("the training session %s (id %d)", strings.Join(parts, " "), id)
}

// DeleteTrainingSessionHandler deletes a completed session after confirmation.
//
// Flow's delete answers 200 for an id that does not exist, so the handler
// checks existence through the summary endpoint first (404 → "no session",
// 403 → another account's) and reads it again afterwards to confirm the
// session is really gone.
func DeleteTrainingSessionHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, err := idArg(req, "session_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		summary, err := fc.CheckTrainingSession(ctx, id)
		if err != nil {
			return sessionErrorResult(err, id), nil
		}
		if result, proceed := requireConfirm(ctx, req, "delete_training_session", func() string {
			return fmt.Sprintf("Permanently delete %s? It disappears from your diary, weekly totals and "+
				"training load. This cannot be undone.", describeSession(id, summary))
		}); !proceed {
			return result, nil
		}
		if err := fc.DeleteTrainingSession(ctx, id); err != nil {
			return sessionErrorResult(err, id), nil
		}
		if _, err := fc.CheckTrainingSession(ctx, id); !errors.Is(err, flow.ErrSessionNotFound) {
			if err == nil {
				return mcpgo.NewToolResultError(fmt.Sprintf(
					"Polar acknowledged the delete but session %d still exists.", id)), nil
			}
			return mcpgo.NewToolResultText(fmt.Sprintf(
				"Deleted session %d (could not re-check: %v).", id, err)), nil
		}
		result := mcpgo.NewToolResultText(fmt.Sprintf("Deleted training session %d.", id))
		result.StructuredContent = map[string]any{"type": "session_deleted", "data": map[string]any{"id": id}}
		return result, nil
	}
}

// sessionEdit is the validated set of changes requested by the caller.
type sessionEdit struct {
	name, note         *string
	feeling            *string // wire value
	sportID            *int
	durationS          *int
	distanceM          *float64
	hrAvg, hrMax, kcal *int
	speedKmh           *float64
}

func (e sessionEdit) onlyNoteFeeling() bool {
	return e.name == nil && e.sportID == nil && e.durationS == nil && e.distanceM == nil &&
		e.hrAvg == nil && e.hrMax == nil && e.kcal == nil && e.speedKmh == nil
}

func (e sessionEdit) empty() bool {
	return e.onlyNoteFeeling() && e.note == nil && e.feeling == nil
}

// parseSessionEdit validates every edit argument against the ranges Flow
// enforces (it answers any violation with a bare 500 and no hint).
//
//nolint:gocyclo // one straight-line check per argument
func parseSessionEdit(req mcpgo.CallToolRequest) (sessionEdit, error) {
	var e sessionEdit
	if v, ok, err := textArg(req, "name", convert.MaxSessionNameRunes, true); err != nil {
		return e, err
	} else if ok {
		e.name = &v
	}
	if v, ok, err := textArg(req, "note", convert.MaxSessionNoteRunes, false); err != nil {
		return e, err
	} else if ok {
		e.note = &v
	}
	if v, ok, err := intArgRange(req, "feeling", 1, 5, "(1 = worst, 5 = best)"); err != nil {
		return e, err
	} else if ok {
		wire, _ := convert.FeelingToWire(int(v))
		e.feeling = &wire
	}
	if v, ok, err := intArgRange(req, "sport_id", 1, 100000, ""); err != nil {
		return e, err
	} else if ok {
		n := int(v)
		e.sportID = &n
	}
	if v, ok, err := intArgRange(req, "duration_s", 1, convert.MaxSessionDurationS, "seconds (99:59:59)"); err != nil {
		return e, err
	} else if ok {
		n := int(v)
		e.durationS = &n
	}
	if v, ok, err := floatArgRange(req, "distance_m", 0, convert.MaxSessionDistanceM, "metres"); err != nil {
		return e, err
	} else if ok {
		e.distanceM = &v
	}
	if v, ok, err := intArgRange(req, "hr_avg", 0, convert.MaxSessionHR, "bpm (0 clears it)"); err != nil {
		return e, err
	} else if ok {
		n := int(v)
		e.hrAvg = &n
	}
	if v, ok, err := intArgRange(req, "hr_max", 0, convert.MaxSessionHR, "bpm (0 clears it)"); err != nil {
		return e, err
	} else if ok {
		n := int(v)
		e.hrMax = &n
	}
	if v, ok, err := intArgRange(req, "kcal", 0, convert.MaxSessionKcal, "kcal"); err != nil {
		return e, err
	} else if ok {
		n := int(v)
		e.kcal = &n
	}
	if v, ok, err := floatArgRange(req, "speed_kmh", 0, convert.MaxSessionSpeedKmh, "km/h"); err != nil {
		return e, err
	} else if ok {
		e.speedKmh = &v
	}
	if e.empty() {
		return e, errors.New("nothing to change: pass at least one of name, note, feeling, sport_id, " +
			"duration_s, distance_m, hr_avg, hr_max, kcal, speed_kmh")
	}
	return e, nil
}

// applyTo overlays the requested changes on a body built from the live
// session, then cross-checks the merged heart rates.
func (e sessionEdit) applyTo(body *gen.TrainingSessionEdit) error {
	if e.name != nil {
		body.TrainingSessionName.SetTo(*e.name)
	}
	if e.note != nil {
		body.Note.SetTo(*e.note)
	}
	if e.feeling != nil {
		body.Feeling.SetTo(*e.feeling)
	}
	if e.sportID != nil {
		body.Sport.SetTo(*e.sportID)
	}
	if e.durationS != nil {
		body.Duration.SetTo(*e.durationS)
	}
	if e.distanceM != nil {
		body.Distance.SetTo(*e.distanceM)
	}
	if e.hrAvg != nil {
		body.HrAverage.SetTo(*e.hrAvg)
	}
	if e.hrMax != nil {
		body.HrMax.SetTo(*e.hrMax)
	}
	if e.kcal != nil {
		body.KiloCalories.SetTo(*e.kcal)
	}
	if e.speedKmh != nil {
		body.SpeedAverage.SetTo(*e.speedKmh)
	}
	avg, _ := body.HrAverage.Get()
	maxHR, _ := body.HrMax.Get()
	if avg > 0 && maxHR > 0 && maxHR < avg {
		return fmt.Errorf("hr_max (%d bpm) is below hr_avg (%d bpm)", maxHR, avg)
	}
	return nil
}

// EditTrainingSessionHandler edits a completed session.
//
// Note/feeling-only edits go through the partial updateTrainingData endpoint,
// which is safe on any session. Everything else goes through the full edit
// form endpoint, which is only used on single-exercise manual sessions: the
// handler reads the live values, overlays the requested changes and sends
// every field (a missing name would reset it to the sport name; a null note
// would be stored as the text "null").
func EditTrainingSessionHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, err := idArg(req, "session_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		edit, err := parseSessionEdit(req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if edit.sportID != nil {
			if err := checkSport(ctx, fc, *edit.sportID); err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
		}
		summary, err := fc.CheckTrainingSession(ctx, id)
		if err != nil {
			return sessionErrorResult(err, id), nil
		}
		if edit.onlyNoteFeeling() {
			body := &gen.TrainingSessionDataUpdate{}
			if edit.note != nil {
				body.Note.SetTo(*edit.note)
			}
			if edit.feeling != nil {
				body.Feeling.SetTo(*edit.feeling)
			}
			if err := fc.UpdateTrainingSessionData(ctx, id, body); err != nil {
				return sessionErrorResult(err, id), nil
			}
		} else {
			body, err := convert.EditBodyFromSummary(summary)
			if err != nil {
				return mcpgo.NewToolResultError(fmt.Sprintf("session %d: %v", id, err)), nil
			}
			if err := edit.applyTo(body); err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			if err := fc.EditTrainingSession(ctx, id, body); err != nil {
				return sessionErrorResult(err, id), nil
			}
		}
		after, err := fc.CheckTrainingSession(ctx, id)
		if err != nil {
			return mcpgo.NewToolResultText(fmt.Sprintf("Edited session %d (read-back failed: %v).", id, err)), nil
		}
		return widgetResultText(fmt.Sprintf("Edited session %d.", id), map[string]any{
			"type": "session_summary", "summary": convert.FromWireSessionSummary(after),
		}), nil
	}
}
