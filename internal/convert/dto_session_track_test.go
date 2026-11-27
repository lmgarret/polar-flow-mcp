package convert

import (
	"math"
	"testing"
	"time"

	"github.com/go-faster/jx"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// SessionTrack is read off the cached details fixture: the GPS fixes there are
// synthetic, evenly spaced one minute apart from PT4.013S.
//nolint:gocyclo // one sequential scenario
func TestFromWireSessionTrack(t *testing.T) {
	d := loadDetailsFixture(t)
	b, ok := d.Samples.Or(nil)["9000000101"]
	if !ok {
		t.Fatal("fixture has no sample block for the exercise")
	}
	start := time.Date(2026, 1, 15, 17, 20, 0, 708_000_000, time.Local)
	tr, ok := FromWireSessionTrack(b, 9000000001, 9000000101, start, intPtr(3683), 0)
	if !ok {
		t.Fatal("no track reported for a GPS exercise")
	}
	if tr.PointCount != 121 || tr.ReturnedPoints != 121 || len(tr.Points) != 121 {
		t.Fatalf("points = %d (returned %d), want all 121", tr.PointCount, tr.ReturnedPoints)
	}
	if tr.Start != "2026-01-15T17:20:00" {
		t.Errorf("start = %q, want the exercise start to the second", tr.Start)
	}
	if tr.DurationS == nil || *tr.DurationS != 3683 {
		t.Errorf("duration_s = %v, want 3683", tr.DurationS)
	}
	first := tr.Points[0]
	if first.TS != 4 || first.Lat != 48.838001 || first.Lon != 2.440151 || first.AltitudeM == nil || *first.AltitudeM != 66.1 {
		t.Errorf("point 0 = %+v, want t 4 s at 48.838001/2.440151, 66.1 m", first)
	}
	if last := tr.Points[120]; last.TS != 3658 || last.Lat != 48.842225 {
		t.Errorf("last point = %+v, want t 3658 s (PT1H58.013S, rounded)", last)
	}
	if tr.DistanceM <= 0 {
		t.Errorf("distance_m = %v, want the haversine cumulation of the fixes", tr.DistanceM)
	}

	thin, _ := FromWireSessionTrack(b, 9000000001, 9000000101, start, intPtr(3683), 3)
	if thin.PointCount != 121 || thin.ReturnedPoints != 3 || len(thin.Points) != 3 {
		t.Fatalf("max 3: counts %d/%d", thin.PointCount, thin.ReturnedPoints)
	}
	for i, want := range []int{0, 60, 120} {
		if thin.Points[i].TS != tr.Points[want].TS || thin.Points[i].Lat != tr.Points[want].Lat {
			t.Fatalf("max 3: point %d = %+v, want fix %d", i, thin.Points[i], want)
		}
	}
}

func TestFromWireSessionTrackEdges(t *testing.T) {
	var block gen.SampleBlock
	if err := block.Decode(jx.DecodeStr(`{"HEART_RATE":{"samples":{"interval":"PT1S","values":[120]}},"COORDINATE":null}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := FromWireSessionTrack(block, 1, 2, time.Now(), nil, 0); ok {
		t.Error("a block without COORDINATE reported a track")
	}
	if err := block.Decode(jx.DecodeStr(`{"COORDINATE":{"samples":[]}}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := FromWireSessionTrack(block, 1, 2, time.Now(), nil, 0); ok {
		t.Error("an empty COORDINATE reported a track")
	}

	// altitude null stays null.
	if err := block.Decode(jx.DecodeStr(`{"COORDINATE":{"samples":[
		{"latitude":45,"longitude":6,"altitude":null,"time":"PT0S"},
		{"latitude":45.001,"longitude":6,"altitude":1200.5,"time":"PT10S"}]}}`)); err != nil {
		t.Fatal(err)
	}
	tr, ok := FromWireSessionTrack(block, 1, 2, time.Time{}, nil, 0)
	if !ok || len(tr.Points) != 2 {
		t.Fatalf("track = %+v (ok %v), want 2 points", tr, ok)
	}
	if tr.Points[0].AltitudeM != nil {
		t.Error("null altitude must stay null")
	}
	if tr.Start != "" || tr.DurationS != nil {
		t.Errorf("start/duration = %q/%v, want empty when the caller has none", tr.Start, tr.DurationS)
	}
	if math.Abs(tr.DistanceM-HaversineM(45, 6, 45.001, 6)) > 1e-6 {
		t.Errorf("distance_m = %v, want one haversine leg", tr.DistanceM)
	}
}
