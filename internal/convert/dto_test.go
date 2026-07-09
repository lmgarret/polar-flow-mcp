package convert

import (
	"testing"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

func TestFromWireSessionListItem(t *testing.T) {
	var s gen.TrainingSessionSummary
	s.ID = 777
	s.Duration = 1800000 // ms
	s.SportId = 1
	s.StartDate = "2026-05-28 08:00:00.000"
	s.SportName.SetTo("Course à pied")
	s.Distance.SetTo(5000)
	s.HrAvg.SetTo(142)
	s.Calories.SetTo(320)
	s.Note.SetTo(" ")              // blank-note sentinel
	s.RecoveryTime.SetTo(38435999) // milliseconds on this endpoint

	got := FromWireSessionListItem(s)

	if got.RecoveryTimeS == nil || *got.RecoveryTimeS != 38435 {
		t.Errorf("RecoveryTimeS = %v, want 38435 (ms→s)", got.RecoveryTimeS)
	}

	if got.ID != 777 {
		t.Errorf("ID = %d, want 777", got.ID)
	}
	if got.SessionDurationS != 1800 {
		t.Errorf("SessionDurationS = %d, want 1800 (ms→s)", got.SessionDurationS)
	}
	if got.StartTime != "2026-05-28T08:00:00" {
		t.Errorf("StartTime = %q, want ISO T form", got.StartTime)
	}
	if got.DistanceM == nil || *got.DistanceM != 5000 {
		t.Errorf("DistanceM = %v, want 5000", got.DistanceM)
	}
	if got.HRAvg == nil || *got.HRAvg != 142 {
		t.Errorf("HRAvg = %v, want 142", got.HRAvg)
	}
	if got.Note != "" {
		t.Errorf("Note = %q, want empty (blank sentinel cleaned)", got.Note)
	}
}

func TestFromWireSessionListItem_NullsOmitted(t *testing.T) {
	var s gen.TrainingSessionSummary
	s.ID = 1
	s.Duration = 0
	// Distance / HrAvg / Calories left unset (null on the wire).
	got := FromWireSessionListItem(s)
	if got.DistanceM != nil || got.HRAvg != nil || got.Calories != nil {
		t.Errorf("expected nil pointers for absent values, got %v %v %v", got.DistanceM, got.HRAvg, got.Calories)
	}
}

func TestFromWireSessionSummary(t *testing.T) {
	var s gen.SessionSummary
	s.ID.SetTo(555)
	s.StartDate.SetTo("2026-05-25T08:00:00")
	s.Duration.SetTo("PT30M") // ISO 8601 duration
	s.Distance.SetTo(5200)
	s.HrAverage.SetTo(138)
	s.HrMax.SetTo(171)
	s.KiloCalories.SetTo(410)
	s.TrainingSessionName.SetTo("Easy 5k")

	got := FromWireSessionSummary(&s)

	if got.SessionDurationS == nil || *got.SessionDurationS != 1800 {
		t.Errorf("SessionDurationS = %v, want 1800 (PT30M→s)", got.SessionDurationS)
	}
	if got.DistanceM == nil || *got.DistanceM != 5200 {
		t.Errorf("DistanceM = %v, want 5200", got.DistanceM)
	}
	if got.HRAvg == nil || *got.HRAvg != 138 {
		t.Errorf("HRAvg = %v, want 138", got.HRAvg)
	}
	if got.Name != "Easy 5k" {
		t.Errorf("Name = %q, want Easy 5k", got.Name)
	}
	if got.StartTime != "2026-05-25T08:00:00" {
		t.Errorf("StartTime = %q", got.StartTime)
	}
}

func TestFromWireSessionSummary_Nil(t *testing.T) {
	got := FromWireSessionSummary(nil)
	if got.ID != nil || got.SessionDurationS != nil {
		t.Errorf("nil input should yield zero DTO, got %+v", got)
	}
}

func TestFromWireProgressSummary(t *testing.T) {
	var p gen.ProgressViewSummary
	p.TotalTrainingSessionCount.SetTo(12)
	p.TotalDistance.SetTo(84000)
	p.TotalCalories.SetTo(6100)
	var sd gen.StandardDuration
	sd.Millis = 3600000 * 7 // 7h
	p.TotalDuration.SetTo(sd)

	got := FromWireProgressSummary(&p, "2026-04-01", "2026-07-01")

	if got.FromDate != "2026-04-01" || got.ToDate != "2026-07-01" {
		t.Errorf("range = %q..%q", got.FromDate, got.ToDate)
	}
	if got.NumberOfSessions != 12 {
		t.Errorf("NumberOfSessions = %d, want 12", got.NumberOfSessions)
	}
	if got.TotalDurationS != 25200 {
		t.Errorf("TotalDurationS = %d, want 25200 (7h)", got.TotalDurationS)
	}
	if got.TotalDistanceM != 84000 {
		t.Errorf("TotalDistanceM = %v, want 84000", got.TotalDistanceM)
	}
	if got.TotalKcal != 6100 {
		t.Errorf("TotalKcal = %v, want 6100", got.TotalKcal)
	}
}

func TestFromWireProgressSummary_ZeroAccount(t *testing.T) {
	got := FromWireProgressSummary(&gen.ProgressViewSummary{}, "2026-04-01", "2026-07-01")
	if got.NumberOfSessions != 0 || got.TotalDurationS != 0 || got.TotalDistanceM != 0 {
		t.Errorf("zero account should map to zeros, got %+v", got)
	}
}
