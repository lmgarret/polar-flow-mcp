// Package convert is the adapter seam between the MCP tool surface and the
// Polar Flow wire API. The Flow web API is internally inconsistent — durations
// arrive in five encodings, dates in ~eight, and units and field names differ
// endpoint to endpoint (see docs/reference/units-and-dates.md). This package
// owns one canonical, unit-consistent contract and does all wire conversion in
// one place:
//
//	Duration   integer seconds        (field suffix *_s)
//	Distance   metres                 (field suffix *_m)
//	Speed      km/h                   (field suffix *_kmh)
//	Heart rate integer bpm, null when absent
//	Date       ISO 8601 YYYY-MM-DD
//	Datetime   ISO 8601 (tz-less for targets, offset for sessions)
//	Absent     JSON null — never "", -1, or " "
//
// units.go holds the pure primitive converters (no gen dependency); dto.go
// holds the response DTOs and their gen.* → canonical mappers.
package convert

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ISODate is the canonical date-only layout (YYYY-MM-DD).
const ISODate = "2006-01-02"

// clock24 is the canonical 24-hour wall-clock layout (HH:MM).
const clock24 = "15:04"

// ParseISODate parses a canonical YYYY-MM-DD date. The returned error message is
// caller-friendly (suitable for surfacing straight to an MCP client).
func ParseISODate(s string) (time.Time, error) {
	t, err := time.Parse(ISODate, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be ISO 8601 YYYY-MM-DD: %w", err)
	}
	return t, nil
}

// ToDotDMY formats t as "D.M.YYYY" (single-digit day/month, no zero-padding) —
// the form the calendar endpoints (getCalendarEvents, getCalendarWeekSummary)
// expect.
func ToDotDMY(t time.Time) string {
	return fmt.Sprintf("%d.%d.%d", t.Day(), int(t.Month()), t.Year())
}

// ToDashDMY formats t as "DD-MM-YYYY" (dashes, zero-padded) — the form the
// progress endpoints (getProgressViewSummaryAsJson, getSummaryDataAsJson)
// expect. This differs from the dot form used by the calendar endpoints; the
// two are not interchangeable.
func ToDashDMY(t time.Time) string {
	return fmt.Sprintf("%02d-%02d-%04d", t.Day(), int(t.Month()), t.Year())
}

// ParseLocalDateTime parses a canonical date (YYYY-MM-DD) and 24h clock (HH:MM)
// in the local timezone into a single time.Time. It is the one validated
// assembly path shared by every write handler, replacing the two divergent
// implementations that previously lived in the target and session builders.
// Callers pick the wire encoding with WireDateTimeTZLess or WireDateTimeOffset.
func ParseLocalDateTime(dateStr, clockStr string) (time.Time, error) {
	day, err := time.ParseInLocation(ISODate, dateStr, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must be ISO 8601 YYYY-MM-DD: %w", err)
	}
	hm, err := time.ParseInLocation(clock24, clockStr, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("time must be HH:MM (24h): %w", err)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), hm.Hour(), hm.Minute(), 0, 0, time.Local), nil
}

// WireDateTimeTZLess renders t as "YYYY-MM-DDTHH:MM" with no timezone — the form
// TrainingTargetCreate.datetime wants (the server applies the account timezone).
func WireDateTimeTZLess(t time.Time) string {
	return t.Format("2006-01-02T15:04")
}

// WireDateTimeOffset renders t as "YYYY-MM-DDTHH:MM±ZZZZ" — the form
// TrainingSessionCreate.date wants (local datetime with an explicit offset).
func WireDateTimeOffset(t time.Time) string {
	return t.Format("2006-01-02T15:04-0700")
}

// SecondsToClock formats a whole-second duration as the "HH:MM:SS" string the
// training-target endpoints expect (PhaseLeaf.duration, ExerciseTarget.duration).
func SecondsToClock(secs int) string {
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)
}

// MillisToSeconds converts an integer-millisecond duration (as returned by
// TrainingSessionSummary.duration and StandardDuration.millis) to whole seconds.
func MillisToSeconds(ms int64) int {
	return int(ms / 1000)
}

var isoDurationRE = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// ISO8601DurationToSeconds parses an ISO 8601 duration string (e.g. "PT30M",
// "PT1H2M3S") — the form SessionSummary.duration uses — into whole seconds.
// Returns ok=false when the string is empty or not a recognised duration.
func ISO8601DurationToSeconds(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "P" || s == "PT" {
		return 0, false
	}
	m := isoDurationRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	var total float64
	units := []float64{86400, 3600, 60, 1} // D, H, M, S
	for i, mult := range units {
		if m[i+1] == "" {
			continue
		}
		v, err := strconv.ParseFloat(m[i+1], 64)
		if err != nil {
			return 0, false
		}
		total += v * mult
	}
	return int(total), true
}

// wireDatetimeLayouts are the datetime encodings observed across Flow read
// endpoints, tried in order by NormalizeWireDatetime.
var wireDatetimeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05.000",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05.000", // TrainingSessionSummary.startDate (space separator)
	"2006-01-02 15:04:05",
}

// NormalizeWireDatetime coerces any of the datetime encodings the read
// endpoints emit (tz-less ISO, space-separated, millisecond, offset/Z) into a
// canonical ISO 8601 string: RFC3339 when the source carries a timezone,
// otherwise "YYYY-MM-DDTHH:MM:SS". The input is returned unchanged when it
// matches none of the known layouts, so callers never lose data.
func NormalizeWireDatetime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, layout := range wireDatetimeLayouts {
		t, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		if strings.ContainsAny(layout, "Z7") {
			return t.Format(time.RFC3339)
		}
		return t.Format("2006-01-02T15:04:05")
	}
	return s
}

// WireHRString renders a heart-rate value for TrainingSessionCreate, whose
// hrAverage/hrMax fields are integer-as-string and expect "" (not "0" or null)
// when unset.
func WireHRString(bpm int) string {
	if bpm <= 0 {
		return ""
	}
	return strconv.Itoa(bpm)
}

// HRZoneForLabel maps an effort label to a (lower, upper) Polar HR zone pair.
// Conservative single-zone defaults (except "easy", a 1–2 band); callers can
// override by supplying an explicit hr_zone. ok=false for an unknown label.
func HRZoneForLabel(label string) (lower, upper int, ok bool) {
	switch label {
	case "easy":
		return 1, 2, true
	case "aerobic":
		return 2, 2, true
	case "tempo":
		return 3, 3, true
	case "threshold":
		return 4, 4, true
	case "vo2max":
		return 5, 5, true
	default:
		return 0, 0, false
	}
}

// CleanNote normalises a free-text note read from the wire: Flow returns a
// single space " " (and sometimes "") for a blank note. Returns "" for any
// all-whitespace value so the canonical layer can omit it.
func CleanNote(s string) string {
	return strings.TrimSpace(s)
}

// NilIfSentinelID maps Flow's "-1 means none" id sentinel (used by
// nextTrainingId / previousTrainingId) to a nil pointer.
func NilIfSentinelID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}
