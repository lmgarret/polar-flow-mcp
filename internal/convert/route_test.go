package convert

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/go-faster/jx"
)

// Files fed to the Flow web UI's import dialog on 2026-09-28, with the
// importRoute bodies it produced. The parser must reproduce those bodies.
const (
	gpxNoNameNoEle = `<?xml version="1.0"?><gpx version="1.1" creator="t" xmlns="http://www.topografix.com/GPX/1/1"><trk><trkseg><trkpt lat="45.0" lon="6.0"></trkpt><trkpt lat="45.001" lon="6.001"></trkpt><trkpt lat="45.002" lon="6.0015"><ele>1200.5</ele></trkpt></trkseg></trk></gpx>`
	// The UI sent name "Itinéraire 1" (its placeholder) — the name is chosen
	// by the tool, so it is set explicitly below.
	uiBodyGPX = `{"route":[{"latitude":45,"longitude":6,"altitude":0,"distance":0},{"latitude":45.001,"longitude":6.001,"altitude":0,"distance":136.18501998200443},{"latitude":45.002,"longitude":6.0015,"altitude":1200.5,"distance":254.12470690353456}],"sport":null,"name":"Itinéraire 1","distance":254.12470690353456}`

	tcxCourse = `<?xml version="1.0" encoding="UTF-8"?><TrainingCenterDatabase xmlns="http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2"><Courses><Course><Name>TcxCourse</Name><Track><Trackpoint><Position><LatitudeDegrees>45.0</LatitudeDegrees><LongitudeDegrees>6.0</LongitudeDegrees></Position><AltitudeMeters>100</AltitudeMeters><DistanceMeters>0</DistanceMeters></Trackpoint><Trackpoint><Position><LatitudeDegrees>45.001</LatitudeDegrees><LongitudeDegrees>6.001</LongitudeDegrees></Position><AltitudeMeters>101</AltitudeMeters><DistanceMeters>500</DistanceMeters></Trackpoint><Trackpoint><Position><LatitudeDegrees>45.002</LatitudeDegrees><LongitudeDegrees>6.0015</LongitudeDegrees></Position><DistanceMeters>900</DistanceMeters></Trackpoint></Track></Course></Courses></TrainingCenterDatabase>`
	uiBodyTCX = `{"route":[{"latitude":45,"longitude":6,"altitude":100,"distance":0},{"latitude":45.001,"longitude":6.001,"altitude":101,"distance":500},{"latitude":45.002,"longitude":6.0015,"altitude":null,"distance":900}],"sport":null,"name":"TcxCourse","distance":900}`

	tcxActivity = `<?xml version="1.0"?><TrainingCenterDatabase xmlns="http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2"><Activities><Activity Sport="Running"><Id>2026-01-01T10:00:00Z</Id><Lap StartTime="2026-01-01T10:00:00Z"><Track><Trackpoint><Time>2026-01-01T10:00:00Z</Time><Position><LatitudeDegrees>45.0</LatitudeDegrees><LongitudeDegrees>6.0</LongitudeDegrees></Position><DistanceMeters>0</DistanceMeters></Trackpoint><Trackpoint><Time>2026-01-01T10:00:05Z</Time><HeartRateBpm><Value>120</Value></HeartRateBpm></Trackpoint><Trackpoint><Time>2026-01-01T10:00:10Z</Time><Position><LatitudeDegrees>45.001</LatitudeDegrees><LongitudeDegrees>6.001</LongitudeDegrees></Position><DistanceMeters>140</DistanceMeters></Trackpoint></Track></Lap></Activity></Activities></TrainingCenterDatabase>`
	uiBodyTCXActivity = `{"route":[{"time":"2026-01-01T10:00:00.000Z","latitude":45,"longitude":6,"altitude":null,"distance":0},{"time":"2026-01-01T10:00:10.000Z","latitude":45.001,"longitude":6.001,"altitude":null,"distance":140}],"sport":null,"name":"Itinéraire 1","distance":140}`
)

func mustJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad JSON fixture: %v", err)
	}
	return v
}

// The wire body the tool uploads must equal what the web UI uploads for the
// same file — field names, altitude 0-vs-null convention, haversine distances.
func TestToWireRouteImport_MatchesWebUI(t *testing.T) {
	tests := []struct {
		name, file, uiBody, routeName string
	}{
		{"gpx without name or elevation", gpxNoNameNoEle, uiBodyGPX, "Itinéraire 1"},
		{"tcx course", tcxCourse, uiBodyTCX, "TcxCourse"},
		{"tcx activity", tcxActivity, uiBodyTCXActivity, "Itinéraire 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRoute([]byte(tt.file), "auto")
			if err != nil {
				t.Fatalf("ParseRoute: %v", err)
			}
			body := ToWireRouteImport(r, tt.routeName, nil)
			var e jx.Encoder
			body.Encode(&e)
			got, want := mustJSON(t, e.String()), mustJSON(t, tt.uiBody)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("wire body mismatch\n got: %s\nwant: %s", e.String(), tt.uiBody)
			}
		})
	}
}

func TestParseRoute_Names(t *testing.T) {
	tests := []struct {
		name, file, want string
	}{
		{"trk name wins over metadata", `<gpx><metadata><name>Meta</name></metadata><trk><name>Trk</name><trkseg><trkpt lat="45" lon="6"/><trkpt lat="45.001" lon="6"/></trkseg></trk></gpx>`, "Trk"},
		{"metadata when trk unnamed", `<gpx><metadata><name>Meta</name></metadata><trk><trkseg><trkpt lat="45" lon="6"/><trkpt lat="45.001" lon="6"/></trkseg></trk></gpx>`, "Meta"},
		{"rte name", `<gpx><rte><name>Rte</name><rtept lat="45" lon="6"/><rtept lat="45.001" lon="6"/></rte></gpx>`, "Rte"},
		{"none", `<gpx><trk><trkseg><trkpt lat="45" lon="6"/><trkpt lat="45.001" lon="6"/></trkseg></trk></gpx>`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRoute([]byte(tt.file), "gpx")
			if err != nil {
				t.Fatalf("ParseRoute: %v", err)
			}
			if r.Name != tt.want {
				t.Fatalf("name = %q, want %q", r.Name, tt.want)
			}
		})
	}
}

// The UI keeps only the first <trkseg>; the tool keeps every segment of the
// first track (and still ignores later tracks).
func TestParseRoute_GPXSegmentsAndTracks(t *testing.T) {
	file := `<gpx><trk><name>A</name><trkseg><trkpt lat="45" lon="6"/><trkpt lat="45.001" lon="6.001"/></trkseg><trkseg><trkpt lat="45.002" lon="6.002"/></trkseg></trk><trk><name>B</name><trkseg><trkpt lat="46" lon="7"/></trkseg></trk></gpx>`
	r, err := ParseRoute([]byte(file), "")
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	if len(r.Points) != 3 {
		t.Fatalf("points = %d, want 3 (both segments of the first track)", len(r.Points))
	}
	if r.Points[0].DistanceM != 0 || !(r.Points[2].DistanceM > r.Points[1].DistanceM) {
		t.Fatalf("cumulative distances not increasing: %+v", r.Points)
	}
	if r.DistanceM != r.Points[2].DistanceM {
		t.Fatalf("total %v != last point %v", r.DistanceM, r.Points[2].DistanceM)
	}
}

func TestParseRoute_TCXWithoutDistancesFallsBackToHaversine(t *testing.T) {
	file := `<TrainingCenterDatabase><Courses><Course><Name>C</Name><Track><Trackpoint><Position><LatitudeDegrees>45</LatitudeDegrees><LongitudeDegrees>6</LongitudeDegrees></Position></Trackpoint><Trackpoint><Position><LatitudeDegrees>45.001</LatitudeDegrees><LongitudeDegrees>6.001</LongitudeDegrees></Position></Trackpoint></Track></Course></Courses></TrainingCenterDatabase>`
	r, err := ParseRoute([]byte(file), "tcx")
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	if math.Abs(r.DistanceM-136.18501998200443) > 1e-6 {
		t.Fatalf("distance = %v, want the UI's haversine 136.185…", r.DistanceM)
	}
}

func TestParseRoute_Rejects(t *testing.T) {
	onePoint := `<gpx><trk><trkseg><trkpt lat="45" lon="6"/></trkseg></trk></gpx>`
	samePoint := `<gpx><trk><trkseg><trkpt lat="45" lon="6"/><trkpt lat="45" lon="6"/></trkseg></trk></gpx>`
	tests := []struct {
		name, file, format, wantErr string
	}{
		{"empty", "  ", "", "empty"},
		{"not xml", "this is not xml", "", "not valid XML"},
		{"json instead of xml", `{"route":[]}`, "", "not valid XML"},
		{"unknown root", `<kml><Document/></kml>`, "", "neither <gpx> nor <TrainingCenterDatabase>"},
		{"format mismatch", tcxCourse, "gpx", `format "gpx" was requested`},
		{"bad format arg", gpxNoNameNoEle, "fit", `format must be "auto", "gpx" or "tcx"`},
		{"gpx without track or route", `<gpx><wpt lat="45" lon="6"/></gpx>`, "", "no <trk> or <rte>"},
		{"tcx without course or activity", `<TrainingCenterDatabase/>`, "", "no <Course> or <Activity>"},
		{"zero points", `<gpx><trk><trkseg></trkseg></trk></gpx>`, "", "0 point(s)"},
		{"one point", onePoint, "", "1 point(s)"},
		{"tcx only hr samples", `<TrainingCenterDatabase><Activities><Activity><Lap><Track><Trackpoint><Time>2026-01-01T10:00:00Z</Time></Trackpoint><Trackpoint><Time>2026-01-01T10:00:01Z</Time></Trackpoint></Track></Lap></Activity></Activities></TrainingCenterDatabase>`, "", "0 point(s)"},
		{"latitude out of range", `<gpx><trk><trkseg><trkpt lat="91" lon="6"/><trkpt lat="45" lon="6"/></trkseg></trk></gpx>`, "", "latitude 91"},
		{"longitude out of range", `<gpx><trk><trkseg><trkpt lat="45" lon="6"/><trkpt lat="45" lon="200"/></trkseg></trk></gpx>`, "", "point 1: longitude 200"},
		{"non-numeric lat", `<gpx><trk><trkseg><trkpt lat="north" lon="6"/><trkpt lat="45" lon="6"/></trkseg></trk></gpx>`, "", `lat "north" is not a number`},
		{"missing lon", `<gpx><trk><trkseg><trkpt lat="45"/><trkpt lat="45" lon="6"/></trkseg></trk></gpx>`, "", "lon \"\" is not a number"},
		{"zero length", samePoint, "", "zero length"},
		{"decreasing tcx distance", strings.Replace(tcxCourse, "<DistanceMeters>900</DistanceMeters>", "<DistanceMeters>100</DistanceMeters>", 1), "", "decreases"},
		{"truncated xml", gpxNoNameNoEle[:120], "", "invalid GPX"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRoute([]byte(tt.file), tt.format)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRoute_TooLarge(t *testing.T) {
	big := make([]byte, MaxRouteFileBytes+1)
	for i := range big {
		big[i] = ' '
	}
	copy(big, "<gpx>")
	if _, err := ParseRoute(big, ""); err == nil || !strings.Contains(err.Error(), "25 MB") {
		t.Fatalf("err = %v, want the 25 MB limit", err)
	}
}

func TestValidateRouteName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"ok", "Park loop", ""},
		{"45 runes ok", strings.Repeat("é", 45), ""},
		{"46 runes", strings.Repeat("r", 46), "46 characters"},
		{"empty", "", "non-whitespace"},
		{"blank", "   ", "non-whitespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRouteName(tt.in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestHaversineM_MatchesWebUI(t *testing.T) {
	// Captured cumulative distance for (45,6) → (45.001,6.001).
	if got := HaversineM(45, 6, 45.001, 6.001); got != 136.18501998200443 {
		t.Fatalf("HaversineM = %.12f, want 136.185019982004", got)
	}
}
