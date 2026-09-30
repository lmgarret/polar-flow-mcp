package convert

import (
	"testing"
	"time"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

func wakeState(state, offset int) gen.SleepNightSleepWakeStatesItem {
	return gen.SleepNightSleepWakeStatesItem{SleepWakeState: gen.NewOptInt(state), OffsetFromStart: gen.NewOptInt(offset)}
}

// A night from a device without Sleep Plus Stages (no cycles, no REM/light
// states): state 3 is plain sleep, not deep, and there are no stage totals —
// as in Flow's sleep report.
func TestFromWireSleepNight_WithoutStages(t *testing.T) {
	n := gen.SleepNight{
		Date:           time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
		SleepStartTime: "2026-09-28T23:30:00",
		SleepEndTime:   "2026-09-29T06:30:00",
		SleepWakeStates: []gen.SleepNightSleepWakeStatesItem{
			wakeState(3, 0), wakeState(0, 7200), wakeState(3, 7500),
		},
	}
	got, ok := FromWireSleepNight(n)
	if !ok {
		t.Fatal("night did not parse")
	}
	if got.Stages != nil || got.Score != nil || got.SleepS != 7*3600 || got.InterruptionsS != 300 {
		t.Fatalf("night = %+v", got)
	}
	want := []SleepSegment{{StageSleep, 0, 7200, false}, {StageWake, 7200, 7500, false}, {StageSleep, 7500, 25200, false}}
	if len(got.Hypnogram) != len(want) {
		t.Fatalf("hypnogram = %+v", got.Hypnogram)
	}
	for i := range want {
		if got.Hypnogram[i] != want[i] {
			t.Fatalf("segment %d = %+v, want %+v", i, got.Hypnogram[i], want[i])
		}
	}
}

func TestFromWireSleepNight_UnparseableTimes(t *testing.T) {
	if _, ok := FromWireSleepNight(gen.SleepNight{SleepStartTime: "yesterday", SleepEndTime: "today"}); ok {
		t.Fatal("expected the night to be skipped")
	}
}

// Bedtimes straddling midnight average around midnight, not noon.
func TestAverageSleep_BedtimeAcrossMidnight(t *testing.T) {
	avg := AverageSleep([]SleepNight{
		{FellAsleep: "2026-09-28T23:30:00", WokeUp: "2026-09-29T07:00:00", SleepS: 27000},
		{FellAsleep: "2026-09-30T00:30:00", WokeUp: "2026-09-30T07:30:00", SleepS: 25200},
	})
	if *avg.FellAsleep != "00:00" || *avg.WokeUp != "07:15" || *avg.SleepS != 26100 || avg.Score != nil {
		t.Fatalf("averages = %+v", avg)
	}
}
