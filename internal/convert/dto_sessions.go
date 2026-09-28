package convert

import (
	"errors"
	"math"
	"sort"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Session edit limits, as probed on PUT /api/training/editTraining/{id}
// (2026-09-28). Flow answers anything outside them with a bare 500 and no
// body, so the edit tool checks them before sending.
const (
	MaxSessionNameRunes = 100
	MaxSessionNoteRunes = 10000
	MaxSessionDurationS = 359999 // 99:59:59
	MaxSessionDistanceM = 9999000
	MaxSessionHR        = 240
	MaxSessionKcal      = 65535
	MaxSessionSpeedKmh  = 399
)

// ErrSessionNotEditable is returned by EditBodyFromSummary for sessions whose
// full edit is unverified and potentially destructive: device-recorded
// sessions (the web form re-synthesizes the HR trace) and multi-exercise
// sessions (the form edits one exercise).
var ErrSessionNotEditable = errors.New("only single-exercise, manually entered sessions can have their " +
	"sport, duration, distance, heart rate, calories, speed or name edited; " +
	"note and feeling can be changed on any session")

// EditBodyFromSummary builds a full TrainingSessionEdit body carrying the
// session's current values, read from GET /api/training/analysis/{id}/summary.
// The edit endpoint treats a missing name as "reset to the sport name" and a
// null note as the literal string "null", so the edit tool starts from the
// live values and overrides only what the caller changed. Feeling is left
// unset (null → unchanged). Unknown numeric values are left unset too, which
// the endpoint also treats as unchanged.
func EditBodyFromSummary(s *gen.SessionSummary) (*gen.TrainingSessionEdit, error) {
	if s == nil {
		return nil, errors.New("no session summary")
	}
	if name, ok := s.DeviceName.Get(); ok && name != "" {
		return nil, ErrSessionNotEditable
	}
	ex, ok := singleExercise(s)
	if !ok {
		return nil, ErrSessionNotEditable
	}
	body := &gen.TrainingSessionEdit{}
	editMetricsFromSummary(body, s, ex)
	name := ""
	if v, ok := s.TrainingSessionName.Get(); ok {
		name = v
	}
	body.TrainingSessionName.SetTo(name)
	note := ""
	if v, ok := s.Note.Get(); ok {
		note = CleanNote(v)
	}
	body.Note.SetTo(note)
	body.EditedExerciseId.SetToNull()
	return body, nil
}

// editMetricsFromSummary copies sport, duration, distance, heart rates,
// calories and average speed from the live session into the edit body; values
// the summary lacks stay unset (the endpoint reads a missing key as unchanged).
func editMetricsFromSummary(body *gen.TrainingSessionEdit, s *gen.SessionSummary, ex gen.SessionExercise) {
	if sp, ok := ex.Sport.Get(); ok {
		if id, ok := sp.ID.Get(); ok {
			body.Sport.SetTo(id)
		}
	}
	if d, ok := s.Duration.Get(); ok {
		if secs, parsed := ISO8601DurationToSeconds(d); parsed {
			body.Duration.SetTo(secs)
		}
	}
	// A fresh manual session has a null top-level distance; the exercise
	// always carries it.
	if v, ok := ex.Distance.Get(); ok {
		body.Distance.SetTo(v)
	} else if v, ok := s.Distance.Get(); ok {
		body.Distance.SetTo(v)
	}
	if v, ok := s.HrAverage.Get(); ok {
		body.HrAverage.SetTo(v)
	}
	if v, ok := s.HrMax.Get(); ok {
		body.HrMax.SetTo(v)
	}
	if v, ok := s.KiloCalories.Get(); ok {
		body.KiloCalories.SetTo(v)
	} else if v, ok := ex.Calories.Get(); ok {
		body.KiloCalories.SetTo(v)
	}
	if stats, ok := ex.TrainingStatistic.Get(); ok {
		if v, ok := stats["SPEED"].Avg.Get(); ok {
			body.SpeedAverage.SetTo(v)
		}
	}
}

// singleExercise returns the session's only exercise.
func singleExercise(s *gen.SessionSummary) (gen.SessionExercise, bool) {
	exs, ok := s.Exercises.Get()
	if !ok || len(exs) != 1 {
		return gen.SessionExercise{}, false
	}
	for _, ex := range exs {
		return ex, true
	}
	return gen.SessionExercise{}, false
}

// SessionSportID returns the sport id of a session's first exercise (sorted by
// exercise key for determinism) — the top-level sportId is usually null.
func SessionSportID(s *gen.SessionSummary) (int, bool) {
	if s == nil {
		return 0, false
	}
	if v, ok := s.SportId.Get(); ok {
		return v, true
	}
	exs, ok := s.Exercises.Get()
	if !ok {
		return 0, false
	}
	keys := make([]string, 0, len(exs))
	for k := range exs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if sp, ok := exs[k].Sport.Get(); ok {
			if id, ok := sp.ID.Get(); ok {
				return id, true
			}
		}
	}
	return 0, false
}

// RoundTo rounds v to the given number of decimals (for comparing read-backs
// of float fields such as speed).
func RoundTo(v float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}
