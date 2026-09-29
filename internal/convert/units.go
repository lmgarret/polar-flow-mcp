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
//	Pace       seconds per km         (field suffix *_s_per_km)
//	Power      watts                  (field suffix *_w)
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
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
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

// HumanDuration formats a whole-second duration as prose ("1 h 5 min",
// "45 min", "30 s") for text a user reads rather than a machine parses — the
// confirmation prompts in internal/mcp, where SecondsToClock's "01:05:00" wire
// form reads like a stopwatch readout. Seconds are dropped once the duration
// reaches a minute, since no prompt needs that precision.
func HumanDuration(secs int) string {
	if secs <= 0 {
		return "0 s"
	}
	h, m, s := secs/3600, (secs%3600)/60, secs%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d h %d min", h, m)
	case h > 0:
		return fmt.Sprintf("%d h", h)
	case m > 0:
		return fmt.Sprintf("%d min", m)
	default:
		return fmt.Sprintf("%d s", s)
	}
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

// WireFavouriteTo renders the wall-clock time t as the only `to` form
// POST /training/target/createTargetFromFavourite accepts:
// "YYYY-MM-DDTHH:MM:SS.sss+00:00" (milliseconds and a numeric offset are both
// mandatory). Flow ignores the offset and schedules at the wall-clock part, so
// the offset is fixed rather than derived from t's location.
func WireFavouriteTo(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000") + "+00:00"
}

// ParseDashDMY parses Flow's "DD-MM-YYYY" date (the createTargetFromFavourite
// response's `date`) into a canonical ISO 8601 YYYY-MM-DD string. ok=false
// when s is not in that form.
func ParseDashDMY(s string) (string, bool) {
	t, err := time.Parse("02-01-2006", strings.TrimSpace(s))
	if err != nil {
		return "", false
	}
	return t.Format(ISODate), true
}

// ClockToSeconds parses an "HH:MM:SS" duration (training-target and favorite
// exerciseTargets / phases) into whole seconds. ok=false for anything else.
func ClockToSeconds(s string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 3 {
		return 0, false
	}
	var total int
	for i, mult := range []int{3600, 60, 1} {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 || (i > 0 && n > 59) {
			return 0, false
		}
		total += n * mult
	}
	return total, true
}

// Session "feeling" is stored on an inverted 0–1 scale: the web UI's five
// choices send "0.19" (best, "Au top") … "0.99" (worst, "Mal"). The canonical
// contract exposes it as an integer rating 1 (worst) – 5 (best).
var feelingWire = map[int]string{5: "0.19", 4: "0.39", 3: "0.59", 2: "0.79", 1: "0.99"}

// FeelingToWire maps a 1–5 rating (5 = best) to Flow's wire string.
// ok=false for any other rating.
func FeelingToWire(rating int) (string, bool) {
	v, ok := feelingWire[rating]
	return v, ok
}

// FeelingFromWire maps Flow's stored feeling back to a 1–5 rating. Values are
// matched to the nearest of the five UI anchors; 0 (Flow's "cleared / invalid
// input" value) and anything outside (0, 1] read as nil — no rating.
func FeelingFromWire(v float64) *int {
	if v <= 0 || v > 1 {
		return nil
	}
	// 0.19 → 5, 0.39 → 4, … 0.99 → 1: invert and bucket by 0.2.
	rating := 5 - int((v-0.09)/0.2)
	if rating < 1 {
		rating = 1
	}
	if rating > 5 {
		rating = 5
	}
	return &rating
}

// KmhToPaceSeconds converts a speed in km/h to a pace in whole seconds per
// kilometre (12 km/h → 300). ok=false for a non-positive speed, which has no
// finite pace.
func KmhToPaceSeconds(kmh float64) (int, bool) {
	if kmh <= 0 {
		return 0, false
	}
	return int(math.Round(3600 / kmh)), true
}

// PolarEnumTail lowercases the part of a Polar enum constant after marker:
// PolarEnumTail("SPEED_ZONE_CALCULATION_METHOD_MAS_BASED", "_METHOD_") →
// "mas_based", PolarEnumTail("FTP_SOURCE_ESTIMATED", "_SOURCE_") →
// "estimated". Returns "" for an empty or *_UNKNOWN value, and the whole value
// lowercased when marker is absent (so an unexpected constant still shows).
func PolarEnumTail(v, marker string) string {
	if i := strings.LastIndex(v, marker); i >= 0 {
		v = v[i+len(marker):]
	}
	v = strings.ToLower(v)
	if v == "unknown" {
		return ""
	}
	return v
}

// PaceClock formats a pace in seconds per km as "M:SS" (299 → "4:59").
func PaceClock(secs int) string {
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// PolarTextLen is a string's length as Flow's length limits count it: UTF-16
// code units (the backend is Java, String.length()). It equals the rune count
// except that each character outside the Basic Multilingual Plane — most
// emoji — counts 2 (probed 2026-09-29: 44 letters + "🏃" is rejected by a
// 45-character limit).
func PolarTextLen(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}
