package convert

import (
	"testing"
	"time"
)

func TestSecondsToClock(t *testing.T) {
	tests := []struct {
		secs int
		want string
	}{
		{0, "00:00:00"},
		{59, "00:00:59"},
		{60, "00:01:00"},
		{2100, "00:35:00"},
		{3661, "01:01:01"},
		{-5, "00:00:00"},
	}
	for _, tt := range tests {
		if got := SecondsToClock(tt.secs); got != tt.want {
			t.Errorf("SecondsToClock(%d) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}

func TestMillisToSeconds(t *testing.T) {
	tests := []struct {
		ms   int64
		want int
	}{
		{0, 0},
		{1800000, 1800},
		{999, 0},
		{1500, 1},
	}
	for _, tt := range tests {
		if got := MillisToSeconds(tt.ms); got != tt.want {
			t.Errorf("MillisToSeconds(%d) = %d, want %d", tt.ms, got, tt.want)
		}
	}
}

func TestISO8601DurationToSeconds(t *testing.T) {
	tests := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"PT30M", 1800, true},
		{"PT1H", 3600, true},
		{"PT1H2M3S", 3723, true},
		{"PT45S", 45, true},
		{"P1DT1H", 90000, true},
		{"PT0S", 0, true},
		{"", 0, false},
		{"PT", 0, false},
		{"P", 0, false},
		{"30M", 0, false},
		{"garbage", 0, false},
	}
	for _, tt := range tests {
		got, ok := ISO8601DurationToSeconds(tt.in)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("ISO8601DurationToSeconds(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestToDotDMY(t *testing.T) {
	got := ToDotDMY(time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC))
	if got != "27.4.2026" {
		t.Errorf("ToDotDMY = %q, want %q", got, "27.4.2026")
	}
}

func TestToDashDMY(t *testing.T) {
	got := ToDashDMY(time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC))
	if got != "07-04-2026" {
		t.Errorf("ToDashDMY = %q, want %q", got, "07-04-2026")
	}
}

func TestParseLocalDateTimeAndWireForms(t *testing.T) {
	when, err := ParseLocalDateTime("2026-06-02", "09:00")
	if err != nil {
		t.Fatalf("ParseLocalDateTime: %v", err)
	}
	if got := WireDateTimeTZLess(when); got != "2026-06-02T09:00" {
		t.Errorf("WireDateTimeTZLess = %q, want %q", got, "2026-06-02T09:00")
	}
	// Offset form: compare against the same instant formatted the reference way.
	if got, want := WireDateTimeOffset(when), when.Format("2006-01-02T15:04-0700"); got != want {
		t.Errorf("WireDateTimeOffset = %q, want %q", got, want)
	}
}

func TestParseLocalDateTimeErrors(t *testing.T) {
	if _, err := ParseLocalDateTime("06/02/2026", "09:00"); err == nil {
		t.Error("expected error for non-ISO date")
	}
	if _, err := ParseLocalDateTime("2026-06-02", "9am"); err == nil {
		t.Error("expected error for non-24h time")
	}
}

func TestNormalizeWireDatetime(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"2026-05-28 08:00:00.000", "2026-05-28T08:00:00"}, // space-separated (session summary list)
		{"2026-05-25T08:00:00", "2026-05-25T08:00:00"},     // tz-less ISO
		{"2026-07-03T07:28:09.855", "2026-07-03T07:28:09"}, // tz-less ISO, 3-digit millis (real startDate)
		{"2026-07-03T08:03:39.05", "2026-07-03T08:03:39"},  // tz-less ISO, 2-digit millis (real stopTime)
		{"2026-06-02T09:00", "2026-06-02T09:00:00"},        // no seconds
		{"", ""},
		{"not a date", "not a date"}, // passthrough
	}
	for _, tt := range tests {
		if got := NormalizeWireDatetime(tt.in); got != tt.want {
			t.Errorf("NormalizeWireDatetime(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// Offset-bearing input round-trips through RFC3339 (same instant).
	got := NormalizeWireDatetime("2026-05-25T08:48:00+02:00")
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Errorf("NormalizeWireDatetime offset form = %q, not RFC3339: %v", got, err)
	}
}

func TestHRZoneForLabel(t *testing.T) {
	tests := []struct {
		label  string
		lo, hi int
		ok     bool
	}{
		{"easy", 1, 2, true},
		{"aerobic", 2, 2, true},
		{"tempo", 3, 3, true},
		{"threshold", 4, 4, true},
		{"vo2max", 5, 5, true},
		{"nonsense", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, tt := range tests {
		lo, hi, ok := HRZoneForLabel(tt.label)
		if lo != tt.lo || hi != tt.hi || ok != tt.ok {
			t.Errorf("HRZoneForLabel(%q) = (%d,%d,%v), want (%d,%d,%v)", tt.label, lo, hi, ok, tt.lo, tt.hi, tt.ok)
		}
	}
}

func TestWireHRString(t *testing.T) {
	tests := []struct {
		bpm  int
		want string
	}{
		{0, ""},
		{-3, ""},
		{142, "142"},
	}
	for _, tt := range tests {
		if got := WireHRString(tt.bpm); got != tt.want {
			t.Errorf("WireHRString(%d) = %q, want %q", tt.bpm, got, tt.want)
		}
	}
}

func TestCleanNote(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{" ", ""}, {"", ""}, {"  real  ", "real"}, {"note", "note"},
	} {
		if got := CleanNote(tt.in); got != tt.want {
			t.Errorf("CleanNote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNilIfSentinelID(t *testing.T) {
	if got := NilIfSentinelID(-1); got != nil {
		t.Errorf("NilIfSentinelID(-1) = %v, want nil", *got)
	}
	if got := NilIfSentinelID(42); got == nil || *got != 42 {
		t.Errorf("NilIfSentinelID(42) = %v, want 42", got)
	}
}
