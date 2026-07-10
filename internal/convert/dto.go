package convert

import "github.com/lmgarret/polar-flow-mcp/internal/flow/gen"

// This file holds the canonical response DTOs and their gen.* → DTO mappers.
// Each DTO uses the conventions documented in units.go: durations in seconds
// (*_s), distances in metres (*_m), heart rate in bpm, ISO datetimes, and
// pointer/omitempty fields for values that are genuinely absent (so Flow's ""
// / -1 / " " sentinels never leak to the caller).
//
// Field names are chosen to match what the MCP-app UIs already read as their
// first choice (e.g. session_duration_s, total_duration_s, number_of_sessions),
// so normalising the output completes the contract the UIs were coded against.

// SessionListItem is the canonical shape for one entry of list_training_sessions,
// mapped from gen.TrainingSessionSummary (whose duration is milliseconds, whose
// startDate is space-separated, and whose calorie/HR field names differ from the
// create and summary endpoints).
type SessionListItem struct {
	ID                int64    `json:"id"`
	SportID           int      `json:"sport_id"`
	SportName         string   `json:"sport_name,omitempty"`
	SportCategory     string   `json:"sport_category,omitempty"`
	StartTime         string   `json:"start_time,omitempty"`
	SessionDurationS  int      `json:"session_duration_s"`
	DistanceM         *float64 `json:"distance_m,omitempty"`
	HRAvg             *int     `json:"hr_avg,omitempty"`
	Calories          *int     `json:"calories,omitempty"`
	RecoveryTimeS     *int     `json:"recovery_time_s,omitempty"`
	Note              string   `json:"note,omitempty"`
	HasTrainingTarget bool     `json:"has_training_target"`
	IsTest            bool     `json:"is_test"`
}

// FromWireSessionListItem maps one gen.TrainingSessionSummary to the canonical
// SessionListItem.
func FromWireSessionListItem(s gen.TrainingSessionSummary) SessionListItem {
	item := SessionListItem{
		ID:               s.ID,
		SportID:          s.SportId,
		SessionDurationS: MillisToSeconds(int64(s.Duration)),
		StartTime:        NormalizeWireDatetime(s.StartDate),
	}
	if v, ok := s.SportName.Get(); ok {
		item.SportName = v
	}
	item.SportCategory = SportCategory(item.SportName, item.SportID)
	if v, ok := s.Distance.Get(); ok {
		item.DistanceM = &v
	}
	if v, ok := s.HrAvg.Get(); ok {
		item.HRAvg = &v
	}
	if v, ok := s.Calories.Get(); ok {
		item.Calories = &v
	}
	if v, ok := s.RecoveryTime.Get(); ok {
		// recoveryTime is milliseconds here (cross-checked: the list's
		// integer value equals the summary's ISO "PTxxH..S" for the same
		// session), so normalise to seconds like every other duration.
		secs := MillisToSeconds(int64(v))
		item.RecoveryTimeS = &secs
	}
	if v, ok := s.Note.Get(); ok {
		item.Note = CleanNote(v)
	}
	if v, ok := s.HasTrainingTarget.Get(); ok {
		item.HasTrainingTarget = v
	}
	if v, ok := s.IsTest.Get(); ok {
		item.IsTest = v
	}
	return item
}

// FromWireSessionList maps a slice of gen.TrainingSessionSummary to canonical
// items.
func FromWireSessionList(in []gen.TrainingSessionSummary) []SessionListItem {
	out := make([]SessionListItem, 0, len(in))
	for _, s := range in {
		out = append(out, FromWireSessionListItem(s))
	}
	return out
}

// SessionSummary is the canonical shape for get_training_session_summary,
// mapped from gen.SessionSummary (whose duration is an ISO 8601 "PTxxM" string
// and whose top-level distance/sportId are often null).
type SessionSummary struct {
	ID               *int64   `json:"id,omitempty"`
	Name             string   `json:"name,omitempty"`
	SportID          *int     `json:"sport_id,omitempty"`
	SportCategory    string   `json:"sport_category,omitempty"`
	StartTime        string   `json:"start_time,omitempty"`
	StopTime         string   `json:"stop_time,omitempty"`
	SessionDurationS *int     `json:"session_duration_s,omitempty"`
	DistanceM        *float64 `json:"distance_m,omitempty"`
	HRAvg            *int     `json:"hr_avg,omitempty"`
	HRMax            *int     `json:"hr_max,omitempty"`
	Calories         *int     `json:"calories,omitempty"`
	TrainingLoad     *float64 `json:"training_load,omitempty"`
	Note             string   `json:"note,omitempty"`
}

// FromWireSessionSummary maps gen.SessionSummary to the canonical SessionSummary.
func FromWireSessionSummary(s *gen.SessionSummary) SessionSummary {
	out := SessionSummary{}
	if s == nil {
		return out
	}
	if v, ok := s.ID.Get(); ok {
		out.ID = &v
	}
	if v, ok := s.SportId.Get(); ok {
		out.SportID = &v
	}
	if v, ok := s.StartDate.Get(); ok {
		out.StartTime = NormalizeWireDatetime(v)
	}
	if v, ok := s.StopTime.Get(); ok {
		out.StopTime = NormalizeWireDatetime(v)
	}
	if v, ok := s.Duration.Get(); ok {
		if secs, parsed := ISO8601DurationToSeconds(v); parsed {
			out.SessionDurationS = &secs
		}
	}
	if v, ok := s.Distance.Get(); ok {
		out.DistanceM = &v
	}
	if v, ok := s.HrAverage.Get(); ok {
		out.HRAvg = &v
	}
	if v, ok := s.HrMax.Get(); ok {
		out.HRMax = &v
	}
	if v, ok := s.KiloCalories.Get(); ok {
		out.Calories = &v
	}
	if v, ok := s.TrainingLoad.Get(); ok {
		out.TrainingLoad = &v
	}
	if v, ok := s.TrainingSessionName.Get(); ok {
		out.Name = v
	}
	if v, ok := s.Note.Get(); ok {
		out.Note = CleanNote(v)
	}
	id := 0
	if out.SportID != nil {
		id = *out.SportID
	}
	out.SportCategory = SportCategory(out.Name, id)
	return out
}

// ProgressSummary is the canonical shape for get_progress_summary, mapped from
// gen.ProgressViewSummary (whose total duration is a StandardDuration object and
// whose totals are float32 metres/kcal). The from/to range is supplied by the
// handler. The three breakdown lists are passed through as-is: their element
// shapes are only partly pinned in the spec, so the canonical layer normalises
// the reliable headline totals and leaves the breakdowns untouched (Pragmatic
// scope — deep normalisation of these is deferred).
type ProgressSummary struct {
	FromDate                 string  `json:"from_date,omitempty"`
	ToDate                   string  `json:"to_date,omitempty"`
	NumberOfSessions         int     `json:"number_of_sessions"`
	TotalDurationS           int     `json:"total_duration_s"`
	TotalDistanceM           float64 `json:"total_distance_m"`
	TotalKcal                float64 `json:"total_kcal"`
	TotalAscentM             float64 `json:"total_ascent_m,omitempty"`
	TotalDescentM            float64 `json:"total_descent_m,omitempty"`
	SportBreakdown           any     `json:"sport_breakdown,omitempty"`
	HeartRateZones           any     `json:"heart_rate_zones,omitempty"`
	TrainingBenefitBreakdown any     `json:"training_benefit_breakdown,omitempty"`
	// SportCategories maps each sport name appearing in the (raw) breakdown to
	// its UI category, so the progress app can colour bars without re-deriving
	// the classification client-side. Derived, not a normalisation of the raw
	// breakdown lists themselves (which stay pass-through by design).
	SportCategories map[string]string `json:"sport_categories,omitempty"`
}

// FromWireProgressSummary maps gen.ProgressViewSummary plus the request date
// range to the canonical ProgressSummary.
func FromWireProgressSummary(p *gen.ProgressViewSummary, fromDate, toDate string) ProgressSummary {
	out := ProgressSummary{FromDate: fromDate, ToDate: toDate}
	if p == nil {
		return out
	}
	if v, ok := p.TotalTrainingSessionCount.Get(); ok {
		out.NumberOfSessions = v
	}
	if v, ok := p.TotalDuration.Get(); ok {
		out.TotalDurationS = MillisToSeconds(v.Millis)
	}
	if v, ok := p.TotalDistance.Get(); ok {
		out.TotalDistanceM = float64(v)
	}
	if v, ok := p.TotalCalories.Get(); ok {
		out.TotalKcal = float64(v)
	}
	if v, ok := p.TotalAscent.Get(); ok {
		out.TotalAscentM = float64(v)
	}
	if v, ok := p.TotalDescent.Get(); ok {
		out.TotalDescentM = float64(v)
	}
	if v, ok := p.SportDistributions.Get(); ok {
		out.SportBreakdown = v
		cats := map[string]string{}
		for _, list := range [][]gen.SportDistributionEntry{v.Distance, v.Duration, v.Sessions} {
			for _, e := range list {
				if name, ok := e.SportName.Get(); ok && name != "" {
					cats[name] = SportCategory(name, 0)
				}
			}
		}
		if len(cats) > 0 {
			out.SportCategories = cats
		}
	}
	if len(p.TotalHeartRateZoneList) > 0 {
		out.HeartRateZones = p.TotalHeartRateZoneList
	}
	if len(p.TrainingBenefitDistributionList) > 0 {
		out.TrainingBenefitBreakdown = p.TrainingBenefitDistributionList
	}
	return out
}
